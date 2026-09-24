// Command api serves the hazardous-chemical storage planning JSON API.
//
// It is a pure net/http backend (Go 1.23, no third-party dependencies).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"hazchem-store/storage"
)

const maxBatchIDLen = 64

// planWire is the first-stage request shape. Fields are RawMessages so
// that presence and exact types can be validated independently and so
// that unknown fields can be rejected with a 422.
type planWire struct {
	Batches    json.RawMessage `json:"batches"`
	Capacity   json.RawMessage `json:"capacity"`
	Exclusions json.RawMessage `json:"exclusions"`
}

type roomDTO struct {
	Room      int      `json:"room"`
	Batches   []string `json:"batches"`
	Occupancy int      `json:"occupancy"`
}

type planResponse struct {
	Batches         []string  `json:"batches"`
	RoomCount       int       `json:"room_count"`
	NumberingVector []int     `json:"numbering_vector"`
	Rooms           []roomDTO `json:"rooms"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /plan", handlePlan)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("hazchem-store api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handlePlan(w http.ResponseWriter, r *http.Request) {
	// Keep one maliciously large body from exhausting memory.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var wire planWire
	if err := dec.Decode(&wire); err != nil {
		var syntaxErr *json.SyntaxError
		var maxBytesErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF), errors.As(err, &syntaxErr),
			errors.Is(err, io.ErrUnexpectedEOF):
			// Empty body or broken JSON syntax: the document itself
			// cannot be parsed.
			writeError(w, http.StatusBadRequest, "malformed JSON body")
		case errors.As(err, &maxBytesErr):
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			// Unknown top-level fields, wrong root type, etc. are
			// semantically invalid requests: 422.
			writeError(w, http.StatusUnprocessableEntity, "malformed request: "+err.Error())
		}
		return
	}
	// Reject trailing data beyond the single JSON document.
	if tok, err := dec.Token(); !errors.Is(err, io.EOF) {
		_ = tok
		writeError(w, http.StatusBadRequest, "extraneous data after JSON document")
		return
	}

	ids, pairs, capacity, ok := validate(w, wire)
	if !ok {
		return
	}

	sol := storage.Solve(ids, pairs, capacity)

	sortedIDs := append([]string(nil), ids...)
	sortStrings(sortedIDs)

	rooms := make([]roomDTO, sol.RoomCount)
	for r := 0; r < sol.RoomCount; r++ {
		rooms[r] = roomDTO{
			Room:      r,
			Batches:   sol.Rooms[r],
			Occupancy: sol.Occupancy[r],
		}
	}
	writeJSON(w, http.StatusOK, planResponse{
		Batches:         sortedIDs,
		RoomCount:       sol.RoomCount,
		NumberingVector: reorderVector(sol.Assignment, ids, sortedIDs),
		Rooms:           rooms,
	})
}

// reorderVector makes the numbering vector index by the sorted id order
// advertised in the response, instead of the request's id order.
func reorderVector(assignmentInRequestOrder []int, ids, sortedIDs []string) []int {
	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	out := make([]int, len(sortedIDs))
	for i, id := range sortedIDs {
		out[i] = assignmentInRequestOrder[index[id]]
	}
	return out
}

// validate performs every request-level check. On failure it writes the
// 422 itself and returns ok == false.
func validate(w http.ResponseWriter, wire planWire) (ids []string, pairs [][2]int, capacity int, ok bool) {
	fail := func(msg string) ([]string, [][2]int, int, bool) {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return nil, nil, 0, false
	}

	if len(wire.Batches) == 0 || len(wire.Capacity) == 0 || len(wire.Exclusions) == 0 {
		return fail("request must contain batches, capacity and exclusions")
	}

	// Batches must be a JSON array of strings.
	if err := strictUnmarshal(wire.Batches, &ids); err != nil {
		return fail("batches must be an array of strings: " + err.Error())
	}
	if len(ids) < 2 || len(ids) > 14 {
		return fail(fmt.Sprintf("batches must contain between 2 and 14 unique ids, got %d", len(ids)))
	}
	seenID := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !validBatchID(id) {
			return fail(fmt.Sprintf("invalid batch id %q: must be 1-%d printable ASCII characters without spaces", id, maxBatchIDLen))
		}
		if seenID[id] {
			return fail("duplicate batch id: " + id)
		}
		seenID[id] = true
	}

	// Capacity must be a plain JSON integer.
	if capVal, err := decodeInt(wire.Capacity); err != nil {
		return fail("capacity must be an integer: " + err.Error())
	} else {
		capacity = capVal
	}
	if capacity < 1 || capacity > 14 {
		return fail(fmt.Sprintf("capacity must be between 1 and 14, got %d", capacity))
	}

	// Exclusions must be a JSON array of [string, string] pairs.
	var rawPairs []json.RawMessage
	if err := strictUnmarshal(wire.Exclusions, &rawPairs); err != nil {
		return fail("exclusions must be an array of [id, id] pairs: " + err.Error())
	}
	pairs = make([][2]int, 0, len(rawPairs))
	type edge struct{ a, b int }
	seenEdge := make(map[edge]bool, len(rawPairs))
	for i, rp := range rawPairs {
		var elems []json.RawMessage
		if err := strictUnmarshal(rp, &elems); err != nil {
			return fail(fmt.Sprintf("exclusions[%d] must be a [id, id] pair: %s", i, err.Error()))
		}
		if len(elems) != 2 {
			return fail(fmt.Sprintf("exclusions[%d] must contain exactly two ids, got %d", i, len(elems)))
		}
		var a, b string
		if err := strictUnmarshal(elems[0], &a); err != nil {
			return fail(fmt.Sprintf("exclusions[%d][0] must be a string id: %s", i, err.Error()))
		}
		if err := strictUnmarshal(elems[1], &b); err != nil {
			return fail(fmt.Sprintf("exclusions[%d][1] must be a string id: %s", i, err.Error()))
		}
		ai, aok := lookupID(ids, a)
		bi, bok := lookupID(ids, b)
		if !aok || !bok {
			return fail(fmt.Sprintf("exclusions[%d] references unknown batch id", i))
		}
		if ai == bi {
			return fail(fmt.Sprintf("exclusions[%d] declares a batch mutually exclusive with itself", i))
		}
		lo, hi := ai, bi
		if lo > hi {
			lo, hi = hi, lo
		}
		e := edge{lo, hi}
		if seenEdge[e] {
			return fail(fmt.Sprintf("exclusions[%d] duplicates an existing undirected exclusion edge", i))
		}
		seenEdge[e] = true
		pairs = append(pairs, [2]int{ai, bi})
	}

	return ids, pairs, capacity, true
}

// strictUnmarshal rejects unknown fields and trailing tokens for the
// document fragment.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if tok, err := dec.Token(); !errors.Is(err, io.EOF) {
		_ = tok
		return errors.New("extraneous data in value")
	}
	return nil
}

// decodeInt accepts only an integral JSON number (3, not 3.0/true/"3").
func decodeInt(data []byte) (int, error) {
	var n int
	if err := strictUnmarshal(data, &n); err != nil {
		return 0, err
	}
	return n, nil
}

func validBatchID(id string) bool {
	if len(id) == 0 || len(id) > maxBatchIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c < 0x21 || c > 0x7E {
			return false
		}
	}
	return true
}

func lookupID(ids []string, id string) (int, bool) {
	for i, x := range ids {
		if x == id {
			return i, true
		}
	}
	return 0, false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}
