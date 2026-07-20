package nodes

import (
	"context"

	"gonum.org/v1/gonum/graph/topo"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Summarises the structure of a graph: vertex and edge counts, edge density,
// mean degree, self-loop count, total edge weight, connectivity, component
// count, and whether a directed graph is acyclic. Connectivity and
// component_count use weak connectivity (edge direction ignored) for directed
// input. Piping a MinimumSpanningTree result in here is how you weigh the tree.
func Describe(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.GraphStats, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.GraphStats{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.GraphStats{Error: "cancelled: " + err.Error()}, nil
	}

	n := len(b.ids)
	m := b.edgeCount
	out := &gen.GraphStats{
		NodeCount:     int32(n),
		EdgeCount:     int32(m),
		Directed:      b.directed,
		SelfLoopCount: int32(b.selfLoops),
		TotalWeight:   b.totalWeight,
	}

	comps := b.weakComponents()
	out.ComponentCount = int32(len(comps))
	out.IsConnected = n > 0 && len(comps) == 1

	// Mean TOTAL degree, counting in+out for a directed graph. Summing every
	// vertex's degree gives 2*|E| either way, so this is 2|E|/|V| in both
	// cases — and it is exactly the mean of what Centrality's "degree" measure
	// reports per vertex, so the two nodes cannot disagree about a graph.
	if n > 0 {
		out.AverageDegree = 2 * float64(m) / float64(n)
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
