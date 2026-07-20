package nodes

import (
	"context"
	"sort"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/topo"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Orders the vertices of a directed acyclic graph so that every edge points
// from an earlier vertex to a later one — the standard dependency/build order.
// Ties are broken by vertex id so the ordering is deterministic. Reports
// is_dag=false with an empty order when the graph contains a cycle. Requires a
// directed graph.
func TopologicalSort(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.TopoSortResult, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.TopoSortResult{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.TopoSortResult{Error: "cancelled: " + err.Error()}, nil
	}
	if !b.directed {
		return &gen.TopoSortResult{Error: "TopologicalSort requires a directed graph; set `directed` to true"}, nil
	}
	if b.selfLoops > 0 {
		// A self-loop is a cycle, but it is excluded from the gonum structure,
		// so report it explicitly rather than returning a bogus ordering.
		return &gen.TopoSortResult{IsDag: false}, nil
	}

	// SortStabilized with a lexical tie-break makes the ordering a pure
	// function of the input rather than of gonum's internal iteration order.
	sorted, err := topo.SortStabilized(b.directedView(), func(nodes []graph.Node) {
		sort.Slice(nodes, func(i, j int) bool {
			return b.nameOf[nodes[i].ID()] < b.nameOf[nodes[j].ID()]
		})
	})
	if err != nil {
		return &gen.TopoSortResult{IsDag: false}, nil
	}
	return &gen.TopoSortResult{IsDag: true, Order: b.names(sorted)}, nil
}
