package nodes

import (
	"context"
	"math"

	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Selects the cheapest set of edges that connects every vertex without forming
// a cycle, using Kruskal's algorithm. The result is returned as a Graph so it
// can be piped straight into the other nodes in this package. When the input is
// disconnected the result is a minimum spanning forest and component_count is
// greater than one. Requires an undirected graph.
func MinimumSpanningTree(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.SpanningTreeResult, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.SpanningTreeResult{Error: err.Error()}, nil
	}
	if b.directed {
		return &gen.SpanningTreeResult{Error: "MinimumSpanningTree requires an undirected graph; set `directed` to false"}, nil
	}

	// Kruskal panics if dst already holds nodes that exist in g; it populates
	// dst with every vertex itself, including isolated ones.
	dst := simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	total := path.Kruskal(dst, b.weightedUndirectedView())

	tree := &gen.Graph{Directed: false}
	for _, id := range b.ids {
		tree.Nodes = append(tree.Nodes, &gen.GraphNode{Id: id, Label: b.labelOf[id]})
	}
	var picked []edgePair
	edges := orderedWU{dst}.WeightedEdges()
	for edges.Next() {
		e := edges.WeightedEdge()
		u, v := b.nameOf[e.From().ID()], b.nameOf[e.To().ID()]
		if v < u {
			u, v = v, u
		}
		picked = append(picked, edgePair{u, v})
	}
	// Deterministic edge ordering in the emitted graph.
	sortPairs(picked)
	for _, p := range picked {
		w := b.wug.WeightedEdge(b.idOf[p.a], b.idOf[p.b]).Weight()
		tree.Edges = append(tree.Edges, &gen.GraphEdge{
			From:               p.a,
			To:                 p.b,
			Weight:             w,
			ExplicitZeroWeight: w == 0,
		})
	}

	return &gen.SpanningTreeResult{
		Tree:           tree,
		TotalWeight:    total,
		ComponentCount: int32(len(b.weakComponents())),
	}, nil
}
