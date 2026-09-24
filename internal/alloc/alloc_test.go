package alloc

import (
	"math/rand"
	"slices"
	"testing"
)

func completeForb(n int) []uint32 {
	forb := make([]uint32, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j {
				forb[i] |= 1 << j
			}
		}
	}
	return forb
}

func link(forb []uint32, a, b int) {
	forb[a] |= 1 << b
	forb[b] |= 1 << a
}

// Complete mutual exclusion: every batch needs its own room.
func TestCompleteExclusion(t *testing.T) {
	const n = 6
	got := Assign(n, 14, completeForb(n))
	want := []int{0, 1, 2, 3, 4, 5}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Empty exclusion graph: batches pack into ceil(n/cap) rooms, filling the
// lowest-numbered rooms first.
func TestEmptyExclusion(t *testing.T) {
	forb := make([]uint32, 5)
	got := Assign(5, 2, forb)
	want := []int{0, 0, 1, 1, 2}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEmptyExclusionSingleRoom(t *testing.T) {
	forb := make([]uint32, 14)
	got := Assign(14, 14, forb)
	want := make([]int, 14) // all in room 0
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEmptyExclusionCapacityOne(t *testing.T) {
	forb := make([]uint32, 14)
	got := Assign(14, 1, forb)
	want := make([]int, 14)
	for i := range want {
		want[i] = i
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Two optimal 2-room layouts exist ({0,1}|{2} and {0}|{1,2}); the
// lexicographically smallest vector [0 0 1] must win.
func TestLexicographicTieBreak(t *testing.T) {
	forb := make([]uint32, 3)
	link(forb, 0, 2)
	got := Assign(3, 2, forb)
	want := []int{0, 0, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Optima [0 0 1 1] and [0 1 0 1] tie on room count; [0 0 1 1] is smaller.
func TestLexicographicTieBreak4(t *testing.T) {
	forb := make([]uint32, 4)
	link(forb, 0, 3)
	link(forb, 1, 2)
	got := Assign(4, 2, forb)
	want := []int{0, 0, 1, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Crown graph on 6 batches: a greedy first-fit pass over the sorted ids
// opens 3 rooms ({0,1}|{2,3}|{4,5}) while the optimum is 2 rooms
// ({0,2,4}|{1,3,5}). The solver must not pass off the greedy result as
// optimal.
func TestGreedyWouldBeSuboptimal(t *testing.T) {
	forb := make([]uint32, 6)
	link(forb, 0, 3)
	link(forb, 0, 5)
	link(forb, 2, 1)
	link(forb, 2, 5)
	link(forb, 4, 1)
	link(forb, 4, 3)
	got := Assign(6, 3, forb)
	want := []int{0, 1, 0, 1, 0, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// bruteForce enumerates every canonical grouping (restricted growth
// string) and keeps the best valid one: fewest rooms, then the
// lexicographically smallest vector. It is deliberately independent of the
// solver's pruning and lower bounds, so the two can cross-check each other.
func bruteForce(n, capacity int, forb []uint32) []int {
	cur := make([]int, n)
	var best []int
	var rec func(pos, rooms int)
	rec = func(pos, rooms int) {
		if pos == n {
			size := make([]int, rooms)
			for i, r := range cur {
				size[r]++
				if size[r] > capacity {
					return
				}
				for j := 0; j < i; j++ {
					if cur[j] == r && forb[i]&(1<<j) != 0 {
						return
					}
				}
			}
			cand := slices.Clone(cur)
			if best == nil || better(cand, best) {
				best = cand
			}
			return
		}
		for r := 0; r <= rooms; r++ {
			cur[pos] = r
			next := rooms
			if r == rooms {
				next = rooms + 1
			}
			rec(pos+1, next)
		}
	}
	rec(0, 0)
	return best
}

func better(a, b []int) bool {
	ra, rb := roomCount(a), roomCount(b)
	if ra != rb {
		return ra < rb
	}
	return slices.Compare(a, b) < 0
}

func roomCount(v []int) int {
	m := 0
	for _, r := range v {
		if r+1 > m {
			m = r + 1
		}
	}
	return m
}

// Exhaustively cross-check every exclusion graph on 4 and 5 batches
// against the brute-force enumerator, for every capacity.
func TestExhaustiveSmall(t *testing.T) {
	for _, n := range []int{4, 5} {
		var pairs [][2]int
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				pairs = append(pairs, [2]int{i, j})
			}
		}
		for mask := 0; mask < 1<<len(pairs); mask++ {
			forb := make([]uint32, n)
			for b, p := range pairs {
				if mask>>b&1 == 1 {
					link(forb, p[0], p[1])
				}
			}
			for cap := 1; cap <= n; cap++ {
				got := Assign(n, cap, forb)
				want := bruteForce(n, cap, forb)
				if !slices.Equal(got, want) {
					t.Fatalf("n=%d mask=%b cap=%d: got %v, want %v", n, mask, cap, got, want)
				}
			}
		}
	}
}

// Randomized cross-check on larger instances (up to 8 batches, Bell(8)
// canonical groupings each), including empty and complete graphs.
func TestAssignMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 300; trial++ {
		n := 2 + rng.Intn(7) // 2..8
		cap := 1 + rng.Intn(n)
		p := []float64{0, 0.2, 0.4, 0.6, 0.8, 1}[rng.Intn(6)]
		forb := make([]uint32, n)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if rng.Float64() < p {
					link(forb, i, j)
				}
			}
		}
		got := Assign(n, cap, forb)
		want := bruteForce(n, cap, forb)
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d cap=%d forb=%v: got %v, want %v", n, cap, forb, got, want)
		}
	}
}

func checkValid(t *testing.T, n, capacity int, forb []uint32, v []int) {
	t.Helper()
	if len(v) != n {
		t.Fatalf("vector length %d, want %d", len(v), n)
	}
	size := map[int]int{}
	mem := map[int]uint32{}
	maxRoom := -1
	for i, r := range v {
		if r < 0 || r > maxRoom+1 {
			t.Fatalf("vector %v is not in canonical first-appearance form", v)
		}
		if r > maxRoom {
			maxRoom = r
		}
		size[r]++
		if size[r] > capacity {
			t.Fatalf("room %d exceeds capacity in %v", r, v)
		}
		if mem[r]&forb[i] != 0 {
			t.Fatalf("excluded batches share room %d in %v", r, v)
		}
		mem[r] |= 1 << i
	}
}

// Large instances (n = 14) cannot be brute-forced, but the result must
// still be a valid canonical assignment and reproducible run to run.
func TestLargeValidAndDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 50; trial++ {
		const n = 14
		cap := 1 + rng.Intn(14)
		p := []float64{0, 0.25, 0.5, 0.75}[rng.Intn(4)]
		forb := make([]uint32, n)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if rng.Float64() < p {
					link(forb, i, j)
				}
			}
		}
		got := Assign(n, cap, forb)
		checkValid(t, n, cap, forb, got)
		again := Assign(n, cap, forb)
		if !slices.Equal(got, again) {
			t.Fatalf("non-deterministic result: %v then %v", got, again)
		}
	}
}

// Worst-case shapes for the search at n = 14.
func TestLargeExtremes(t *testing.T) {
	// Complete graph: 14 rooms, forced vector.
	got := Assign(14, 14, completeForb(14))
	for i, r := range got {
		if r != i {
			t.Fatalf("complete graph: got %v", got)
		}
	}
	// Empty graph, capacity 2: 7 rooms filled in order.
	forb := make([]uint32, 14)
	got = Assign(14, 2, forb)
	want := []int{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Two disjoint 7-cliques, capacity 14: exactly 7 rooms.
	forb = make([]uint32, 14)
	for i := 0; i < 14; i++ {
		for j := 0; j < 14; j++ {
			if i != j && i/7 == j/7 {
				link(forb, i, j)
			}
		}
	}
	got = Assign(14, 14, forb)
	if roomCount(got) != 7 {
		t.Fatalf("two 7-cliques: got %v, want 7 rooms", got)
	}
	checkValid(t, 14, 14, forb, got)
}
