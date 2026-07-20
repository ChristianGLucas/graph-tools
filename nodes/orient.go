package nodes

import (
	"context"
	"sort"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Reinterprets a graph's edge direction, returning a plain Graph so it chains
// with no adapter. This is the bridge across the package's directed/undirected
// split: MinimumSpanningTree requires an undirected graph and TopologicalSort
// requires a directed one, so without this node a directed graph could never
// reach the former and an undirected graph could never reach the latter inside
// a flow.
//
// Converting to UNDIRECTED collapses any pair of opposing edges into a single
// edge carrying the smaller of the two weights — a directed a->b of 5 and
// b->a of 2 become one a-b of 2 — because an undirected graph cannot hold both
// and silently keeping an arbitrary one would be a guess.
//
// Converting to DIRECTED replaces each undirected edge with a pair of opposing
// edges, which preserves reachability exactly. Note that this makes the result
// cyclic by construction, so a topological sort of it will report is_dag=false
// unless the original had no edges.
//
// A graph that already has the requested direction is returned unchanged.
// Self-loops and vertex labels are preserved in every case.
func Orient(ctx context.Context, ax axiom.Context, input *gen.OrientRequest) (*gen.Graph, error) {
	if input == nil {
		return nil, errRequestRequired()
	}
	b, err := buildGraph(input.Graph)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := &gen.Graph{Directed: input.Directed}
	for _, id := range b.ids {
		out.Nodes = append(out.Nodes, &gen.GraphNode{Id: id, Label: b.labelOf[id]})
	}

	// Unchanged direction: copy the edges through verbatim.
	if b.directed == input.Directed {
		for _, e := range input.Graph.Edges {
			out.Edges = append(out.Edges, cloneEdge(e))
		}
		return out, nil
	}

	if input.Directed {
		// Undirected -> directed: each edge becomes an opposing pair. A
		// self-loop is already its own reverse, so it is emitted once.
		for _, e := range input.Graph.Edges {
			out.Edges = append(out.Edges, cloneEdge(e))
			if e.From != e.To {
				rev := cloneEdge(e)
				rev.From, rev.To = e.To, e.From
				out.Edges = append(out.Edges, rev)
			}
		}
		sortEdges(out.Edges)
		return out, nil
	}

	// Directed -> undirected: collapse opposing pairs, keeping the smaller
	// weight so the result is deterministic and never loses the cheaper route.
	type key struct{ a, b string }
	best := map[key]*gen.GraphEdge{}
	var order []key
	for _, e := range input.Graph.Edges {
		k := key{e.From, e.To}
		if e.To < e.From {
			k = key{e.To, e.From}
		}
		cand := cloneEdge(e)
		cand.From, cand.To = k.a, k.b
		prev, seen := best[k]
		if !seen {
			best[k] = cand
			order = append(order, k)
			continue
		}
		if resolveWeight(cand.Weight, cand.ExplicitZeroWeight) <
			resolveWeight(prev.Weight, prev.ExplicitZeroWeight) {
			best[k] = cand
		}
	}
	for _, k := range order {
		out.Edges = append(out.Edges, best[k])
	}
	sortEdges(out.Edges)
	return out, nil
}

func cloneEdge(e *gen.GraphEdge) *gen.GraphEdge {
	return &gen.GraphEdge{
		From:               e.From,
		To:                 e.To,
		Weight:             e.Weight,
		ExplicitZeroWeight: e.ExplicitZeroWeight,
	}
}

// sortEdges gives the emitted edge list a deterministic order.
func sortEdges(es []*gen.GraphEdge) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].From != es[j].From {
			return es[i].From < es[j].From
		}
		return es[i].To < es[j].To
	})
}
