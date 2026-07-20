package nodes

import (
	"context"
	"sort"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Computes the shortest-path distance from one source vertex to every other
// vertex in the graph, returning the reachable vertices with their total cost
// and hop count, plus the list of vertices that cannot be reached. Uses
// Dijkstra's algorithm, switching to Bellman-Ford for graphs with negative
// edge weights.
func Distances(ctx context.Context, ax axiom.Context, input *gen.DistancesRequest) (*gen.DistancesResult, error) {
	if input == nil {
		return &gen.DistancesResult{Error: "request is required"}, nil
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return &gen.DistancesResult{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.DistancesResult{Error: "cancelled: " + err.Error()}, nil
	}
	fromID, ok := b.idOf[input.From]
	if !ok {
		return &gen.DistancesResult{Error: "unknown source node id " + quote(input.From)}, nil
	}

	sp, errMsg := b.shortestFrom(fromID)
	if errMsg != "" {
		return &gen.DistancesResult{Error: errMsg}, nil
	}

	out := &gen.DistancesResult{}
	for _, id := range b.ids {
		vid := b.idOf[id]
		p, w := sp.To(vid)
		// Unreachable is signalled by an EMPTY path; buildGraph has already
		// ruled out overflow as a cause.
		if len(p) == 0 {
			out.Unreachable = append(out.Unreachable, id)
			continue
		}
		out.Distances = append(out.Distances, &gen.Distance{
			Node:     id,
			Weight:   w,
			HopCount: int32(len(p) - 1),
		})
	}
	sort.Slice(out.Distances, func(i, j int) bool { return out.Distances[i].Node < out.Distances[j].Node })
	sort.Strings(out.Unreachable)
	return out, nil
}
