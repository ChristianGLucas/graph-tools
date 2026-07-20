package nodes

import (
	"context"
	"math"
	"sort"

	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/path"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Scores every vertex by structural importance under the requested measure:
//
//	"degree"       — the number of incident edges (in plus out for a directed
//	                 graph); a self-loop counts 2, per the usual convention.
//	"betweenness"  — how many shortest paths run through the vertex.
//	"closeness"    — the reciprocal of the summed distance FROM every vertex
//	                 that can reach it (the standard incoming convention).
//	"eccentricity" — the distance to the farthest vertex this vertex can REACH
//	                 (the outgoing convention).
//	"harmonic"     — the sum of reciprocal distances FROM every vertex that can
//	                 reach it (the standard incoming convention).
//
// Defaults to "degree". Every measure except "degree" needs an all-pairs
// shortest-path computation and therefore rejects negative edge weights and
// graphs beyond the documented all-pairs size bounds. Scores are always finite:
// a vertex with no reachable peers scores 0 rather than infinity.
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
			if b.directed {
				in := b.directedView().To(vid)
				for in.Next() {
					n++
				}
			}
			// Self-loops live outside the gonum structures, so add them back
			// here. A self-loop contributes 2 either way: twice to an
			// undirected vertex's degree, and once each to its in- and
			// out-degree when directed.
			scores[vid] = float64(n + 2*b.selfLoopOf[id])
		}

	case "betweenness", "closeness", "harmonic", "eccentricity":
		if err := b.requireQuadraticBudget(measure + " centrality"); err != nil {
			return &gen.CentralityResult{Error: err.Error()}, nil
		}
		if b.hasNeg {
			return &gen.CentralityResult{Error: measure + " centrality requires non-negative edge weights"}, nil
		}
		if err := ctx.Err(); err != nil {
			return &gen.CentralityResult{Error: "cancelled before computing " + measure + ": " + err.Error()}, nil
		}
		g := b.weightedGraph()

		switch measure {
		case "betweenness":
			// Only the weighted form needs the all-pairs result. Computing it
			// for the unweighted form and then discarding it doubles the cost
			// of the most expensive measure in the package.
			if b.weighted {
				scores = network.BetweennessWeighted(b.weightedLister(), path.DijkstraAllPaths(g))
			} else {
				scores = network.Betweenness(g)
			}
		case "closeness":
			scores = network.Closeness(g, path.DijkstraAllPaths(g))
		case "harmonic":
			scores = network.Harmonic(g, path.DijkstraAllPaths(g))
		case "eccentricity":
			// gonum's Eccentricity uses INCOMING paths, i.e. max over u of
			// d(u,v). The documented and conventional meaning is the outgoing
			// one — how far this vertex can reach — so it is computed on the
			// TRANSPOSED graph, which turns every incoming path into an
			// outgoing one. For an undirected graph the transpose is the graph
			// itself, so this is a no-op there.
			tg := b.transposedWeightedGraph()
			scores = network.Eccentricity(tg, path.DijkstraAllPaths(tg))
		}

	default:
		return &gen.CentralityResult{Error: "unknown measure " + quote(input.Measure) +
			"; expected one of: degree, betweenness, closeness, harmonic, eccentricity"}, nil
	}

	out := &gen.CentralityResult{Measure: measure}
	for _, id := range b.ids {
		// gonum omits zero-valued entries from some of its maps, so a missing
		// key is a genuine zero. It also yields +Inf for a vertex with no
		// reachable peers (the closeness of an isolated vertex is 1/0). An
		// infinity is not representable as a JSON number — it serialises as the
		// STRING "Infinity" out of a double field — and would poison any
		// downstream arithmetic, so it is reported as 0, the standard
		// convention for a vertex with nothing to be close to.
		s := scores[b.idOf[id]]
		if math.IsInf(s, 0) || math.IsNaN(s) {
			s = 0
		}
		out.Scores = append(out.Scores, &gen.CentralityScore{Node: id, Score: s})
	}
	sort.Slice(out.Scores, func(i, j int) bool { return out.Scores[i].Node < out.Scores[j].Node })
	return out, nil
}
