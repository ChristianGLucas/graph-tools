package nodes

import (
	"sort"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/iterator"
	"gonum.org/v1/gonum/graph/simple"
)

// gonum's simple graphs store their adjacency in Go maps, so Nodes(), From(),
// To() and Edges() yield elements in randomised order. Several gonum algorithms
// are order-sensitive in ways that are invisible until they bite:
//
//   - path.DijkstraFrom pushes neighbours onto a binary heap in From() order,
//     so two equal-cost paths are returned interchangeably between runs.
//   - path.Kruskal sorts edges with the non-stable slices.SortFunc, so equal
//     weights select different (equally minimal) spanning trees between runs.
//   - network.PageRank accumulates floating-point sums in Nodes() order.
//
// Every node in this package therefore drives gonum through these wrappers,
// which impose a total order (ascending gonum id, which by construction is
// ascending caller id) on every iterator. That makes each algorithm a pure
// function of the input message, which is what the package's determinism
// guarantee rests on.

func orderedNodes(it graph.Nodes) graph.Nodes {
	ns := graph.NodesOf(it)
	sort.Slice(ns, func(i, j int) bool { return ns[i].ID() < ns[j].ID() })
	return iterator.NewOrderedNodes(ns)
}

func orderedEdges(it graph.Edges) graph.Edges {
	es := graph.EdgesOf(it)
	sort.Slice(es, func(i, j int) bool {
		if es[i].From().ID() != es[j].From().ID() {
			return es[i].From().ID() < es[j].From().ID()
		}
		return es[i].To().ID() < es[j].To().ID()
	})
	return iterator.NewOrderedEdges(es)
}

func orderedWeightedEdges(it graph.WeightedEdges) graph.WeightedEdges {
	es := graph.WeightedEdgesOf(it)
	sort.Slice(es, func(i, j int) bool {
		if es[i].From().ID() != es[j].From().ID() {
			return es[i].From().ID() < es[j].From().ID()
		}
		return es[i].To().ID() < es[j].To().ID()
	})
	return iterator.NewOrderedWeightedEdges(es)
}

// ── weighted undirected ──────────────────────────────────────────────────────

type orderedWU struct{ *simple.WeightedUndirectedGraph }

func (g orderedWU) Nodes() graph.Nodes                  { return orderedNodes(g.WeightedUndirectedGraph.Nodes()) }
func (g orderedWU) From(id int64) graph.Nodes           { return orderedNodes(g.WeightedUndirectedGraph.From(id)) }
func (g orderedWU) Edges() graph.Edges                  { return orderedEdges(g.WeightedUndirectedGraph.Edges()) }
func (g orderedWU) WeightedEdges() graph.WeightedEdges {
	return orderedWeightedEdges(g.WeightedUndirectedGraph.WeightedEdges())
}

// ── weighted directed ────────────────────────────────────────────────────────

type orderedWD struct{ *simple.WeightedDirectedGraph }

func (g orderedWD) Nodes() graph.Nodes        { return orderedNodes(g.WeightedDirectedGraph.Nodes()) }
func (g orderedWD) From(id int64) graph.Nodes { return orderedNodes(g.WeightedDirectedGraph.From(id)) }
func (g orderedWD) To(id int64) graph.Nodes   { return orderedNodes(g.WeightedDirectedGraph.To(id)) }
func (g orderedWD) Edges() graph.Edges        { return orderedEdges(g.WeightedDirectedGraph.Edges()) }
func (g orderedWD) WeightedEdges() graph.WeightedEdges {
	return orderedWeightedEdges(g.WeightedDirectedGraph.WeightedEdges())
}

// ── unweighted directed ──────────────────────────────────────────────────────

type orderedD struct{ *simple.DirectedGraph }

func (g orderedD) Nodes() graph.Nodes        { return orderedNodes(g.DirectedGraph.Nodes()) }
func (g orderedD) From(id int64) graph.Nodes { return orderedNodes(g.DirectedGraph.From(id)) }
func (g orderedD) To(id int64) graph.Nodes   { return orderedNodes(g.DirectedGraph.To(id)) }
func (g orderedD) Edges() graph.Edges        { return orderedEdges(g.DirectedGraph.Edges()) }

// ── unweighted undirected ────────────────────────────────────────────────────

type orderedU struct{ *simple.UndirectedGraph }

func (g orderedU) Nodes() graph.Nodes        { return orderedNodes(g.UndirectedGraph.Nodes()) }
func (g orderedU) From(id int64) graph.Nodes { return orderedNodes(g.UndirectedGraph.From(id)) }
func (g orderedU) Edges() graph.Edges        { return orderedEdges(g.UndirectedGraph.Edges()) }

// Compile-time proof that each wrapper still satisfies the gonum interfaces the
// algorithms in this package require.
var (
	_ graph.WeightedUndirected = orderedWU{}
	_ graph.WeightedDirected   = orderedWD{}
	_ graph.Directed           = orderedD{}
	_ graph.Undirected         = orderedU{}
)
