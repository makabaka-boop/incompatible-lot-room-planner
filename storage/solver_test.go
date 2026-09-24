package storage

import (
	"math/rand"
	"testing"
)

// checkSolution independently validates a solution against the request:
// every batch appears exactly once, room sizes respect capacity, and no
// room contains an adjacent pair. It also verifies canonical labeling
// (rooms numbered by first appearance in sorted id order).
func checkSolution(t *testing.T, ids []string, edges [][2]int, capacity int, sol Solution) {
	t.Helper()
	n := len(ids)

	sorted := append([]string(nil), ids...)
	sortStrings(sorted)

	if len(sol.Assignment) != n {
		t.Fatalf("assignment length = %d, want %d", len(sol.Assignment), n)
	}
	if len(sol.Rooms) != sol.RoomCount || len(sol.Occupancy) != sol.RoomCount {
		t.Fatalf("room list/occupancy length mismatch: rooms=%d occ=%d count=%d",
			len(sol.Rooms), len(sol.Occupancy), sol.RoomCount)
	}

	idIndex := map[string]int{}
	for i, id := range ids {
		idIndex[id] = i
	}
	conflict := make([][]bool, n)
	for i := range conflict {
		conflict[i] = make([]bool, n)
	}
	for _, e := range edges {
		conflict[e[0]][e[1]] = true
		conflict[e[1]][e[0]] = true
	}

	seen := make([]bool, n)
	firstSeenRoom := map[int]int{} // room -> sorted position of first member
	for r, room := range sol.Rooms {
		if sol.Occupancy[r] != len(room) {
			t.Fatalf("room %d occupancy %d != listed %d", r, sol.Occupancy[r], len(room))
		}
		if len(room) > capacity {
			t.Fatalf("room %d holds %d > capacity %d", r, len(room), capacity)
		}
		for i := 1; i < len(room); i++ {
			if room[i-1] >= room[i] {
				t.Fatalf("room %d batches not byte-sorted: %v", r, room)
			}
		}
		for _, id := range room {
			idx, ok := idIndex[id]
			if !ok {
				t.Fatalf("unknown id %q in room %d", id, r)
			}
			if seen[idx] {
				t.Fatalf("batch %q assigned more than once", id)
			}
			seen[idx] = true
			if sol.Assignment[idx] != r {
				t.Fatalf("batch %q in room %d but assignment vector says %d",
					id, r, sol.Assignment[idx])
			}
		}
		for i := 0; i < len(room); i++ {
			for j := i + 1; j < len(room); j++ {
				if conflict[idIndex[room[i]]][idIndex[room[j]]] {
					t.Fatalf("room %d contains mutually exclusive pair %q,%q",
						r, room[i], room[j])
				}
			}
		}
		for pos, id := range sorted {
			if room[0] == id {
				if prev, ok := firstSeenRoom[r]; ok && prev < pos {
					t.Fatalf("room %d first appearance inconsistent", r)
				} else if !ok {
					firstSeenRoom[r] = pos
				}
			}
		}
	}
	for i, s := range seen {
		if !s {
			t.Fatalf("batch %q not assigned", ids[i])
		}
	}

	// Canonical numbering: labels assigned in order of first appearance.
	expectRoom := 0
	roomOfPos := make([]int, n)
	for pos, id := range sorted {
		roomOfPos[pos] = sol.Assignment[idIndex[id]]
	}
	firstAppeared := map[int]bool{}
	for _, r := range roomOfPos {
		if !firstAppeared[r] {
			if r != expectRoom {
				t.Fatalf("rooms not numbered by first appearance: got room %d, want %d", r, expectRoom)
			}
			firstAppeared[r] = true
			expectRoom++
		}
	}
}

// bruteOptimal is an independent reference solver: it enumerates every
// restricted growth string (each unlabeled partition exactly once) and
// keeps the valid partition with the fewest rooms, ties broken by the
// lexicographically smallest vector. Exponential; used for n <= 9.
func bruteOptimal(n, capacity int, adj []uint16) (vector []int, rooms int) {
	a := make([]int, n)
	roomMask := make([]uint16, n+1)
	roomSize := make([]int, n+1)
	used := 1
	bestRooms := n + 1
	var best []int

	var dfs func(int)
	dfs = func(i int) {
		if i == n {
			// Snapshot now: a is mutated again during backtracking,
			// so the comparison must never alias it.
			cur := append([]int(nil), a...)
			if used < bestRooms || (used == bestRooms && lexLess(cur, best)) {
				bestRooms = used
				best = cur
			}
			return
		}
		for r := 0; r < used; r++ {
			if roomSize[r] >= capacity || adj[i]&roomMask[r] != 0 {
				continue
			}
			a[i] = r
			roomMask[r] |= 1 << uint(i)
			roomSize[r]++
			dfs(i + 1)
			roomMask[r] &^= 1 << uint(i)
			roomSize[r]--
		}
		r := used
		a[i] = r
		roomMask[r] = 1 << uint(i)
		roomSize[r] = 1
		used++
		dfs(i + 1)
		used--
		roomMask[r] = 0
		roomSize[r] = 0
		// a[i] is intentionally left as r: the next sibling branch
		// overwrites it before reaching a leaf.
	}
	a[0] = 0
	roomMask[0] = 1
	roomSize[0] = 1
	dfs(1)
	return best, bestRooms
}

func lexLess(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// canonicalVector converts the solution vector to the sorted-id order
// used by the reference solver.
func canonicalVector(ids []string, sol Solution) []int {
	sorted := append([]string(nil), ids...)
	sortStrings(sorted)
	idx := map[string]int{}
	for i, id := range ids {
		idx[id] = i
	}
	out := make([]int, len(ids))
	for pos, id := range sorted {
		out[pos] = sol.Assignment[idx[id]]
	}
	return out
}

func makeAdj(n int, edges [][2]int) []uint16 {
	adj := make([]uint16, n)
	for _, e := range edges {
		adj[e[0]] |= 1 << uint(e[1])
		adj[e[1]] |= 1 << uint(e[0])
	}
	return adj
}

func idsN(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "B" + string(rune('a'+i))
	}
	return ids
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// TestExhaustiveSmall enumerates every graph on n <= 5 vertices and every
// capacity, comparing the exact solver against brute force.
func TestExhaustiveSmall(t *testing.T) {
	for n := 2; n <= 5; n++ {
		ids := idsN(n)
		maxEdges := n * (n - 1) / 2
		for graphBits := 0; graphBits < 1<<uint(maxEdges); graphBits++ {
			var edges [][2]int
			b := 0
			for u := 0; u < n; u++ {
				for v := u + 1; v < n; v++ {
					if graphBits&(1<<uint(b)) != 0 {
						edges = append(edges, [2]int{u, v})
					}
					b++
				}
			}
			adj := makeAdj(n, edges)
			for cap := 1; cap <= n; cap++ {
				sol := Solve(ids, edges, cap)
				checkSolution(t, ids, edges, cap, sol)
				wantVec, wantRooms := bruteOptimal(n, cap, adj)
				gotVec := canonicalVector(ids, sol)
				if sol.RoomCount != wantRooms {
					t.Fatalf("n=%d graph=%b cap=%d: rooms=%d want %d (edges=%v)",
						n, graphBits, cap, sol.RoomCount, wantRooms, edges)
				}
				if !equalInts(gotVec, wantVec) {
					t.Fatalf("n=%d graph=%b cap=%d: vector=%v want %v",
						n, graphBits, cap, gotVec, wantVec)
				}
			}
		}
	}
}

// TestRandomLarger samples random graphs on 6..9 vertices and compares
// against the brute-force reference.
func TestRandomLarger(t *testing.T) {
	rng := rand.New(rand.NewSource(20260924))
	for n := 6; n <= 9; n++ {
		ids := idsN(n)
		for trial := 0; trial < 120; trial++ {
			var edges [][2]int
			p := []float64{0.1, 0.35, 0.6, 0.9}[trial%4]
			for u := 0; u < n; u++ {
				for v := u + 1; v < n; v++ {
					if rng.Float64() < p {
						edges = append(edges, [2]int{u, v})
					}
				}
			}
			cap := 1 + rng.Intn(n)
			adj := makeAdj(n, edges)
			sol := Solve(ids, edges, cap)
			checkSolution(t, ids, edges, cap, sol)
			wantVec, wantRooms := bruteOptimal(n, cap, adj)
			gotVec := canonicalVector(ids, sol)
			if sol.RoomCount != wantRooms || !equalInts(gotVec, wantVec) {
				t.Fatalf("n=%d trial=%d cap=%d edges=%v: got (rooms=%d vec=%v), want (rooms=%d vec=%v)",
					n, trial, cap, edges, sol.RoomCount, gotVec, wantRooms, wantVec)
			}
		}
	}
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

// TestCompleteExclusion: pairwise mutually exclusive batches each need a
// private room regardless of capacity.
func TestCompleteExclusion(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	var edges [][2]int
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			edges = append(edges, [2]int{i, j})
		}
	}
	for _, cap := range []int{1, 2, 14} {
		sol := Solve(ids, edges, cap)
		checkSolution(t, ids, edges, cap, sol)
		if sol.RoomCount != 4 {
			t.Fatalf("K4 cap=%d: rooms=%d want 4", cap, sol.RoomCount)
		}
		for r := 0; r < 4; r++ {
			if sol.Occupancy[r] != 1 || sol.Rooms[r][0] != ids[r] {
				t.Fatalf("K4 cap=%d: room %d not singleton %q", cap, r, ids[r])
			}
		}
	}
}

// TestEmptyExclusion: with no constraints everyone packs greedily by
// capacity, and the canonical vector is the lex-smallest packing.
func TestEmptyExclusion(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	var edges [][2]int

	sol := Solve(ids, edges, 2)
	checkSolution(t, ids, edges, 2, sol)
	if sol.RoomCount != 3 {
		t.Fatalf("empty graph cap=2: rooms=%d want 3", sol.RoomCount)
	}
	if got := canonicalVector(ids, sol); !equalInts(got, []int{0, 0, 1, 1, 2}) {
		t.Fatalf("empty graph cap=2: vector=%v want [0 0 1 1 2]", got)
	}

	sol = Solve(ids, edges, 14)
	if sol.RoomCount != 1 {
		t.Fatalf("empty graph cap=14: rooms=%d want 1", sol.RoomCount)
	}

	sol = Solve(ids, edges, 1)
	if sol.RoomCount != 5 {
		t.Fatalf("empty graph cap=1: rooms=%d want 5", sol.RoomCount)
	}
	if got := canonicalVector(ids, sol); !equalInts(got, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("empty graph cap=1: vector=%v want identity", got)
	}
}

// TestTieBreaking covers a case with several optimum partitions and
// checks the canonical (lexicographically smallest first-appearance
// numbering) one is chosen.
func TestTieBreaking(t *testing.T) {
	// cap=3, n=4, no edges: optimum is 2 rooms and the lex-smallest
	// partition is {a,b,c}|{d} -> [0 0 0 1].
	ids := []string{"a", "b", "c", "d"}
	sol := Solve(ids, nil, 3)
	checkSolution(t, ids, nil, 3, sol)
	if sol.RoomCount != 2 {
		t.Fatalf("rooms=%d want 2", sol.RoomCount)
	}
	if got := canonicalVector(ids, sol); !equalInts(got, []int{0, 0, 0, 1}) {
		t.Fatalf("vector=%v want [0 0 0 1]", got)
	}
	if len(sol.Rooms[0]) != 3 || len(sol.Rooms[1]) != 1 || sol.Rooms[1][0] != "d" {
		t.Fatalf("unexpected rooms: %v", sol.Rooms)
	}

	// Request order shuffled must not change the canonical result.
	shuffled := []string{"d", "b", "a", "c"}
	sol2 := Solve(shuffled, nil, 3)
	checkSolution(t, shuffled, nil, 3, sol2)
	if got := canonicalVector(shuffled, sol2); !equalInts(got, []int{0, 0, 0, 1}) {
		t.Fatalf("shuffled request changed canonical result: %v", got)
	}
}

// TestGreedyIsSuboptimal reproduces the warehouse keeper's trap:
// "put the next batch in the first room that fits" uses 3 rooms here,
// while the exact optimum uses 2. Edges: ac, bd, cd (cap=2).
//
//	a -> room0 {a}
//	b -> room0 {a,b}
//	c conflicts with a -> room1 {c}
//	d conflicts with b and c -> opens room2   (greedy: 3 rooms)
//
// optimum: {a,d} | {b,c} -> canonical vector [0 1 1 0].
func TestGreedyIsSuboptimal(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	edges := [][2]int{{0, 2}, {1, 3}, {2, 3}} // ac, bd, cd
	sol := Solve(ids, edges, 2)
	checkSolution(t, ids, edges, 2, sol)
	if sol.RoomCount != 2 {
		t.Fatalf("exact optimum rooms=%d want 2 (greedy would return 3)", sol.RoomCount)
	}
	if got := canonicalVector(ids, sol); !equalInts(got, []int{0, 1, 1, 0}) {
		t.Fatalf("vector=%v want [0 1 1 0]", got)
	}
	wantRooms := [][]string{{"a", "d"}, {"b", "c"}}
	for r := range wantRooms {
		if !equalStrings(sol.Rooms[r], wantRooms[r]) {
			t.Fatalf("room %d = %v, want %v", r, sol.Rooms[r], wantRooms[r])
		}
	}
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

// TestByteOrder uses ids whose ASCII/UTF-8 byte order differs from a
// casual alphabetic reading: 'B' (0x42) sorts before 'a' (0x61).
func TestByteOrder(t *testing.T) {
	ids := []string{"a", "B", "_", "z"}
	sol := Solve(ids, nil, 1)
	checkSolution(t, ids, nil, 1, sol)
	// Byte order is 0x42 'B' < 0x5F '_' < 0x61 'a' < 0x7A 'z', so the
	// canonical vector (indexed by byte-sorted ids) is [0 1 2 3] ...
	if got := canonicalVector(ids, sol); !equalInts(got, []int{0, 1, 2, 3}) {
		t.Fatalf("canonical byte-order vector=%v want [0 1 2 3]", got)
	}
	// ... while the same solution expressed in the request's id order
	// (a, B, _, z) is [2 0 1 3].
	if !equalInts(sol.Assignment, []int{2, 0, 1, 3}) {
		t.Fatalf("request-order vector=%v want [2 0 1 3]", sol.Assignment)
	}
	if sol.Rooms[0][0] != "B" || sol.Rooms[1][0] != "_" ||
		sol.Rooms[2][0] != "a" || sol.Rooms[3][0] != "z" {
		t.Fatalf("byte-order rooms = %v", sol.Rooms)
	}
}

// TestWorstCase14 ensures the exact search stays fast at the maximum
// input size for several difficult structures.
func TestWorstCase14(t *testing.T) {
	n := 14
	ids := idsN(n)

	type tc struct {
		name  string
		edges [][2]int
		cap   int
	}
	cases := []tc{
		{"empty-cap1", nil, 1},
		{"empty-cap2", nil, 2},
		{"empty-cap14", nil, 14},
		{"complete-cap14", completeEdges(n), 14},
		{"matching-cap2", matchingEdges(n), 2},
		{"cycle-cap2", cycleEdges(n), 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sol := Solve(ids, c.edges, c.cap)
			checkSolution(t, ids, c.edges, c.cap, sol)
			if sol.RoomCount < 1 {
				t.Fatal("no solution")
			}
		})
	}
}

func completeEdges(n int) [][2]int {
	var e [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			e = append(e, [2]int{i, j})
		}
	}
	return e
}

func matchingEdges(n int) [][2]int {
	var e [][2]int
	for i := 0; i+1 < n; i += 2 {
		e = append(e, [2]int{i, i + 1})
	}
	return e
}

func cycleEdges(n int) [][2]int {
	var e [][2]int
	for i := 0; i < n; i++ {
		e = append(e, [2]int{i, (i + 1) % n})
	}
	return e
}
