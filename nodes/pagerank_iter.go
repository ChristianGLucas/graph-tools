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
const (
	// pageRankMaxIterations is a hard backstop. The analytic rate needs about
	// log(tol)/log(damping) steps — under 3000 at the maximum permitted damping
	// of 0.99 — so a genuinely converging input never approaches this.
	pageRankMaxIterations = 5000

	// pageRankStagnationLimit accepts the result once the residual has stopped
	// improving for this many consecutive iterations, which means it has hit
	// the floating-point floor and further work cannot refine the answer.
	pageRankStagnationLimit = 20
)

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

	prevDiff := math.Inf(1)
	stagnant := 0

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

		// The convergence target is PER VERTEX, not a total. An L1 residual
		// summed over n vertices has a floating-point noise floor that grows
		// with n, so comparing it against a size-independent constant is
		// unreachable on large graphs: a 20000-vertex star plateaus around
		// 5e-11 and would never meet a fixed 1e-12 target, spinning out the
		// whole iteration cap and failing on a graph that has a perfectly good
		// answer. Scaling by n keeps the accuracy requirement fixed per score
		// — far finer than the 6-decimal rounding applied downstream.
		if diff <= tol*float64(n) {
			return collect(sorted, cur), nil
		}

		// Belt and braces: once the residual stops improving it has hit the
		// floating-point floor and further iterations cannot do better, so
		// accept rather than spin. Deterministic, like everything else here.
		if iter > 0 && diff >= prevDiff {
			stagnant++
			if stagnant >= pageRankStagnationLimit {
				return collect(sorted, cur), nil
			}
		} else {
			stagnant = 0
		}
		prevDiff = diff
	}

	return nil, fmt.Errorf(
		"PageRank did not converge within %d iterations", pageRankMaxIterations)
}

// collect maps the score vector back onto gonum ids.
func collect(sorted []int64, v []float64) map[int64]float64 {
	scores := make(map[int64]float64, len(sorted))
	for i, id := range sorted {
		scores[id] = v[i]
	}
	return scores
}
