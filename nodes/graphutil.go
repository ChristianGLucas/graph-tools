package nodes

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
	"google.golang.org/protobuf/proto"

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

	// maxPageRankNodes states the PageRank vertex limit explicitly rather than
	// leaving it implicit. It equals maxNodes because the in-package power
	// iteration (see pagerank_iter.go) is O(V+E) per step over adjacency lists:
	// at 20000 vertices a full run completes in tens of milliseconds. No dense
	// V*V matrix is ever allocated.
	maxPageRankNodes = maxNodes

	// maxPageRankDamping bounds the iteration count. PageRank needs about
	// log(tol)/log(damping) steps, which diverges as damping approaches 1:
	// 0.99 converges in ~0.3s on a full-size graph, 0.9999999999 never returns.
	maxPageRankDamping = 0.99

	// maxNegativeWeightNodes bounds the VERTEX count on the Bellman-Ford path. A
	// single negative edge weight switches the algorithm from Dijkstra to
	// Bellman-Ford, which the 20000-vertex cap sized for Dijkstra does not bound
	// at all: 20000 vertices and three edges forming a negative cycle cost ~106s
	// from a 200KB payload. Both this and the product cap below are needed —
	// neither dimension bounds the cost alone.
	maxNegativeWeightNodes = 2000

	// maxNegativeWeightProduct bounds the ACTUAL cost of the Bellman-Ford path.
	// gonum uses Bellman-Ford-Moore (SPFA): its `loops > V*(V-1)` guard caps
	// DEQUEUES, and each dequeue scans that vertex's out-edges — so the real
	// cost is O(V*E), and the edge count the vertex cap ignores is the dominant
	// term. 2000 vertices with 200000 edges measured over a minute; bounding
	// the product instead keeps the worst case near 0.2s.
	maxNegativeWeightProduct = 1200000

	// maxQuadraticNodes bounds the VERTEX count for the all-pairs measures
	// (betweenness, closeness, harmonic, eccentricity).
	maxQuadraticNodes = 600

	// maxQuadraticProduct bounds the actual COST of the all-pairs measures,
	// which is O(V*E) in time. Bounding V alone is not enough: a complete
	// 600-vertex graph has 179700 edges, which passes both the vertex and the
	// edge cap while costing over a minute of CPU. This product cap is what
	// keeps the worst admissible all-pairs input to roughly a second.
	maxQuadraticProduct = 1200000

	// maxIDBytes / maxLabelBytes bound the per-vertex string sizes. Counting
	// elements alone leaves the byte dimension unbounded: 20000 vertices with
	// 10 KiB ids is a 381 MiB payload that every element-based cap accepts.
	maxIDBytes    = 256
	maxLabelBytes = 1024

	// maxEncodedBytes bounds the whole request, as a backstop for any byte
	// dimension the per-field caps do not model.
	// Kept below the transport's own message ceiling (~4 MiB) so that this
	// node's structured error actually fires, rather than the caller getting an
	// opaque gateway failure before any node code runs.
	maxEncodedBytes = 3 << 20 // 3 MiB
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
	// selfLoopOf counts self-loops per vertex, so the degree measures can apply
	// the standard "a self-loop adds 2 to a vertex's degree" convention even
	// though self-loops are held outside the gonum structures.
	selfLoopOf  map[string]int
	totalWeight float64 // sum of every resolved edge weight
	edgeCount   int     // total edges in the input, including self-loops
	hasNeg      bool    // true when any resolved edge weight is negative
	weighted    bool    // true when any resolved edge weight differs from 1

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
	if n := proto.Size(g); n > maxEncodedBytes {
		return nil, fmt.Errorf("graph is %d bytes encoded, exceeding the limit of %d", n, maxEncodedBytes)
	}
	if len(g.Nodes) > maxNodes {
		return nil, fmt.Errorf("graph has %d nodes, exceeding the limit of %d", len(g.Nodes), maxNodes)
	}
	if len(g.Edges) > maxEdges {
		return nil, fmt.Errorf("graph has %d edges, exceeding the limit of %d", len(g.Edges), maxEdges)
	}

	b := &built{
		directed:   g.Directed,
		idOf:       make(map[string]int64, len(g.Nodes)),
		nameOf:     make(map[int64]string, len(g.Nodes)),
		labelOf:    make(map[string]string, len(g.Nodes)),
		selfLoopOf: make(map[string]int),
		ids:        make([]string, 0, len(g.Nodes)),
	}

	seen := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		if n == nil {
			return nil, fmt.Errorf("graph contains a null node entry")
		}
		if n.Id == "" {
			return nil, fmt.Errorf("node id must not be empty")
		}
		if len(n.Id) > maxIDBytes {
			return nil, fmt.Errorf("node id is %d bytes, exceeding the limit of %d", len(n.Id), maxIDBytes)
		}
		if i := indexControlChar(n.Id); i >= 0 {
			return nil, fmt.Errorf("node id %s contains a control character at byte %d", quote(n.Id), i)
		}
		if len(n.Label) > maxLabelBytes {
			return nil, fmt.Errorf("label on node %s is %d bytes, exceeding the limit of %d",
				quote(n.Id), len(n.Label), maxLabelBytes)
		}
		if seen[n.Id] {
			return nil, fmt.Errorf("duplicate node id %s", quote(n.Id))
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
		// Endpoint strings are caller-controlled and are echoed back in the
		// error below, so they are length-checked here too — the node-id check
		// above does not cover them.
		if len(e.From) > maxIDBytes || len(e.To) > maxIDBytes {
			return nil, fmt.Errorf("edge endpoint id exceeds the limit of %d bytes", maxIDBytes)
		}
		fromID, ok := b.idOf[e.From]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %s in `from`", quote(e.From))
		}
		toID, ok := b.idOf[e.To]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %s in `to`", quote(e.To))
		}

		if math.Signbit(e.Weight) && e.Weight == 0 && !e.ExplicitZeroWeight {
			return nil, fmt.Errorf("edge %s -> %s has a weight of negative zero; set `explicit_zero_weight` to mean a genuine zero cost",
				quote(e.From), quote(e.To))
		}
		w := resolveWeight(e.Weight, e.ExplicitZeroWeight)
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return nil, fmt.Errorf("edge %s -> %s has a non-finite weight", quote(e.From), quote(e.To))
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
		b.totalWeight += w
		if w != 1 {
			b.weighted = true
		}

		key := pair{fromID, toID}
		if !g.Directed && toID < fromID {
			key = pair{toID, fromID}
		}
		if edgeSeen[key] {
			return nil, fmt.Errorf("duplicate edge %s -> %s", quote(e.From), quote(e.To))
		}
		edgeSeen[key] = true

		if fromID == toID {
			// gonum's simple graphs panic on self-edges. Self-loops are
			// tracked separately: they are reported by Describe and DetectCycle
			// and are irrelevant to shortest paths, spanning trees and
			// centrality, so they are excluded from the gonum structures.
			b.selfLoops++
			b.selfLoopOf[e.From]++
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

// quote renders a caller-supplied id for inclusion in an error message. The
// value is truncated first: strconv.Quote expands binary bytes up to fourfold,
// so echoing an unbounded caller string back would amplify the response.
func quote(s string) string {
	const maxEchoBytes = 64
	if len(s) > maxEchoBytes {
		return strconv.Quote(s[:maxEchoBytes]) + "... (truncated)"
	}
	return strconv.Quote(s)
}

// indexControlChar returns the index of the first C0 control character in s, or
// -1. Control characters in an id are almost always a caller bug and would be
// carried silently into every emitted graph, so they are rejected.
func indexControlChar(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return i
		}
	}
	return -1
}

// shortestFrom runs the single-source shortest-path algorithm appropriate for
// the graph's weights: Dijkstra when every weight is non-negative, Bellman-Ford
// otherwise. The second return value is a non-empty error message when the
// result is undefined (a reachable negative-weight cycle).
func (b *built) shortestFrom(ctx context.Context, from int64) (path.Shortest, string) {
	g := b.weightedGraph()
	src := simple.Node(from)
	if !b.hasNeg {
		// Dijkstra is O(E log V) and fully bounded by the global input caps —
		// under half a second at the largest admissible payload.
		return path.DijkstraFrom(src, g), ""
	}

	// A single negative weight switches to Bellman-Ford, which is the one path
	// in this package whose cost is NOT bounded by the global caps. gonum uses
	// Bellman-Ford-Moore (SPFA): its `loops > V*(V-1)` guard caps DEQUEUES, and
	// each dequeue scans that vertex's out-edges, so the real cost is O(V*E) —
	// the edge count is the dominant term, not an irrelevant one. Hence both a
	// vertex cap and a product cap, and the wall-clock budget below.
	if len(b.ids) > maxNegativeWeightNodes {
		return path.Shortest{}, fmt.Sprintf(
			"graphs with negative edge weights use Bellman-Ford, which is limited to %d nodes; graph has %d",
			maxNegativeWeightNodes, len(b.ids))
	}
	if product := len(b.ids) * b.edgeCount; product > maxNegativeWeightProduct {
		return path.Shortest{}, fmt.Sprintf(
			"graphs with negative edge weights use Bellman-Ford, which costs O(nodes*edges) and is limited to a product of %d; graph has %d nodes * %d edges = %d",
			maxNegativeWeightProduct, len(b.ids), b.edgeCount, product)
	}

	type bfResult struct {
		sp path.Shortest
		ok bool
	}
	res, budgetErr := runBounded(ctx, "negative-weight shortest-path search", func() bfResult {
		sp, ok := path.BellmanFordFrom(src, g)
		return bfResult{sp, ok}
	})
	if budgetErr != nil {
		return path.Shortest{}, budgetErr.Error()
	}
	if !res.ok {
		return res.sp, "graph contains a negative-weight cycle reachable from the source; shortest paths are undefined"
	}
	return res.sp, ""
}

// requireQuadraticBudget guards the all-pairs measures on BOTH dimensions that
// drive their cost. The vertex cap alone is not a cost bound, because time is
// O(V*E) and a dense graph can sit under the vertex cap while costing minutes.
func (b *built) requireQuadraticBudget(op string) error {
	if len(b.ids) > maxQuadraticNodes {
		return fmt.Errorf("%s requires an all-pairs computation and is limited to %d nodes; graph has %d",
			op, maxQuadraticNodes, len(b.ids))
	}
	if product := len(b.ids) * b.edgeCount; product > maxQuadraticProduct {
		return fmt.Errorf("%s costs O(nodes*edges) and is limited to a product of %d; graph has %d nodes * %d edges = %d",
			op, maxQuadraticProduct, len(b.ids), b.edgeCount, product)
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

// The two graph-emitting nodes return the bare `Graph` envelope so their output
// composes with an identity edge. Graph has no `error` field, so those nodes
// report a rejected request as a Go error, which the platform surfaces as a
// node failure rather than as a silently empty graph.

func errRequestRequired() error { return fmt.Errorf("request is required") }

func errUnknownNode(id string) error { return fmt.Errorf("unknown node id %s", quote(id)) }

func errUndirectedRequired(node string) error {
	return fmt.Errorf("%s requires an undirected graph; set `directed` to false", node)
}

func errTooManySelected(n, limit int) error {
	return fmt.Errorf("selection lists %d nodes, exceeding the limit of %d", n, limit)
}

// transposedWeightedGraph returns the graph with every edge reversed, as a
// deterministic weighted view. Used by the eccentricity measure to convert
// gonum's incoming-path convention into the outgoing one this package
// documents. For an undirected graph the transpose is the graph itself.
func (b *built) transposedWeightedGraph() graph.Graph {
	if !b.directed {
		return orderedWU{b.wug}
	}
	t := simple.NewWeightedDirectedGraph(0, math.Inf(1))
	for _, id := range b.ids {
		t.AddNode(simple.Node(b.idOf[id]))
	}
	edges := orderedWD{b.wdg}.WeightedEdges()
	for edges.Next() {
		e := edges.WeightedEdge()
		t.SetWeightedEdge(simple.WeightedEdge{
			F: simple.Node(e.To().ID()),
			T: simple.Node(e.From().ID()),
			W: e.Weight(),
		})
	}
	return orderedWD{t}
}

func errSelectionIDTooLong(limit int) error {
	return fmt.Errorf("a selected node id exceeds the limit of %d bytes", limit)
}
