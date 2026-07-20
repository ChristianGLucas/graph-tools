package nodes

import (
	"context"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Extracts the subgraph induced by a set of vertices: keeps exactly those
// vertices and every edge whose endpoints are both kept. The result is returned
// as a plain Graph — the package's canonical envelope — so it chains straight
// into any graph-consuming node with an identity edge and no adapter. A
// requested vertex id that is not in the source graph is rejected rather than
// silently ignored; an empty selection yields an empty graph.
func Subgraph(ctx context.Context, ax axiom.Context, input *gen.SubgraphRequest) (*gen.Graph, error) {
	if input == nil {
		return nil, errRequestRequired()
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return nil, err
	}
	if len(input.Nodes) > maxNodes {
		return nil, errTooManySelected(len(input.Nodes), maxNodes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keep := make(map[string]bool, len(input.Nodes))
	for _, id := range input.Nodes {
		if _, ok := b.idOf[id]; !ok {
			return nil, errUnknownNode(id)
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
	return out, nil
}
