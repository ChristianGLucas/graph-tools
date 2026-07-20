package nodes

import (
	"context"
	"math"
	"sort"

	"gonum.org/v1/gonum/graph/path"

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

	sp, errMsg := b.shortestFrom(ctx, fromID)
	if errMsg != "" {
		return &gen.DistancesResult{Error: errMsg}, nil
	}

	// The hop-count walk below is O(V+E) (see distancesFrom), but still runs
	// under the shared wall-clock budget as a backstop.
	out, budgetErr := runBounded(ctx, "Distances", func() *gen.DistancesResult {
		return distancesFrom(b, sp, fromID)
	})
	if budgetErr != nil {
		return &gen.DistancesResult{Error: budgetErr.Error()}, nil
	}
	return out, nil
}

// distancesFrom turns a computed shortest-path tree into the result message.
//
// Hop counts come from a breadth-first search over the SHORTEST-PATH DAG — the
// sub-graph of edges (u,v) satisfying dist[u] + w(u,v) == dist[v] — which is
// O(V+E) and depends on neither the graph's diameter nor its tie structure.
//
// The obvious implementation, calling sp.To(v) per vertex and reading len(path),
// is O(V * diameter) because each call materialises the whole path. Resolving
// the farthest vertices first and memoising helps only when distances are
// DISTINCT: give every edge an explicit zero weight, or hang many leaves off the
// end of a long chain, and every vertex ties, the ordering degenerates, and the
// quadratic behaviour returns — a 20000-vertex zero-weight chain cost 3.8s and
// churned 14 GB. The DAG walk has no such branch.
//
// The float equality test is exact by construction, not approximate: the search
// computed dist[v] AS dist[u] + w(u,v) for whichever predecessor it chose, and
// recomputing that same IEEE sum reproduces the same bits, so at least one
// in-edge always matches.
func distancesFrom(b *built, sp path.Shortest, source int64) *gen.DistancesResult {
	out := &gen.DistancesResult{}

	dist := make(map[int64]float64, len(b.ids))
	for _, id := range b.ids {
		vid := b.idOf[id]
		dist[vid] = sp.WeightTo(vid)
	}

	wg := b.weightedLister()
	hops := make(map[int64]int32, len(b.ids))
	hops[source] = 0
	queue := make([]int64, 0, len(b.ids))
	queue = append(queue, source)

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		it := wg.From(u) // ordered wrapper: deterministic neighbour order
		for it.Next() {
			v := it.Node().ID()
			if _, seen := hops[v]; seen {
				continue
			}
			w, ok := wg.Weight(u, v)
			if !ok {
				continue
			}
			if dist[u]+w == dist[v] {
				hops[v] = hops[u] + 1
				queue = append(queue, v)
			}
		}
	}

	for _, id := range b.ids {
		vid := b.idOf[id]
		h, reachable := hops[vid]
		// A vertex is unreachable exactly when the search never assigned it a
		// finite distance; buildGraph has already ruled out weight overflow as
		// a cause.
		if !reachable || math.IsInf(dist[vid], 1) {
			out.Unreachable = append(out.Unreachable, id)
			continue
		}
		out.Distances = append(out.Distances, &gen.Distance{
			Node:     id,
			Weight:   dist[vid],
			HopCount: h,
		})
	}
	sort.Slice(out.Distances, func(i, j int) bool { return out.Distances[i].Node < out.Distances[j].Node })
	sort.Strings(out.Unreachable)
	return out
}
