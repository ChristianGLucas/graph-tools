package nodes

import (
	"context"
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

	// The reconstruction below is memoised (see distancesFrom) and so is
	// near-linear, but it still runs under the shared wall-clock budget as a
	// backstop: it was O(V * diameter) before, and a 20000-vertex chain — barely
	// 380 KB of input — cost seconds and churned gigabytes.
	out, budgetErr := runBounded(ctx, "Distances", func() *gen.DistancesResult {
		return distancesFrom(b, sp)
	})
	if budgetErr != nil {
		return &gen.DistancesResult{Error: budgetErr.Error()}, nil
	}
	return out, nil
}

// distancesFrom turns a computed shortest-path tree into the result message.
//
// Hop counts are memoised across vertices, which is what keeps this near-linear
// rather than O(V * diameter). Every PREFIX of a shortest path is itself a
// shortest path to its own endpoint, so one call to sp.To(v) yields the hop
// count of every vertex along that path, not just v's. Resolving vertices that
// are still unknown therefore walks each tree edge once in total. Without this,
// a 20000-vertex chain — barely 380 KB of input — cost seconds and churned
// gigabytes, because each of the 20000 paths was materialised in full.
func distancesFrom(b *built, sp path.Shortest) *gen.DistancesResult {
	out := &gen.DistancesResult{}
	hops := make(map[int64]int32, len(b.ids))

	// Resolve the FARTHEST vertices first. Each sp.To() call costs the length of
	// the path it returns, and memoising only pays off if the long paths are
	// walked first: on a chain, the single farthest vertex yields the whole
	// spine in one call and every other vertex is then already known. Ascending
	// id order would instead walk lengthening prefixes over and over, which is
	// what made this quadratic.
	order := make([]int64, 0, len(b.ids))
	for _, id := range b.ids {
		order = append(order, b.idOf[id])
	}
	sort.Slice(order, func(i, j int) bool {
		wi, wj := sp.WeightTo(order[i]), sp.WeightTo(order[j])
		if wi != wj {
			// Descending distance; unreachable vertices (+Inf) sort first and
			// cost nothing, since their path is empty.
			return wi > wj
		}
		return order[i] < order[j] // stable, deterministic tie-break
	})

	for _, vid := range order {
		if _, known := hops[vid]; known {
			continue
		}
		p, _ := sp.To(vid)
		if len(p) == 0 {
			continue // unreachable; recorded below
		}
		for i, node := range p {
			if _, seen := hops[node.ID()]; !seen {
				hops[node.ID()] = int32(i)
			}
		}
	}

	for _, id := range b.ids {
		vid := b.idOf[id]
		h, reachable := hops[vid]
		// Unreachable is signalled by an EMPTY path; buildGraph has already
		// ruled out weight overflow as a cause.
		if !reachable {
			out.Unreachable = append(out.Unreachable, id)
			continue
		}
		out.Distances = append(out.Distances, &gen.Distance{
			Node:     id,
			Weight:   sp.WeightTo(vid),
			HopCount: h,
		})
	}
	sort.Slice(out.Distances, func(i, j int) bool { return out.Distances[i].Node < out.Distances[j].Node })
	sort.Strings(out.Unreachable)
	return out
}
