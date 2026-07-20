package nodes

import (
	"context"

	"gonum.org/v1/gonum/graph/topo"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Summarises the structure of a graph: vertex and edge counts, edge density,
// mean degree, self-loop count, connectivity, component count, and whether a
// directed graph is acyclic. Connectivity and component_count use weak
// connectivity (edge direction ignored) for directed input.
func Describe(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.GraphStats, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.GraphStats{Error: err.Error()}, nil
	}

	n := len(b.ids)
	m := b.edgeCount
	out := &gen.GraphStats{
		NodeCount:     int32(n),
		EdgeCount:     int32(m),
		Directed:      b.directed,
		SelfLoopCount: int32(b.selfLoops),
	}

	comps := b.weakComponents()
	out.ComponentCount = int32(len(comps))
	out.IsConnected = n > 0 && len(comps) == 1

	if n > 0 {
		if b.directed {
			out.AverageDegree = float64(m) / float64(n)
		} else {
			out.AverageDegree = 2 * float64(m) / float64(n)
		}
	}

	// Density counts only the simple (non-self-loop) edges a simple graph can
	// hold: n*(n-1) ordered pairs when directed, half that when undirected.
	if n > 1 {
		maxEdges := float64(n) * float64(n-1)
		if !b.directed {
			maxEdges /= 2
		}
		out.Density = float64(m-b.selfLoops) / maxEdges
	}

	if b.directed && b.selfLoops == 0 {
		if _, err := topo.Sort(b.directedView()); err == nil {
			out.IsDag = true
		}
	}

	return out, nil
}
