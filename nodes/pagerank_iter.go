package nodes

import (
	"fmt"
	"math"
	"sort"
)

// gonum's PageRank seeds its power iteration from a RANDOM vector
// (network/page.go calls rand.NormFloat64). The iteration converges to the same
// fixed point either way, but only to within the convergence tolerance — so the
// last digits of the raw scores differ between runs.
//
// Rounding the output was not a sufficient fix. Whenever a vertex's exact score
// lands on a rounding boundary the rounded value still flips between runs, and
// boundary values are common rather than exotic: a source or sink vertex scores
// exactly (1-d)/n, and simple rational combinations land on a trailing 5
// routinely. A four-vertex graph whose exact score is 0.0534375 was observed
// returning both 0.053437 and 0.053438 across repeated invocations of the
// deployed node.
//
// The only sound fix is to make the computation itself bit-for-bit repeatable,
// so this file runs the power iteration from a FIXED uniform start vector,
// accumulating in a fixed (sorted) order. gonum still owns every genuinely hard
// algorithm in this package — Dijkstra, Bellman-Ford, Tarjan, Kruskal, Brandes;
// the PageRank recurrence is a dozen lines of arithmetic and is reproduced here
// only to control its starting conditions. It is checked against
// network.PageRankSparse and against closed-form answers in the tests.
const pageRankMaxIterations = 20000

// deterministicPageRank returns the PageRank of g by power iteration from the
// uniform vector 1/n. `ids` must be the ascending gonum ids of g's vertices.
//
// The recurrence is the standard one, including the dangling-mass term: a
// vertex with no out-edges distributes its rank uniformly over every vertex,
// which is what keeps the scores summing to 1.
func deterministicPageRank(g *loopDirected, ids []int64, damping, tol float64) (map[int64]float64, error) {
	n := len(ids)
	if n == 0 {
		return map[int64]float64{}, nil
	}

	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	index := make(map[int64]int, n)
	for i, id := range sorted {
		index[id] = i
	}

	// out[i] holds the positions this vertex links to, in ascending order, so
	// every floating-point sum below accumulates in a fixed order.
	out := make([][]int, n)
	for i, id := range sorted {
		it := g.From(id)
		for it.Next() {
			out[i] = append(out[i], index[it.Node().ID()])
		}
		sort.Ints(out[i])
	}

	cur := make([]float64, n)
	next := make([]float64, n)
	for i := range cur {
		cur[i] = 1 / float64(n)
	}

	teleport := (1 - damping) / float64(n)

	for iter := 0; iter < pageRankMaxIterations; iter++ {
		// Rank held by vertices with no outgoing edges is redistributed evenly.
		var dangling float64
		for i := range cur {
			if len(out[i]) == 0 {
				dangling += cur[i]
			}
		}
		base := teleport + damping*dangling/float64(n)
		for i := range next {
			next[i] = base
		}
		for i := range cur {
			d := len(out[i])
			if d == 0 {
				continue
			}
			share := damping * cur[i] / float64(d)
			for _, j := range out[i] {
				next[j] += share
			}
		}

		var diff float64
		for i := range cur {
			diff += math.Abs(next[i] - cur[i])
		}
		cur, next = next, cur

		if diff < tol {
			scores := make(map[int64]float64, n)
			for i, id := range sorted {
				scores[id] = cur[i]
			}
			return scores, nil
		}
	}

	return nil, fmt.Errorf(
		"PageRank did not converge within %d iterations; try a smaller damping factor",
		pageRankMaxIterations)
}
