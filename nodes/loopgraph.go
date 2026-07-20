package nodes

import (
	"sort"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/iterator"
	"gonum.org/v1/gonum/graph/simple"
)

// gonum's `simple` graphs PANIC on a self-edge, so the shared builder keeps
// self-loops out of them. That is correct for shortest paths, spanning trees
// and centrality, where a self-loop cannot change the answer — but it is WRONG
// for PageRank, where a self-loop is a rank sink that materially changes every
// score. loopDirected is a minimal directed graph that permits self-edges, so
// PageRank can see the real topology.
//
// Adjacency is held in sorted slices rather than maps, so every iterator is
// deterministic by construction and no ordering wrapper is needed.
type loopDirected struct {
	nodes []graph.Node
	out   map[int64][]graph.Node
	in    map[int64][]graph.Node
	has   map[[2]int64]bool
}

func newLoopDirected(ids []int64) *loopDirected {
	g := &loopDirected{
		out: make(map[int64][]graph.Node, len(ids)),
		in:  make(map[int64][]graph.Node, len(ids)),
		has: make(map[[2]int64]bool),
	}
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, id := range sorted {
		g.nodes = append(g.nodes, simple.Node(id))
	}
	return g
}

// setEdge adds a directed edge, including a self-edge when from == to.
func (g *loopDirected) setEdge(from, to int64) {
	if g.has[[2]int64{from, to}] {
		return
	}
	g.has[[2]int64{from, to}] = true
	g.out[from] = append(g.out[from], simple.Node(to))
	g.in[to] = append(g.in[to], simple.Node(from))
}

// finalise sorts every adjacency list so iteration order is total and stable.
func (g *loopDirected) finalise() {
	for _, m := range []map[int64][]graph.Node{g.out, g.in} {
		for k := range m {
			ns := m[k]
			sort.Slice(ns, func(i, j int) bool { return ns[i].ID() < ns[j].ID() })
			m[k] = ns
		}
	}
}

func (g *loopDirected) Node(id int64) graph.Node {
	i := sort.Search(len(g.nodes), func(i int) bool { return g.nodes[i].ID() >= id })
	if i < len(g.nodes) && g.nodes[i].ID() == id {
		return g.nodes[i]
	}
	return nil
}

func (g *loopDirected) Nodes() graph.Nodes {
	return iterator.NewOrderedNodes(append([]graph.Node(nil), g.nodes...))
}

func (g *loopDirected) From(id int64) graph.Nodes {
	return iterator.NewOrderedNodes(append([]graph.Node(nil), g.out[id]...))
}

func (g *loopDirected) To(id int64) graph.Nodes {
	return iterator.NewOrderedNodes(append([]graph.Node(nil), g.in[id]...))
}

func (g *loopDirected) HasEdgeBetween(xid, yid int64) bool {
	return g.has[[2]int64{xid, yid}] || g.has[[2]int64{yid, xid}]
}

func (g *loopDirected) HasEdgeFromTo(uid, vid int64) bool {
	return g.has[[2]int64{uid, vid}]
}

func (g *loopDirected) Edge(uid, vid int64) graph.Edge {
	if !g.has[[2]int64{uid, vid}] {
		return nil
	}
	return simple.Edge{F: simple.Node(uid), T: simple.Node(vid)}
}

var _ graph.Directed = (*loopDirected)(nil)

// pageRankView builds the directed topology PageRank should see: every edge of
// the graph INCLUDING self-loops, with an undirected graph expanded into edges
// in both directions.
func (b *built) pageRankView(g *graphInput) *loopDirected {
	ids := make([]int64, 0, len(b.ids))
	for _, id := range b.ids {
		ids = append(ids, b.idOf[id])
	}
	lg := newLoopDirected(ids)
	for _, e := range g.GetEdges() {
		from, to := b.idOf[e.From], b.idOf[e.To]
		lg.setEdge(from, to)
		if !b.directed && from != to {
			lg.setEdge(to, from)
		}
	}
	lg.finalise()
	return lg
}
