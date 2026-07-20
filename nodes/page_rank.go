package nodes

import (
	"context"
	"fmt"
	"math"
	"sort"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Ranks vertices by link importance using the PageRank power iteration: a
// vertex is important when important vertices point at it. Self-loops are
// honoured, since a self-loop is a genuine rank sink that changes every score.
// An undirected graph is treated as a directed graph with each edge present in
// both directions. Edge weights are ignored — the ranking is computed on the
// link topology only. Scores sum to 1 and are rounded to 6 decimal places. The
// power iteration runs from a fixed uniform start vector and accumulates in a
// fixed order, so the result is bit-for-bit reproducible before rounding as
// well as after — see pagerank_iter.go for why the recurrence is run here
// rather than by gonum.
func PageRank(ctx context.Context, ax axiom.Context, input *gen.PageRankRequest) (*gen.PageRankResult, error) {
	if input == nil {
		return &gen.PageRankResult{Error: "request is required"}, nil
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return &gen.PageRankResult{Error: err.Error()}, nil
	}
	if len(b.ids) == 0 {
		return &gen.PageRankResult{Error: "graph has no nodes"}, nil
	}
	if len(b.ids) > maxPageRankNodes {
		return &gen.PageRankResult{Error: fmt.Sprintf(
			"graph has %d nodes, exceeding the PageRank limit of %d", len(b.ids), maxPageRankNodes)}, nil
	}

	damping := input.Damping
	// NaN must be rejected BEFORE the range check. Every comparison against a
	// NaN is false, so `damping <= 0 || damping >= 1` lets NaN through, and the
	// power iteration's convergence test then never becomes true — an infinite
	// loop driven by one caller-supplied field.
	if math.IsNaN(damping) || math.IsInf(damping, 0) {
		return &gen.PageRankResult{Error: "damping must be a finite value strictly between 0 and 1"}, nil
	}
	if damping == 0 {
		damping = 0.85
	}
	// The upper bound is 0.99, not 1. The power iteration needs roughly
	// log(tolerance)/log(damping) steps, so the cost grows without limit as
	// damping approaches 1: at 0.99 a full-size graph converges in ~0.3s, but
	// at 0.9999999999 it does not return at all — an unbounded CPU burn from a
	// single caller-supplied float.
	if damping <= 0 || damping > maxPageRankDamping {
		return &gen.PageRankResult{Error: fmt.Sprintf(
			"damping must be greater than 0 and at most %g; values nearer 1 make the power iteration converge arbitrarily slowly",
			maxPageRankDamping)}, nil
	}

	if err := ctx.Err(); err != nil {
		return &gen.PageRankResult{Error: "cancelled before computing PageRank: " + err.Error()}, nil
	}

	// The tolerance is pinned rather than caller-supplied: gonum seeds the
	// power iteration with a random vector, so the converged result varies
	// within the tolerance. Rounding to 6 decimal places afterwards puts the
	// rounding granularity far above the residual noise, so the emitted scores
	// are reproducible.
	//
	// 1e-12, not 1e-14: on a PERIODIC graph (a directed ring, say) the residual
	// oscillates and plateaus near the float64 noise floor, so a 1e-14 target
	// may never be met. 1e-12 is still six orders of magnitude finer than the
	// 6-decimal rounding applied below.
	//
	// The L1 residual is compared against this directly in deterministicPageRank.
	const pageRankTolerance = 1e-13 // per vertex; see deterministicPageRank
	const pageRankDecimals = 6

	// deterministicPageRank, not gonum's network.PageRank/PageRankSparse: gonum
	// seeds its iteration from a random vector, which made this node's output
	// nondeterministic at rounding boundaries. The in-package iteration is also
	// sparse — O(V+E) per step — so the dense-matrix blow-up gonum's PageRank
	// would incur at the vertex limit does not arise either.
	view := b.pageRankView(input.Graph)
	ids := make([]int64, 0, len(b.ids))
	for _, id := range b.ids {
		ids = append(ids, b.idOf[id])
	}

	type prResult struct {
		scores map[int64]float64
		err    error
	}
	res, budgetErr := runBounded(ctx, "PageRank", func() prResult {
		sc, err := deterministicPageRank(view, ids, damping, pageRankTolerance)
		return prResult{sc, err}
	})
	if budgetErr != nil {
		return &gen.PageRankResult{Error: budgetErr.Error()}, nil
	}
	if res.err != nil {
		return &gen.PageRankResult{Error: res.err.Error()}, nil
	}
	scores := res.scores

	out := &gen.PageRankResult{Damping: damping}
	for _, id := range b.ids {
		out.Scores = append(out.Scores, &gen.PageRankScore{
			Node:  id,
			Score: roundTo(scores[b.idOf[id]], pageRankDecimals),
		})
	}
	sort.Slice(out.Scores, func(i, j int) bool { return out.Scores[i].Node < out.Scores[j].Node })
	return out, nil
}
