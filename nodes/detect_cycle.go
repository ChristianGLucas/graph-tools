package nodes

import (
	"context"
	"math"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"

	"christiangeorgelucas/graph-tools/axiom"
	gen "christiangeorgelucas/graph-tools/gen"
)

// Reports whether a graph contains a cycle and returns one concrete example.
// For a directed graph a cycle is a strongly connected component with more than
// one vertex (or a self-loop), and cycle_count is the number of such components
// PLUS the number of self-loops. For an undirected graph cycle_count is the circuit rank
// |E| - |V| + (number of components), the number of independent cycles. The
// returned example cycle repeats its first vertex id at the end to close it.
func DetectCycle(ctx context.Context, ax axiom.Context, input *gen.Graph) (*gen.CycleResult, error) {
	b, err := buildGraph(input)
	if err != nil {
		return &gen.CycleResult{Error: err.Error()}, nil
	}
	if err := ctx.Err(); err != nil {
		return &gen.CycleResult{Error: "cancelled: " + err.Error()}, nil
	}

	out := &gen.CycleResult{}

	// A self-loop is the shortest possible cycle and is always reported first.
	selfLoopNode := b.firstSelfLoop(input)

	if b.directed {
		cyclic := 0
		var best []string
		for _, comp := range topo.TarjanSCC(b.dg) {
			if len(comp) > 1 {
				cyclic++
				names := b.sortedNames(comp)
				if best == nil || names[0] < best[0] {
					best = names
				}
			}
		}
		out.CycleCount = int32(cyclic + b.selfLoops)
		out.HasCycle = out.CycleCount > 0
		if selfLoopNode != "" {
			out.Cycle = []string{selfLoopNode, selfLoopNode}
		} else if best != nil {
			out.Cycle = b.directedCycleWitness(best)
		}
		return out, nil
	}

	// Undirected: circuit rank = |E| - |V| + components.
	nonLoopEdges := b.edgeCount - b.selfLoops
	comps := len(b.weakComponents())
	rank := nonLoopEdges - len(b.ids) + comps
	if rank < 0 {
		rank = 0
	}
	out.CycleCount = int32(rank + b.selfLoops)
	out.HasCycle = out.CycleCount > 0
	if selfLoopNode != "" {
		out.Cycle = []string{selfLoopNode, selfLoopNode}
	} else if rank > 0 {
		out.Cycle = b.undirectedCycleWitness()
	}
	return out, nil
}

// firstSelfLoop returns the lexicographically smallest vertex carrying a
// self-loop, or "" when there is none.
func (b *built) firstSelfLoop(g *gen.Graph) string {
	best := ""
	for _, e := range g.GetEdges() {
		if e.From == e.To && (best == "" || e.From < best) {
			best = e.From
		}
	}
	return best
}

// directedCycleWitness returns one elementary directed cycle inside a strongly
// connected component. It anchors on the component's smallest vertex u, uses a
// shortest (hop-count) path search from u, and closes the cycle through the
// in-neighbour of u that is nearest to u. Because the search is unweighted the
// path is simple, so appending u yields an elementary cycle.
func (b *built) directedCycleWitness(comp []string) []string {
	in := make(map[string]bool, len(comp))
	for _, id := range comp {
		in[id] = true
	}
	sub := simple.NewDirectedGraph()
	for _, id := range comp {
		sub.AddNode(simple.Node(b.idOf[id]))
	}
	for _, id := range comp {
		it := b.directedView().From(b.idOf[id])
		for it.Next() {
			if in[b.nameOf[it.Node().ID()]] {
				sub.SetEdge(simple.Edge{F: simple.Node(b.idOf[id]), T: simple.Node(it.Node().ID())})
			}
		}
	}

	u := b.idOf[comp[0]]
	sp := path.DijkstraFrom(simple.Node(u), orderedD{sub})

	var best []graph.Node
	bestW := math.Inf(1)
	it := orderedD{sub}.To(u)
	for it.Next() {
		y := it.Node().ID()
		p, w := sp.To(y)
		if len(p) == 0 {
			continue
		}
		if w < bestW || (w == bestW && best != nil && b.nameOf[y] < b.nameOf[best[len(best)-1].ID()]) {
			best, bestW = p, w
		}
	}
	if best == nil {
		return nil
	}
	cycle := b.names(best)
	return append(cycle, comp[0])
}

// undirectedCycleWitness returns one elementary undirected cycle. It builds a
// spanning forest with Kruskal's algorithm, picks an input edge the forest did
// not take (adding such an edge to a forest always closes exactly one cycle),
// and walks the unique tree path between that edge's endpoints.
func (b *built) undirectedCycleWitness() []string {
	// Kruskal populates the destination with every vertex itself; pre-adding
	// them would make it panic.
	forest := simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	path.Kruskal(forest, b.weightedUndirectedView())

	var extras []edgePair
	edges := b.weightedUndirectedView().WeightedEdges()
	for edges.Next() {
		e := edges.WeightedEdge()
		uid, vid := e.From().ID(), e.To().ID()
		if forest.HasEdgeBetween(uid, vid) {
			continue
		}
		u, v := b.nameOf[uid], b.nameOf[vid]
		if v < u {
			u, v = v, u
		}
		extras = append(extras, edgePair{u, v})
	}
	if len(extras) == 0 {
		return nil
	}
	sortPairs(extras)
	chosen := extras[0]

	// The witness needs only the UNIQUE tree path between the endpoints, so
	// edge weights are irrelevant — and running Dijkstra over the weighted
	// forest PANICS ("dijkstra: negative edge weight") whenever the graph has a
	// negative weight, which is a supported input class. Search an unweighted
	// copy of the forest instead, where every edge costs 1.
	plain := simple.NewUndirectedGraph()
	for _, id := range b.ids {
		plain.AddNode(simple.Node(b.idOf[id]))
	}
	fe := forest.Edges()
	for fe.Next() {
		e := fe.Edge()
		plain.SetEdge(simple.Edge{F: simple.Node(e.From().ID()), T: simple.Node(e.To().ID())})
	}

	sp := path.DijkstraFrom(simple.Node(b.idOf[chosen.a]), orderedU{plain})
	p, _ := sp.To(b.idOf[chosen.b])
	if len(p) == 0 {
		return nil
	}
	cycle := b.names(p)
	return append(cycle, chosen.a)
}
