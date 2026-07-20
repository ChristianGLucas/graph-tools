package nodes

import (
	"context"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Extracts the subgraph induced by a set of vertices: keeps exactly those
// vertices and every edge whose endpoints are both kept. The result is returned
// as a Graph so it can be piped straight into the other nodes in this package.
// A requested vertex id that is not in the source graph is rejected rather than
// silently ignored.
func Subgraph(ctx context.Context, ax axiom.Context, input *gen.SubgraphRequest) (*gen.SubgraphResult, error) {
	if input == nil {
		return &gen.SubgraphResult{Error: "request is required"}, nil
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return &gen.SubgraphResult{Error: err.Error()}, nil
	}

	keep := make(map[string]bool, len(input.Nodes))
	for _, id := range input.Nodes {
		if _, ok := b.idOf[id]; !ok {
			return &gen.SubgraphResult{Error: "unknown node id " + quote(id)}, nil
		}
		keep[id] = true
	}

	out := &gen.Graph{Directed: input.Graph.Directed}
	for _, id := range b.ids {
		if keep[id] {
			out.Nodes = append(out.Nodes, &gen.GraphNode{Id: id, Label: b.labelOf[id]})
		}
	}
	for _, e := range input.Graph.Edges {
		if keep[e.From] && keep[e.To] {
			out.Edges = append(out.Edges, &gen.GraphEdge{
				From:               e.From,
				To:                 e.To,
				Weight:             e.Weight,
				ExplicitZeroWeight: e.ExplicitZeroWeight,
			})
		}
	}

	return &gen.SubgraphResult{
		Graph:            out,
		DroppedNodeCount: int32(len(b.ids) - len(out.Nodes)),
		DroppedEdgeCount: int32(b.edgeCount - len(out.Edges)),
	}, nil
}
