// Package httpapi exposes the batch-to-room allocator as a JSON HTTP API.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"warehouse/internal/alloc"
)

const (
	minBatches = 2
	maxBatches = 14
	minCap     = 1
	maxCap     = 14
	maxBody    = 1 << 20 // 1 MiB request body limit
)

// NewMux routes the API endpoints.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /allocate", handleAllocate)
	mux.HandleFunc("GET /healthz", handleHealthz)
	return mux
}

type allocateRequest struct {
	Batches    []string   `json:"batches"`
	Capacity   int        `json:"capacity"`
	Exclusions [][]string `json:"exclusions"`
}

type allocateResponse struct {
	Batches   []string   `json:"batches"`   // ids sorted in UTF-8 byte order; vector aligns with this list
	RoomCount int        `json:"roomCount"` // number of rooms used (minimal)
	Vector    []int      `json:"vector"`    // vector[i] = room number of batches[i]
	Rooms     [][]string `json:"rooms"`     // rooms[r] = sorted ids stored in room r
	Occupancy []int      `json:"occupancy"` // occupancy[r] = len(rooms[r])
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleAllocate(w http.ResponseWriter, r *http.Request) {
	var req allocateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			// Extra fields reject the whole request with 422.
			fail(w, http.StatusUnprocessableEntity, err.Error())
		} else {
			fail(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		}
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "unexpected data after JSON body")
		return
	}

	sorted, forb, msg := validate(&req)
	if msg != "" {
		fail(w, http.StatusUnprocessableEntity, msg)
		return
	}

	n := len(sorted)
	vector := alloc.Assign(n, req.Capacity, forb)

	roomCount := 0
	for _, rno := range vector {
		if rno+1 > roomCount {
			roomCount = rno + 1
		}
	}
	rooms := make([][]string, roomCount)
	for i, rno := range vector {
		rooms[rno] = append(rooms[rno], sorted[i])
	}
	occupancy := make([]int, roomCount)
	for r := range rooms {
		occupancy[r] = len(rooms[r])
	}

	writeJSON(w, http.StatusOK, allocateResponse{
		Batches:   sorted,
		RoomCount: roomCount,
		Vector:    vector,
		Rooms:     rooms,
		Occupancy: occupancy,
	})
}

// validate checks the request semantics and, when valid, returns the batch
// ids sorted in UTF-8 byte order together with the exclusion adjacency
// bitmask indexed into that order.
func validate(req *allocateRequest) (sorted []string, forb []uint32, msg string) {
	n := len(req.Batches)
	if n < minBatches || n > maxBatches {
		return nil, nil, "batches must contain between 2 and 14 unique ids"
	}
	if req.Capacity < minCap || req.Capacity > maxCap {
		return nil, nil, "capacity must be between 1 and 14"
	}
	seen := make(map[string]struct{}, n)
	for _, id := range req.Batches {
		if id == "" {
			return nil, nil, "batch ids must be non-empty"
		}
		for i := 0; i < len(id); i++ {
			if id[i] >= 0x80 {
				return nil, nil, "batch ids must be ASCII: " + id
			}
		}
		if _, dup := seen[id]; dup {
			return nil, nil, "duplicate batch id: " + id
		}
		seen[id] = struct{}{}
	}

	sorted = make([]string, n)
	copy(sorted, req.Batches)
	sort.Strings(sorted) // string order is UTF-8 byte order
	index := make(map[string]int, n)
	for i, id := range sorted {
		index[id] = i
	}

	forb = make([]uint32, n)
	type edge struct{ a, b int }
	edges := make(map[edge]struct{}, len(req.Exclusions))
	for _, pair := range req.Exclusions {
		if len(pair) != 2 {
			return nil, nil, "each exclusion must be a pair of batch ids"
		}
		ai, ok := index[pair[0]]
		if !ok {
			return nil, nil, "exclusion references unknown batch id: " + pair[0]
		}
		bi, ok := index[pair[1]]
		if !ok {
			return nil, nil, "exclusion references unknown batch id: " + pair[1]
		}
		if ai == bi {
			return nil, nil, "self exclusion is not allowed: " + pair[0]
		}
		e := edge{min(ai, bi), max(ai, bi)}
		if _, dup := edges[e]; dup {
			return nil, nil, "duplicate exclusion edge: " + pair[0] + " / " + pair[1]
		}
		edges[e] = struct{}{}
		forb[ai] |= 1 << bi
		forb[bi] |= 1 << ai
	}
	return sorted, forb, ""
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
