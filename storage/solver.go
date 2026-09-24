// Package storage implements exact optimal assignment of hazardous
// chemical batches to storage rooms.
//
// The problem is a constrained graph partition: every batch is a vertex,
// a mutual-exclusion pair is an edge, and each room is an independent set
// of size at most capacity. We must use the minimum number of rooms; among
// all optimum partitions we return the canonical one (rooms numbered in
// order of first appearance of their members in UTF-8 byte order of the
// batch ids), i.e. the lexicographically smallest room-number vector.
//
// The search never relies on greedy placement: the minimum room count is
// proven feasible by exhaustive backtracking, and symmetry over room
// labels is broken by construction.
package storage

// Solution is the canonical optimal partition of one request.
type Solution struct {
	// Assignment[v] is the canonical room number (0 based) of the v-th
	// batch, batches sorted by UTF-8 byte order of their ids.
	Assignment []int
	// Rooms[r] holds the batch ids (byte sorted) assigned to room r.
	Rooms [][]string
	// Occupancy[r] is len(Rooms[r]); provided explicitly for the API.
	Occupancy []int
	// RoomCount is the number of rooms used, i.e. the optimum.
	RoomCount int
}

// Solve finds the minimum-room partition.
//
// ids must be non-empty and unique; pairs are undirected edges given as
// indexes into ids and must be free of loops/duplicates; capacity >= 1.
func Solve(ids []string, pairs [][2]int, capacity int) Solution {
	n := len(ids)

	// Canonical vertex order: ascending UTF-8 bytes of the batch id.
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sortIndicesByID(order, ids)

	idAt := make([]int, n) // position in sorted order -> original index
	for pos, orig := range order {
		idAt[pos] = orig
	}

	// Remap caller vertex indexes to sorted positions.
	indexByOrig := make([]int, n)
	for pos, orig := range order {
		indexByOrig[orig] = pos
	}

	adj := make([]uint16, n) // adjacency bitmask over sorted vertices
	for _, p := range pairs {
		u, v := indexByOrig[p[0]], indexByOrig[p[1]]
		adj[u] |= 1 << uint(v)
		adj[v] |= 1 << uint(u)
	}

	lower := (n + capacity - 1) / capacity
	if c := cliqueNumber(n, adj); c > lower {
		lower = c
	}

	var assignment []int
	k := lower
	for ; k <= n; k++ {
		if a, ok := canonicalAssignment(n, adj, capacity, k); ok {
			assignment = a
			break
		}
	}

	roomIDs := make([][]string, k)
	for pos, room := range assignment {
		roomIDs[room] = append(roomIDs[room], ids[idAt[pos]])
	}
	// Batches inside every room are listed in byte order; vertices are
	// processed in byte order, so appending already yields that order.
	occupancy := make([]int, k)
	for r := range roomIDs {
		occupancy[r] = len(roomIDs[r])
	}

	// Re-index back to the caller's id slice order.
	out := make([]int, n)
	for pos, room := range assignment {
		out[idAt[pos]] = room
	}

	return Solution{
		Assignment: out,
		Rooms:      roomIDs,
		Occupancy:  occupancy,
		RoomCount:  k,
	}
}

// cliqueNumber computes the size of the largest clique by subset
// enumeration. With n <= 14 at most 2^14 * 14 work is done. A clique is a
// valid lower bound on the number of rooms since its vertices are pairwise
// mutually exclusive.
func cliqueNumber(n int, adj []uint16) int {
	best := 0
	var dfs func(mask uint16, size int)
	dfs = func(candidates uint16, size int) {
		if size > best {
			best = size
		}
		if candidates == 0 || size+popCount(candidates) <= best {
			return
		}
		// Choose a pivot maximizing P ∩ N(u) (Bron-Kerbosch pivot).
		var pivot uint16
		maxDeg := -1
		for bits := candidates; bits != 0; bits &= bits - 1 {
			v := bits & -bits
			i := bitIndex(v)
			deg := popCount(candidates & adj[i])
			if deg > maxDeg {
				maxDeg = deg
				pivot = v
			}
		}
		// Branch only on candidates not adjacent to the pivot.
		branch := candidates
		if pivot != 0 {
			branch = candidates &^ adj[bitIndex(pivot)]
		}
		for bits := branch; bits != 0; bits &= bits - 1 {
			v := bits & -bits
			i := bitIndex(v)
			dfs(candidates&adj[i], size+1)
			candidates &^= v
		}
	}
	dfs(uint16(1<<uint(n))-1, 0)
	return best
}

func sortIndicesByID(order []int, ids []string) {
	// Simple insertion sort: n <= 14.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && ids[order[j-1]] > ids[order[j]]; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
}

func popCount(x uint16) int {
	c := 0
	for x != 0 {
		x &= x - 1
		c++
	}
	return c
}

func bitIndex(bit uint16) int {
	i := 0
	for bit&1 == 0 {
		bit >>= 1
		i++
	}
	return i
}
