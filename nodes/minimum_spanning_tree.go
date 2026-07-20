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
// a cycle, using Kruskal's algorithm. The result is returned as a plain Graph —
// the package's canonical envelope — so it feeds straight into Describe,
// DetectCycle, ConnectedComponents or TopologicalSort with an identity edge and
// no adapter. Pipe it into Describe to recover the tree's total weight. When
// the input is disconnected the result is a minimum spanning forest rather than
// an error. Requires an undirected graph.
func MinimumSpanningTree(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.Graph, error) {
	b, err := buildGraph(input)
	if err != nil {
		// Graph carries no error field, so a rejected request surfaces as a Go
		// error and the platform reports it as a node failure.
		return nil, err
	}
	if b.directed {
		return nil, errUndirectedRequired("MinimumSpanningTree")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Kruskal panics if dst already holds nodes that exist in g; it populates
	// dst with every vertex itself, including isolated ones.
	dst := simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	path.Kruskal(dst, b.weightedUndirectedView())

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
	return tree, nil
}
