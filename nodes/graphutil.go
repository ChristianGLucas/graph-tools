package nodes

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"

	gen "christiangeorgelucas/graph-tools/gen"
)

// graphInput is the canonical envelope every node in this package consumes.
type graphInput = gen.Graph

// Resource bounds. Every bound is checked against the RAW input message before
// any graph is allocated, so a hostile payload cannot drive unbounded work.
const (
	// maxNodes / maxEdges bound the linear and near-linear algorithms
	// (shortest path, topological sort, components, MST, PageRank, stats).
	maxNodes = 20000
	maxEdges = 200000

	// maxQuadraticNodes bounds the all-pairs measures (betweenness, closeness,
	// harmonic, eccentricity), whose time is O(V*E) and whose memory is O(V^2).
	maxQuadraticNodes = 600

	// maxPageRankNodes bounds PageRank, whose power iteration is O(k*(V+E)).
	maxPageRankNodes = 20000
)

// built is the validated, canonical in-memory form of a gen.Graph.
//
// Node ids are mapped to gonum's int64 ids by sorting the caller's string ids
// lexicographically and assigning 0..n-1. That mapping is a pure function of
// the input, which is what makes every node's output deterministic: gonum
// iterates by int64 id, so a stable id assignment gives a stable result.
type built struct {
	directed  bool
	ids       []string          // caller ids, sorted ascending
	idOf      map[string]int64  // caller id -> gonum id
	nameOf    map[int64]string  // gonum id -> caller id
	labelOf   map[string]string // caller id -> label
	selfLoops int               // count of edges whose endpoints are equal
	edgeCount int               // total edges in the input, including self-loops
	hasNeg    bool              // true when any resolved edge weight is negative
	weighted  bool              // true when any resolved edge weight differs from 1

	// wdg/wug hold the non-self-loop edges with their weights. Exactly one is
	// non-nil, matching `directed`.
	wdg *simple.WeightedDirectedGraph
	wug *simple.WeightedUndirectedGraph

	// dg is an unweighted directed view used by the topology-only algorithms
	// (topological sort, SCC, PageRank). For undirected input each edge is
	// present in both directions.
	dg *simple.DirectedGraph

	// ug is an unweighted undirected view (direction discarded), used for weak
	// connectivity.
	ug *simple.UndirectedGraph
}

// resolveWeight applies the documented default: an omitted/zero weight means
// 1.0 unless the caller explicitly opted into a literal zero.
func resolveWeight(w float64, explicitZero bool) float64 {
	if w == 0 && !explicitZero {
		return 1
	}
	return w
}

// buildGraph validates a caller-supplied graph and converts it into gonum
// structures. It returns a structured error (never a panic) for every form of
// malformed input.
func buildGraph(g *graphInput) (*built, error) {
	if g == nil {
		return nil, fmt.Errorf("graph is required")
	}
	// Bounds are enforced on the RAW input, before any allocation.
	if len(g.Nodes) > maxNodes {
		return nil, fmt.Errorf("graph has %d nodes, exceeding the limit of %d", len(g.Nodes), maxNodes)
	}
	if len(g.Edges) > maxEdges {
		return nil, fmt.Errorf("graph has %d edges, exceeding the limit of %d", len(g.Edges), maxEdges)
	}

	b := &built{
		directed: g.Directed,
		idOf:     make(map[string]int64, len(g.Nodes)),
		nameOf:   make(map[int64]string, len(g.Nodes)),
		labelOf:  make(map[string]string, len(g.Nodes)),
		ids:      make([]string, 0, len(g.Nodes)),
	}

	seen := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		if n == nil {
			return nil, fmt.Errorf("graph contains a null node entry")
		}
		if n.Id == "" {
			return nil, fmt.Errorf("node id must not be empty")
		}
		if seen[n.Id] {
			return nil, fmt.Errorf("duplicate node id %q", n.Id)
		}
		seen[n.Id] = true
		b.ids = append(b.ids, n.Id)
		b.labelOf[n.Id] = n.Label
	}

	// Deterministic id assignment.
	sort.Strings(b.ids)
	for i, id := range b.ids {
		b.idOf[id] = int64(i)
		b.nameOf[int64(i)] = id
	}

	b.dg = simple.NewDirectedGraph()
	b.ug = simple.NewUndirectedGraph()
	if g.Directed {
		b.wdg = simple.NewWeightedDirectedGraph(0, math.Inf(1))
	} else {
		b.wug = simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	}
	for _, id := range b.ids {
		n := simple.Node(b.idOf[id])
		b.dg.AddNode(n)
		b.ug.AddNode(n)
		if g.Directed {
			b.wdg.AddNode(n)
		} else {
			b.wug.AddNode(n)
		}
	}

	var weightMagnitude float64
	b.edgeCount = len(g.Edges)
	// edgeSeen rejects duplicate edges rather than silently letting the later
	// one overwrite the earlier one inside gonum's simple graph.
	type pair struct{ a, b int64 }
	edgeSeen := make(map[pair]bool, len(g.Edges))

	for _, e := range g.Edges {
		if e == nil {
			return nil, fmt.Errorf("graph contains a null edge entry")
		}
		fromID, ok := b.idOf[e.From]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %q in `from`", e.From)
		}
		toID, ok := b.idOf[e.To]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %q in `to`", e.To)
		}

		w := resolveWeight(e.Weight, e.ExplicitZeroWeight)
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return nil, fmt.Errorf("edge %q -> %q has a non-finite weight", e.From, e.To)
		}
		if w < 0 {
			b.hasNeg = true
		}
		// Any path/tree total is bounded by the sum of all edge magnitudes, so
		// checking that sum once here guarantees no downstream accumulation can
		// overflow to +Inf. Without this, individually-finite weights can sum
		// past float64 range and gonum's relaxation step then silently fails to
		// reach the node, which surfaces as a bogus "unreachable".
		weightMagnitude += math.Abs(w)
		if w != 1 {
			b.weighted = true
		}

		key := pair{fromID, toID}
		if !g.Directed && toID < fromID {
			key = pair{toID, fromID}
		}
		if edgeSeen[key] {
			return nil, fmt.Errorf("duplicate edge %q -> %q", e.From, e.To)
		}
		edgeSeen[key] = true

		if fromID == toID {
			// gonum's simple graphs panic on self-edges. Self-loops are
			// tracked separately: they are reported by Describe and DetectCycle
			// and are irrelevant to shortest paths, spanning trees and
			// centrality, so they are excluded from the gonum structures.
			b.selfLoops++
			continue
		}

		fn, tn := simple.Node(fromID), simple.Node(toID)
		b.dg.SetEdge(simple.Edge{F: fn, T: tn})
		b.ug.SetEdge(simple.Edge{F: fn, T: tn})
		if !g.Directed {
			b.dg.SetEdge(simple.Edge{F: tn, T: fn})
			b.wug.SetWeightedEdge(simple.WeightedEdge{F: fn, T: tn, W: w})
		} else {
			b.wdg.SetWeightedEdge(simple.WeightedEdge{F: fn, T: tn, W: w})
		}
	}

	if math.IsInf(weightMagnitude, 0) || math.IsNaN(weightMagnitude) {
		return nil, fmt.Errorf("the edge weights sum to a magnitude that overflows a 64-bit float; reduce the edge weights")
	}

	return b, nil
}

// weightedGraph returns the weighted structure matching the graph's direction,
// as the narrowest interface the path algorithms need.
func (b *built) weightedGraph() graph.Graph {
	if b.directed {
		return orderedWD{b.wdg}
	}
	return orderedWU{b.wug}
}

// directedView is the deterministic unweighted directed view.
func (b *built) directedView() orderedD { return orderedD{b.dg} }

// undirectedView is the deterministic unweighted undirected view.
func (b *built) undirectedView() orderedU { return orderedU{b.ug} }

// weightedUndirectedView is the deterministic weighted undirected view.
func (b *built) weightedUndirectedView() orderedWU { return orderedWU{b.wug} }

// names maps a gonum node path back to caller ids.
func (b *built) names(path []graph.Node) []string {
	out := make([]string, 0, len(path))
	for _, n := range path {
		out = append(out, b.nameOf[n.ID()])
	}
	return out
}

// sortedNames maps and sorts a set of gonum nodes back to caller ids.
func (b *built) sortedNames(nodes []graph.Node) []string {
	out := b.names(nodes)
	sort.Strings(out)
	return out
}

// edgePair is an endpoint pair used to give emitted edge lists a deterministic
// order independent of gonum's internal map iteration.
type edgePair struct{ a, b string }

func sortPairs(s []edgePair) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].a != s[j].a {
			return s[i].a < s[j].a
		}
		return s[i].b < s[j].b
	})
}

// weightedLister exposes the graph as gonum's weighted interface, for the
// weight-aware centrality measures.
func (b *built) weightedLister() graph.Weighted {
	if b.directed {
		return orderedWD{b.wdg}
	}
	return orderedWU{b.wug}
}

// quote renders a caller-supplied id for inclusion in an error message.
func quote(s string) string { return strconv.Quote(s) }

// shortestFrom runs the single-source shortest-path algorithm appropriate for
// the graph's weights: Dijkstra when every weight is non-negative, Bellman-Ford
// otherwise. The second return value is a non-empty error message when the
// result is undefined (a reachable negative-weight cycle).
func (b *built) shortestFrom(from int64) (path.Shortest, string) {
	g := b.weightedGraph()
	src := simple.Node(from)
	if !b.hasNeg {
		return path.DijkstraFrom(src, g), ""
	}
	sp, ok := path.BellmanFordFrom(src, g)
	if !ok {
		return sp, "graph contains a negative-weight cycle reachable from the source; shortest paths are undefined"
	}
	return sp, ""
}

// requireQuadraticBudget guards the all-pairs measures.
func (b *built) requireQuadraticBudget(op string) error {
	if len(b.ids) > maxQuadraticNodes {
		return fmt.Errorf("%s requires an all-pairs computation and is limited to %d nodes; graph has %d",
			op, maxQuadraticNodes, len(b.ids))
	}
	return nil
}

// weakComponents returns the connected components of the underlying undirected
// graph (i.e. ignoring edge direction), as sets of caller ids.
func (b *built) weakComponents() [][]string {
	raw := topo.ConnectedComponents(b.undirectedView())
	out := make([][]string, 0, len(raw))
	for _, comp := range raw {
		out = append(out, b.sortedNames(comp))
	}
	// Deterministic component ordering: by each component's smallest member.
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// roundTo rounds x to the given number of decimal places. It is used to quantise
// away the last-digit noise of iterative numerical algorithms so that a node's
// output is reproducible for a given input.
func roundTo(x float64, decimals int) float64 {
	scale := math.Pow(10, float64(decimals))
	return math.Round(x*scale) / scale
}
