package storage

// canonicalAssignment returns, among all valid partitions using at most k
// rooms, the lexicographically smallest room-number vector a[0..n-1], with
// vertices processed in ascending id byte order and rooms introduced in
// order 0,1,... .
//
// Symmetry breaking: vertex i may only take a room label in
// 0..1+max(a[0..i-1]) (a "restricted growth string"). Every unlabeled
// partition has exactly one such labeling, so the search enumerates each
// partition exactly once instead of once per room permutation. Trying
// labels in ascending order makes the first solution found the
// lexicographically smallest one.
//
// Feasibility is decided by exact exhaustive backtracking: the caller
// invokes this for k = lowerBound, lowerBound+1, ... and the first
// success is therefore the proven minimum room count. No greedy result
// is ever returned.
func canonicalAssignment(n int, adj []uint16, capacity, k int) ([]int, bool) {
	color := make([]int, n)
	for i := range color {
		color[i] = -1
	}
	roomMask := make([]uint16, k)
	roomSize := make([]int, k)
	used := 0

	var dfs func(depth int) bool
	dfs = func(depth int) bool {
		if depth == n {
			return true
		}

		// Forward check: every uncolored vertex must still have at
		// least one room it can enter (an existing compatible room with
		// spare capacity, or a fresh room while fewer than k rooms
		// exist).
		for i := depth; i < n; i++ {
			opts := 0
			if used < k {
				opts = 1
			}
			for r := 0; r < used; r++ {
				if roomSize[r] < capacity && adj[i]&roomMask[r] == 0 {
					opts++
				}
			}
			if opts == 0 {
				return false
			}
		}

		v := depth

		// Equivalent-room symmetry pruning: two existing rooms that
		// have the same size and the same conflict set against the
		// uncolored vertices are interchangeable for the remainder of
		// the search. Trying both would enumerate the same partition
		// twice; only the lower label is needed, and it also yields
		// the lexicographically smaller vector.
		seen := make(map[uint64]bool, used)
		tryRoom := func(r int) bool {
			color[v] = r
			roomMask[r] |= 1 << uint(v)
			roomSize[r]++
			if dfs(depth + 1) {
				// Keep the assignment: color describes the solution.
				return true
			}
			roomSize[r]--
			roomMask[r] &^= 1 << uint(v)
			color[v] = -1
			return false
		}
		for r := 0; r < used; r++ {
			if roomSize[r] >= capacity || adj[v]&roomMask[r] != 0 {
				continue
			}
			conflicts := uint32(0)
			for u := depth + 1; u < n; u++ {
				if adj[u]&roomMask[r] != 0 {
					conflicts |= 1 << uint(u)
				}
			}
			sig := uint64(roomSize[r])<<32 | uint64(conflicts)
			if seen[sig] {
				continue
			}
			seen[sig] = true
			if tryRoom(r) {
				return true
			}
		}
		if used < k {
			used++
			if tryRoom(used - 1) {
				return true
			}
			used--
		}
		return false
	}

	if dfs(0) {
		return color, true
	}
	return nil, false
}
