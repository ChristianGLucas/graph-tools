package nodes

import (
	"context"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Finds the lowest-cost path between two vertices of a graph and returns the
// ordered vertex ids, the total weight and the hop count. Uses Dijkstra's
// algorithm, automatically switching to Bellman-Ford when the graph contains
// negative edge weights; a negative-weight cycle is reported as an error.
func ShortestPath(ctx context.Context, ax axiom.Context, input *gen.ShortestPathRequest) (*gen.ShortestPathResult, error) {
	if input == nil {
		return &gen.ShortestPathResult{Error: "request is required"}, nil
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return &gen.ShortestPathResult{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.ShortestPathResult{Error: "cancelled: " + err.Error()}, nil
	}
	fromID, ok := b.idOf[input.From]
	if !ok {
		return &gen.ShortestPathResult{Error: "unknown source node id " + quote(input.From)}, nil
	}
	toID, ok := b.idOf[input.To]
	if !ok {
		return &gen.ShortestPathResult{Error: "unknown target node id " + quote(input.To)}, nil
	}

	sp, errMsg := b.shortestFrom(fromID)
	if errMsg != "" {
		return &gen.ShortestPathResult{Error: errMsg}, nil
	}

	p, w := sp.To(toID)
	// An unreachable target is signalled by an EMPTY path. buildGraph has
	// already guaranteed that no reachable total can overflow to +Inf, so an
	// empty path here means genuine unreachability and nothing else.
	if len(p) == 0 {
		return &gen.ShortestPathResult{Found: false}, nil
	}
	return &gen.ShortestPathResult{
		Found:       true,
		Path:        b.names(p),
		TotalWeight: w,
		HopCount:    int32(len(p) - 1),
	}, nil
}
