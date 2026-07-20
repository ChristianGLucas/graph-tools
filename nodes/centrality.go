package nodes

import (
	"context"
	"sort"

	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/path"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Scores every vertex by structural importance under the requested measure:
// "degree" (default, the number of incident edges), "betweenness" (how many
// shortest paths run through the vertex), "closeness" (inverse of the summed
// distance to all reachable vertices), "harmonic" (sum of inverse distances) or
// "eccentricity" (distance to the farthest reachable vertex). Every measure
// except "degree" needs an all-pairs shortest-path computation and therefore
// rejects graphs with negative weights or more than 600 vertices.
func Centrality(ctx context.Context, ax axiom.Context, input *gen.CentralityRequest) (*gen.CentralityResult, error) {
	if input == nil {
		return &gen.CentralityResult{Error: "request is required"}, nil
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return &gen.CentralityResult{Error: err.Error()}, nil
	}

	measure := input.Measure
	if measure == "" {
		measure = "degree"
	}

	scores := map[int64]float64{}
	switch measure {
	case "degree":
		for _, id := range b.ids {
			vid := b.idOf[id]
			n := 0
			it := b.directedView().From(vid)
			for it.Next() {
				n++
			}
			if !b.directed {
				scores[vid] = float64(n)
			} else {
				in := b.directedView().To(vid)
				for in.Next() {
					n++
				}
				scores[vid] = float64(n)
			}
		}
	case "betweenness", "closeness", "harmonic", "eccentricity":
		if err := b.requireQuadraticBudget(measure + " centrality"); err != nil {
			return &gen.CentralityResult{Error: err.Error()}, nil
		}
		if b.hasNeg {
			return &gen.CentralityResult{Error: measure + " centrality requires non-negative edge weights"}, nil
		}
		g := b.weightedGraph()
		p := path.DijkstraAllPaths(g)
		switch measure {
		case "betweenness":
			if b.weighted {
				scores = network.BetweennessWeighted(b.weightedLister(), p)
			} else {
				scores = network.Betweenness(g)
			}
		case "closeness":
			scores = network.Closeness(g, p)
		case "harmonic":
			scores = network.Harmonic(g, p)
		case "eccentricity":
			scores = network.Eccentricity(g, p)
		}
	default:
		return &gen.CentralityResult{Error: "unknown measure " + quote(input.Measure) +
			"; expected one of: degree, betweenness, closeness, harmonic, eccentricity"}, nil
	}

	out := &gen.CentralityResult{Measure: measure}
	for _, id := range b.ids {
		// gonum omits zero-valued entries from some maps; a missing key is a
		// genuine zero score.
		out.Scores = append(out.Scores, &gen.CentralityScore{Node: id, Score: scores[b.idOf[id]]})
	}
	sort.Slice(out.Scores, func(i, j int) bool { return out.Scores[i].Node < out.Scores[j].Node })
	return out, nil
}
