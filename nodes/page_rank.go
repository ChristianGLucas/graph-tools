package nodes

import (
	"context"
	"sort"

	"gonum.org/v1/gonum/graph/network"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Ranks vertices by link importance using the PageRank power iteration: a
// vertex is important when important vertices point at it. Scores sum to 1 and
// are rounded to 6 decimal places, which makes the result reproducible despite
// the underlying iteration starting from a randomly seeded vector. An
// undirected graph is treated as a directed graph with each edge present in
// both directions. Edge weights are ignored — PageRank here is computed on the
// link topology only.
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
		return &gen.PageRankResult{Error: "graph exceeds the PageRank node limit"}, nil
	}

	damping := input.Damping
	if damping == 0 {
		damping = 0.85
	}
	if damping <= 0 || damping >= 1 {
		return &gen.PageRankResult{Error: "damping must be strictly between 0 and 1"}, nil
	}
	// The tolerance is pinned rather than caller-supplied: gonum seeds the
	// power iteration with a random vector, so the converged result varies
	// within the tolerance. Converging to 1e-14 (residual error under ~1e-13
	// for any admissible damping) and then rounding to 6 decimal places puts
	// the rounding granularity seven orders of magnitude above the noise, so
	// the emitted scores are reproducible.
	const pageRankTolerance = 1e-14
	const pageRankDecimals = 6

	scores := network.PageRank(b.directedView(), damping, pageRankTolerance)

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
