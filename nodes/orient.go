package nodes

import (
	"context"
	"fmt"
	"sort"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Flips a graph's edge direction — a directed graph in yields an undirected
// graph out, and vice versa. It takes and returns the plain Graph envelope, so
// it chains from and into every other Graph-shaped node with no adapter.
//
// This is the bridge across the package's directed/undirected split:
// MinimumSpanningTree requires an undirected graph and TopologicalSort requires
// a directed one, so without this node a directed graph could never reach the
// former and an undirected graph could never reach the latter inside a flow.
// Because the direction is read from the input graph's own `directed` field
// rather than from a separate parameter, the node needs no configuration and
// composes mid-flow.
//
// Directed -> UNDIRECTED collapses any pair of opposing edges into a single
// edge carrying the smaller of the two weights — a directed a->b of 5 and b->a
// of 2 become one a-b of 2 — because an undirected graph cannot hold both and
// silently keeping an arbitrary one would be a guess.
//
// Undirected -> DIRECTED replaces each undirected edge with a pair of opposing
// edges, which preserves reachability exactly. Note that this makes the result
// cyclic by construction, so a topological sort of it will report is_dag=false
// unless the original had no edges. Because this doubles the edge count, the
// node rejects an input whose DOUBLED edge count would exceed the same edge
// limit every other node in the package enforces, rather than emitting a graph
// its own siblings would refuse.
//
// Self-loops and vertex labels are preserved in every case.
func Orient(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.Graph, error) {
	b, err := buildGraph(input)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// The output direction is the opposite of the input's.
	out := &gen.Graph{Directed: !b.directed}
	for _, id := range b.ids {
		out.Nodes = append(out.Nodes, &gen.GraphNode{Id: id, Label: b.labelOf[id]})
	}

	if out.Directed {
		// Undirected -> directed: each edge becomes an opposing pair. A
		// self-loop is already its own reverse, so it is emitted once.
		//
		// Bound the OUTPUT, not just the input: a graph at the input edge limit
		// would otherwise emit one at twice the limit, which every downstream
		// node in this package rejects and which the transport eventually fails
		// opaquely. Checked before any edge is appended.
		projected := 2*b.edgeCount - b.selfLoops
		if projected > maxEdges {
			return nil, fmt.Errorf(
				"converting to directed doubles the edge count, producing %d edges and exceeding the limit of %d; the input has %d edges",
				projected, maxEdges, b.edgeCount)
		}
		for _, e := range input.Edges {
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
	for _, e := range input.Edges {
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
