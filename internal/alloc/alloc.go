// Package alloc computes exact minimum-room assignments of batches to
// storage rooms under pairwise exclusion constraints and a per-room
// capacity limit.
//
// Batches are referred to by index 0..n-1, already sorted by id in UTF-8
// byte order. A solution is a vector v where v[i] is the room of batch i.
// Rooms are numbered in order of first appearance (v is a restricted
// growth string), which quotients out the k! label symmetry of k rooms.
// Among all valid assignments the solver first minimizes the number of
// rooms and then returns the lexicographically smallest vector, so the
// result is optimal (never a greedy approximation) and reproducible.
package alloc

import "math/bits"

// Assign returns the optimal canonical room vector for n batches with the
// given per-room capacity. forb[i] has bit j set iff batches i and j are
// mutually exclusive. A solution with n rooms always exists, so Assign
// always returns a valid vector.
func Assign(n, capacity int, forb []uint32) []int {
	if capacity < 1 {
		capacity = 1
	}
	s := search{
		n:        n,
		cap:      capacity,
		forb:     forb,
		roomMem:  make([]uint32, n),
		roomSize: make([]int, n),
		assign:   make([]int, n),
	}
	// Lower bounds on the room count: total capacity and the largest set
	// of pairwise-exclusive batches (a clique in the exclusion graph).
	lower := (n + capacity - 1) / capacity
	if c := maxClique(forb); c > lower {
		lower = c
	}
	for k := lower; k <= n; k++ {
		if v, ok := s.solve(k); ok {
			return v
		}
	}
	// Unreachable: k == n is always feasible (one batch per room).
	panic("alloc: no feasible assignment")
}

type search struct {
	n, cap   int
	forb     []uint32
	roomMem  []uint32 // roomMem[r] = bitmask of batches currently in room r
	roomSize []int
	assign   []int
}

// solve searches assignments using at most k rooms in lexicographic order
// of the assignment vector and returns the first (smallest) one found.
func (s *search) solve(k int) ([]int, bool) {
	for r := range s.roomMem {
		s.roomMem[r] = 0
		s.roomSize[r] = 0
	}
	return s.dfs(0, 0, k)
}

// dfs assigns batches in sorted-index order. open is the number of rooms
// opened so far; a new room always receives the next free number, which
// keeps every explored vector in canonical (restricted growth) form.
func (s *search) dfs(pos, open, k int) ([]int, bool) {
	if pos == s.n {
		out := make([]int, s.n)
		copy(out, s.assign)
		return out, true
	}
	// Prune: the remaining batches must fit into the remaining free slots.
	free := (k - open) * s.cap
	for r := 0; r < open; r++ {
		free += s.cap - s.roomSize[r]
	}
	if free < s.n-pos {
		return nil, false
	}
	bit := uint32(1) << pos
	// Try existing rooms first, in ascending room-number order, so the
	// first complete vector found is the lexicographically smallest one.
	for r := 0; r < open; r++ {
		if s.roomSize[r] < s.cap && s.roomMem[r]&s.forb[pos] == 0 {
			s.roomMem[r] |= bit
			s.roomSize[r]++
			s.assign[pos] = r
			if v, ok := s.dfs(pos+1, open, k); ok {
				return v, true
			}
			s.roomMem[r] &^= bit
			s.roomSize[r]--
		}
	}
	if open < k {
		s.roomMem[open] = bit
		s.roomSize[open] = 1
		s.assign[pos] = open
		if v, ok := s.dfs(pos+1, open+1, k); ok {
			return v, true
		}
		s.roomMem[open] = 0
		s.roomSize[open] = 0
	}
	return nil, false
}

// maxClique returns the size of a maximum clique in the exclusion graph;
// it is a lower bound on the number of rooms required.
func maxClique(forb []uint32) int {
	var all uint32
	for i := range forb {
		all |= 1 << i
	}
	best := 0
	var rec func(size int, cand uint32)
	rec = func(size int, cand uint32) {
		if size+bits.OnesCount32(cand) <= best {
			return
		}
		if cand == 0 {
			best = size
			return
		}
		for cand != 0 {
			v := uint32(bits.TrailingZeros32(cand))
			cand &= cand - 1
			rec(size+1, cand&forb[v])
		}
	}
	rec(0, all)
	return best
}
