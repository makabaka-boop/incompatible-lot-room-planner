package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func do(t *testing.T, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/allocate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewMux().ServeHTTP(rec, req)
	return rec
}

func TestAllocateOK(t *testing.T) {
	rec := do(t, http.MethodPost, `{
		"batches": ["c", "a", "b"],
		"capacity": 2,
		"exclusions": [["a", "c"]]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	var resp allocateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := allocateResponse{
		Batches:   []string{"a", "b", "c"},
		RoomCount: 2,
		Vector:    []int{0, 0, 1},
		Rooms:     [][]string{{"a", "b"}, {"c"}},
		Occupancy: []int{2, 1},
	}
	if !reflect.DeepEqual(resp, want) {
		t.Fatalf("got %+v, want %+v", resp, want)
	}
}

// The same instance with a different input order must produce the same
// canonical answer.
func TestAllocateOrderIndependent(t *testing.T) {
	body1 := `{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","c"]]}`
	body2 := `{"batches":["c","b","a"],"capacity":2,"exclusions":[["c","a"]]}`
	r1, r2 := do(t, http.MethodPost, body1), do(t, http.MethodPost, body2)
	if r1.Code != 200 || r2.Code != 200 {
		t.Fatalf("statuses %d, %d", r1.Code, r2.Code)
	}
	if r1.Body.String() != r2.Body.String() {
		t.Fatalf("order-dependent results:\n%s\n%s", r1.Body, r2.Body)
	}
}

func TestAllocateUnprocessable(t *testing.T) {
	many := make([]string, 15)
	for i := range many {
		many[i] = fmt.Sprintf("b%02d", i)
	}
	manyJSON, _ := json.Marshal(map[string]any{"batches": many, "capacity": 2})

	cases := map[string]string{
		"extra field":            `{"batches":["a","b"],"capacity":2,"foo":1}`,
		"duplicate edge flipped": `{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","b"],["b","a"]]}`,
		"duplicate edge exact":   `{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","b"],["a","b"]]}`,
		"self exclusion":         `{"batches":["a","b"],"capacity":2,"exclusions":[["a","a"]]}`,
		"unknown id":             `{"batches":["a","b"],"capacity":2,"exclusions":[["a","z"]]}`,
		"duplicate batch":        `{"batches":["a","a"],"capacity":2}`,
		"too few batches":        `{"batches":["a"],"capacity":2}`,
		"no batches":             `{"capacity":2}`,
		"too many batches":       string(manyJSON),
		"capacity zero":          `{"batches":["a","b"],"capacity":0}`,
		"capacity too big":       `{"batches":["a","b"],"capacity":15}`,
		"capacity missing":       `{"batches":["a","b"]}`,
		"non-ascii id":           `{"batches":["a","bé"],"capacity":2}`,
		"empty id":               `{"batches":["a",""],"capacity":2}`,
		"exclusion triple":       `{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","b","c"]]}`,
		"exclusion single":       `{"batches":["a","b"],"capacity":2,"exclusions":[["a"]]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, http.MethodPost, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status %d, want 422; body %s", rec.Code, rec.Body)
			}
			var errResp map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatal(err)
			}
			if errResp["error"] == "" {
				t.Fatalf("missing error message in %s", rec.Body)
			}
		})
	}
}

func TestAllocateBadRequest(t *testing.T) {
	cases := map[string]string{
		"malformed json":   `{"batches":`,
		"type mismatch":    `{"batches":["a","b"],"capacity":"two"}`,
		"trailing garbage": `{"batches":["a","b"],"capacity":2} extra`,
		"empty body":       ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, http.MethodPost, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, http.MethodGet, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	NewMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
}
