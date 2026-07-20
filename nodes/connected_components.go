package nodes

import (
	"context"
	"sort"

	"gonum.org/v1/gonum/graph/topo"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Partitions a graph into groups of mutually-reachable vertices. For an
// undirected graph these are the connected components; for a directed graph
// they are the strongly connected components (Tarjan's algorithm), and
// strongly_connected is set to true. Components and their members are sorted
// by vertex id.
func ConnectedComponents(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.ComponentsResult, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.ComponentsResult{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.ComponentsResult{Error: "cancelled: " + err.Error()}, nil
	}

	var groups [][]string
	if b.directed {
		for _, comp := range topo.TarjanSCC(b.directedView()) {
			groups = append(groups, b.sortedNames(comp))
		}
		sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	} else {
		groups = b.weakComponents()
	}

	out := &gen.ComponentsResult{
		Count:             int32(len(groups)),
		StronglyConnected: b.directed,
	}
	for _, g := range groups {
		out.Components = append(out.Components, &gen.Component{Nodes: g})
	}
	return out, nil
}
