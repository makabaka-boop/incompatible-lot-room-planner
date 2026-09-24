package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postPlan(t *testing.T, body string) (int, map[string]any) {
	t.Helper()
	srv := httptest.NewServer(routesForTest())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/plan", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func TestPlanSuccessAndCanonicalShape(t *testing.T) {
	// Greedy trap: ac, bd, cd with capacity 2 -> exact optimum 2 rooms.
	body := `{
	  "batches": ["d","c","b","a"],
	  "capacity": 2,
	  "exclusions": [["a","c"],["b","d"],["c","d"]]
	}`
	status, out := postPlan(t, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, out)
	}

	if n := intFromAny(t, out, "room_count"); n != 2 {
		t.Fatalf("room_count=%d want 2 (must not be greedy's 3)", n)
	}
	batches := toStringSlice(t, out["batches"])
	wantBatches := []string{"a", "b", "c", "d"}
	if !equalStrings(batches, wantBatches) {
		t.Fatalf("batches=%v want %v (sorted by UTF-8 bytes)", batches, wantBatches)
	}
	vec := toIntSlice(t, out["numbering_vector"])
	if !equalInts(vec, []int{0, 1, 1, 0}) {
		t.Fatalf("numbering_vector=%v want [0 1 1 0] for sorted batches a,b,c,d", vec)
	}

	rooms := out["rooms"].([]any)
	if len(rooms) != 2 {
		t.Fatalf("rooms length=%d want 2", len(rooms))
	}
	for i, r := range rooms {
		rm := r.(map[string]any)
		if int(rm["room"].(float64)) != i {
			t.Fatalf("room numbering not contiguous from 0")
		}
		b := toStringSlice(t, rm["batches"])
		occ := int(rm["occupancy"].(float64))
		if occ != len(b) {
			t.Fatalf("occupancy=%d != len(batches)=%d", occ, len(b))
		}
	}
}

func TestPlanEmptyExclusions(t *testing.T) {
	status, out := postPlan(t, `{"batches":["x","y"],"capacity":14,"exclusions":[]}`)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, out)
	}
	if n := intFromAny(t, out, "room_count"); n != 1 {
		t.Fatalf("room_count=%d want 1", n)
	}
	if vec := toIntSlice(t, out["numbering_vector"]); !equalInts(vec, []int{0, 0}) {
		t.Fatalf("vector=%v want [0 0]", vec)
	}
}

func TestPlanCompleteExclusion(t *testing.T) {
	status, out := postPlan(t, `{
	  "batches":["p","q","r"],
	  "capacity":9,
	  "exclusions":[["p","q"],["q","r"],["r","p"]]
	}`)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, out)
	}
	if n := intFromAny(t, out, "room_count"); n != 3 {
		t.Fatalf("room_count=%d want 3", n)
	}
}

func TestValidation422(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"duplicate edge reversed",
			`{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","b"],["b","a"]]}`},
		{"duplicate edge same order",
			`{"batches":["a","b","c"],"capacity":2,"exclusions":[["a","b"],["a","b"]]}`},
		{"self exclusion",
			`{"batches":["a","b"],"capacity":2,"exclusions":[["a","a"]]}`},
		{"unknown id in edge",
			`{"batches":["a","b"],"capacity":2,"exclusions":[["a","zzz"]]}`},
		{"extra top level field",
			`{"batches":["a","b"],"capacity":2,"exclusions":[],"surprise":1}`},
		{"extra field nested is irrelevant but extra inside pair",
			`{"batches":["a","b"],"capacity":2,"exclusions":[["a","b","c"]]}`},
		{"too few batches", `{"batches":["a"],"capacity":2,"exclusions":[]}`},
		{"too many batches",
			`{"batches":["a","b","c","d","e","f","g","h","i","j","k","l","m","n","o"],"capacity":2,"exclusions":[]}`},
		{"capacity zero", `{"batches":["a","b"],"capacity":0,"exclusions":[]}`},
		{"capacity too big", `{"batches":["a","b"],"capacity":15,"exclusions":[]}`},
		{"capacity float", `{"batches":["a","b"],"capacity":2.5,"exclusions":[]}`},
		{"capacity string", `{"batches":["a","b"],"capacity":"2","exclusions":[]}`},
		{"missing exclusions", `{"batches":["a","b"],"capacity":2}`},
		{"missing capacity", `{"batches":["a","b"],"exclusions":[]}`},
		{"missing batches", `{"capacity":2,"exclusions":[]}`},
		{"duplicate batch ids", `{"batches":["a","a"],"capacity":2,"exclusions":[]}`},
		{"non ascii id", `{"batches":["a","危"],"capacity":2,"exclusions":[]}`},
		{"empty id", `{"batches":["a",""],"capacity":2,"exclusions":[]}`},
		{"space in id", `{"batches":["a","b c"],"capacity":2,"exclusions":[]}`},
		{"batches wrong type", `{"batches":"ab","capacity":2,"exclusions":[]}`},
		{"exclusions wrong type", `{"batches":["a","b"],"capacity":2,"exclusions":{}}`},
		{"edge element number", `{"batches":["a","b"],"capacity":2,"exclusions":[["a"]]}`},
		{"edge id number", `{"batches":["a","b"],"capacity":2,"exclusions":[[1,2]]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, out := postPlan(t, tc.body)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d want 422, body=%v", status, out)
			}
			if _, ok := out["error"]; !ok {
				t.Fatalf("422 response missing error field: %v", out)
			}
		})
	}
}

func TestMalformedAndMethod(t *testing.T) {
	srv := httptest.NewServer(routesForTest())
	defer srv.Close()

	// Broken JSON -> 400.
	resp, err := http.Post(srv.URL+"/plan", "application/json", bytes.NewBufferString(`{"batches":`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("broken JSON status=%d want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Empty body -> 400.
	resp, err = http.Post(srv.URL+"/plan", "application/json", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body status=%d want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Trailing token -> 400.
	resp, err = http.Post(srv.URL+"/plan", "application/json",
		strings.NewReader(`{"batches":["a","b"],"capacity":2,"exclusions":[]} junk`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("trailing data status=%d want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /plan -> 405.
	resp, err = http.Get(srv.URL + "/plan")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /plan status=%d want 405", resp.StatusCode)
	}
	resp.Body.Close()

	// Health check.
	resp, err = http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status=%d want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func routesForTest() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /plan", handlePlan)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func intFromAny(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q missing or not a number: %v", key, m)
	}
	return int(v)
}

func toStringSlice(t *testing.T, v any) []string {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("not an array: %v", v)
	}
	out := make([]string, len(arr))
	for i, e := range arr {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("element %d not string: %v", i, e)
		}
		out[i] = s
	}
	return out
}

func toIntSlice(t *testing.T, v any) []int {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("not an array: %v", v)
	}
	out := make([]int, len(arr))
	for i, e := range arr {
		f, ok := e.(float64)
		if !ok {
			t.Fatalf("element %d not number: %v", i, e)
		}
		out[i] = int(f)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
