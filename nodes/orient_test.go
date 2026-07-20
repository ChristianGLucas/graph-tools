package nodes_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

func TestOrientToUndirectedCollapsesOpposingEdges(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// a->b costs 5, b->a costs 2. Undirected cannot hold both; the cheaper
	// route must survive.
	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{
		{"a", "b", 5}, {"b", "a", 2}, {"b", "c", 7},
	})
	got, err := nodes.Orient(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Directed {
		t.Fatalf("result must be undirected")
	}
	if len(got.Edges) != 2 {
		t.Fatalf("got %d edges, want 2 (the opposing pair collapses): %+v", len(got.Edges), got.Edges)
	}
	for _, e := range got.Edges {
		if e.From == "a" && e.To == "b" && e.Weight != 2 {
			t.Errorf("collapsed a-b weight = %v, want 2 (the smaller of 5 and 2)", e.Weight)
		}
	}
	// The result must be a graph the rest of the package accepts.
	if st, err := nodes.Describe(ctx, ax, got); err != nil || st.Error != "" {
		t.Fatalf("emitted graph rejected downstream: err=%v nodeErr=%s", err, st.Error)
	}
}

func TestOrientToDirectedDuplicatesEachEdge(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"a", "b", "c"}, [][3]any{{"a", "b", 3}, {"b", "c", 4}})
	got, err := nodes.Orient(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Directed {
		t.Fatalf("result must be directed")
	}
	if len(got.Edges) != 4 {
		t.Fatalf("got %d edges, want 4 (each undirected edge becomes a pair): %+v", len(got.Edges), got.Edges)
	}
	seen := map[string]float64{}
	for _, e := range got.Edges {
		seen[e.From+"->"+e.To] = e.Weight
	}
	for _, want := range []string{"a->b", "b->a", "b->c", "c->b"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("missing edge %s in %+v", want, got.Edges)
		}
	}
	if seen["b->a"] != 3 {
		t.Errorf("reversed edge weight = %v, want 3", seen["b->a"])
	}
	if st, err := nodes.Describe(ctx, ax, got); err != nil || st.Error != "" {
		t.Fatalf("emitted graph rejected downstream: err=%v nodeErr=%s", err, st.Error)
	}
}

// Orient reads its direction from the graph's own `directed` field, so its
// input is the bare Graph envelope. That is what lets it receive a Graph from
// an upstream flow edge; a request wrapper could not. This test pins the
// signature by feeding it another node's Graph output directly, with no
// wrapper construction anywhere.
func TestOrientConsumesAnUpstreamGraphDirectly(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	src := mkGraph(false, []string{"a", "b", "c", "d"}, [][3]any{
		{"a", "b", 1}, {"b", "c", 2}, {"c", "d", 3}, {"a", "d", 9},
	})
	// Subgraph -> Orient -> TopologicalSort, every hop a bare Graph.
	sub, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{Graph: src, Nodes: []string{"a", "b", "c"}})
	if err != nil {
		t.Fatalf("Subgraph: %v", err)
	}
	dir, err := nodes.Orient(ctx, ax, sub) // <- the upstream Graph, unwrapped
	if err != nil {
		t.Fatalf("Orient: %v", err)
	}
	if !dir.Directed {
		t.Fatalf("Orient must flip the undirected subgraph to directed")
	}
	if ts, err := nodes.TopologicalSort(ctx, ax, dir); err != nil || ts.Error != "" {
		t.Fatalf("TopologicalSort on the reoriented subgraph: err=%v nodeErr=%s", err, ts.Error)
	}
}

// The whole point of the node: it makes the previously-unreachable pairings work.
func TestOrientUnlocksTheDirectedUndirectedSplit(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	// A DIRECTED dependency graph can now reach MinimumSpanningTree.
	directed := mkGraph(true, []string{"a", "b", "c", "d"}, [][3]any{
		{"a", "b", 1}, {"b", "c", 2}, {"c", "d", 3}, {"a", "d", 9},
	})
	if _, err := nodes.MinimumSpanningTree(ctx, ax, directed); err == nil {
		t.Fatalf("precondition: MST must reject a directed graph")
	}
	und, err := nodes.Orient(ctx, ax, directed)
	if err != nil {
		t.Fatalf("Orient: %v", err)
	}
	mst, err := nodes.MinimumSpanningTree(ctx, ax, und)
	if err != nil {
		t.Fatalf("MST on the reoriented graph: %v", err)
	}
	stats, err := nodes.Describe(ctx, ax, mst)
	if err != nil || stats.Error != "" {
		t.Fatalf("Describe: err=%v nodeErr=%s", err, stats.Error)
	}
	// MST of a-b 1, b-c 2, c-d 3, a-d 9 is 1+2+3 = 6.
	if stats.TotalWeight != 6 {
		t.Errorf("MST total_weight = %v, want 6", stats.TotalWeight)
	}

	// And the MST's own undirected output can now reach TopologicalSort — the
	// exact recipe MinimumSpanningTree's published description promises.
	backDir, err := nodes.Orient(ctx, ax, mst)
	if err != nil {
		t.Fatalf("Orient(mst): %v", err)
	}
	if ts, err := nodes.TopologicalSort(ctx, ax, backDir); err != nil || ts.Error != "" {
		t.Fatalf("TopologicalSort on the reoriented MST: err=%v nodeErr=%s", err, ts.Error)
	}

	// An UNDIRECTED graph can now reach TopologicalSort (it will be cyclic by
	// construction, which the node documents).
	undirected := mkGraph(false, []string{"x", "y"}, [][3]any{{"x", "y", 1}})
	if r, err := nodes.TopologicalSort(ctx, ax, undirected); err != nil || r.Error == "" {
		t.Fatalf("precondition: TopologicalSort must reject undirected input")
	}
	dir, err := nodes.Orient(ctx, ax, undirected)
	if err != nil {
		t.Fatalf("Orient: %v", err)
	}
	ts, err := nodes.TopologicalSort(ctx, ax, dir)
	if err != nil || ts.Error != "" {
		t.Fatalf("TopologicalSort on the reoriented graph: err=%v nodeErr=%s", err, ts.Error)
	}
	if ts.IsDag {
		t.Errorf("an undirected edge becomes an opposing pair, which is a cycle: %+v", ts)
	}
}

func TestOrientPreservesLabelsSelfLoopsAndZeroWeights(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{
		Directed: true,
		Nodes:    []*gen.GraphNode{{Id: "a", Label: "alpha"}, {Id: "b", Label: "beta"}},
		Edges: []*gen.GraphEdge{
			{From: "a", To: "a", Weight: 1},
			{From: "a", To: "b", Weight: 0, ExplicitZeroWeight: true},
		},
	}
	got, err := nodes.Orient(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Nodes[0].Label != "alpha" || got.Nodes[1].Label != "beta" {
		t.Errorf("labels not preserved: %+v", got.Nodes)
	}
	loops, zero := 0, false
	for _, e := range got.Edges {
		if e.From == e.To {
			loops++
		}
		if e.From == "a" && e.To == "b" && e.ExplicitZeroWeight {
			zero = true
		}
	}
	if loops != 1 {
		t.Errorf("self-loop not preserved: %+v", got.Edges)
	}
	if !zero {
		t.Errorf("explicit_zero_weight not preserved: %+v", got.Edges)
	}
}

// Orienting a directed graph that holds no opposing pairs to undirected and
// back must return the original edge set, doubled in the documented way.
func TestOrientRoundTrip(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"a", "b", "c"}, [][3]any{{"a", "b", 2}, {"b", "c", 5}})
	dir, err := nodes.Orient(ctx, ax, g)
	if err != nil {
		t.Fatalf("Orient: %v", err)
	}
	back, err := nodes.Orient(ctx, ax, dir)
	if err != nil {
		t.Fatalf("Orient back: %v", err)
	}
	if back.Directed {
		t.Fatalf("round trip must land back on undirected")
	}
	if len(back.Edges) != 2 {
		t.Fatalf("round trip changed the edge set: %+v", back.Edges)
	}
	for _, e := range back.Edges {
		if e.From == "a" && e.To == "b" && e.Weight != 2 {
			t.Errorf("a-b weight = %v, want 2", e.Weight)
		}
		if e.From == "b" && e.To == "c" && e.Weight != 5 {
			t.Errorf("b-c weight = %v, want 5", e.Weight)
		}
	}
}

// Undirected -> directed doubles the edge count. The node must bound its own
// OUTPUT, rather than emitting a graph every sibling node would reject.
func TestOrientBoundsItsDoubledOutput(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// A ring lattice: 20000 vertices (the node cap exactly), each joined to its
	// next 6 neighbours, for 120000 edges. Ids are kept short and weights
	// omitted so the input sits comfortably under the node, edge and
	// encoded-size limits — the ONLY thing it violates is the doubled OUTPUT
	// (2*120000 = 240000 > 200000). Otherwise this test would pass on an
	// unrelated bound and prove nothing.
	const verts, span = 20000, 6
	g := &gen.Graph{Directed: false}
	id := func(i int) string { return strconv.FormatInt(int64(i), 36) }
	for i := 0; i < verts; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: id(i)})
	}
	for i := 0; i < verts; i++ {
		for d := 1; d <= span; d++ {
			g.Edges = append(g.Edges, &gen.GraphEdge{From: id(i), To: id((i + d) % verts)})
		}
	}

	// Precondition: the input itself is perfectly acceptable to the package.
	if st, err := nodes.Describe(ctx, ax, g); err != nil || st.Error != "" {
		t.Fatalf("precondition: the INPUT must be within every other bound; err=%v nodeErr=%s", err, st.Error)
	}

	got, err := nodes.Orient(ctx, ax, g)
	if err == nil {
		t.Fatalf("expected a rejection; got a graph with %d edges", len(got.Edges))
	}
	if got != nil {
		t.Errorf("a rejected request must not also return a graph")
	}
	// It must be the OUTPUT-doubling bound specifically.
	if !strings.Contains(err.Error(), "doubles the edge count") {
		t.Fatalf("wrong bound fired — this test must exercise the output bound, not an\n"+
			"incidental one. got: %v", err)
	}
	t.Logf("bounded as expected: %v", err)
}

func TestOrientDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b", "c", "d"}, [][3]any{
		{"a", "b", 5}, {"b", "a", 2}, {"c", "d", 1}, {"d", "c", 1}, {"a", "c", 3},
	})
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.Orient(ctx, ax, g)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var cur []string
		for _, e := range got.Edges {
			cur = append(cur, e.From+"-"+e.To)
		}
		if i == 0 {
			first = cur
			continue
		}
		if !eqStrings(cur, first) {
			t.Fatalf("nondeterministic edge order: run %d gave %v, first gave %v", i, cur, first)
		}
	}
}

func TestOrientErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for name, g := range map[string]*gen.Graph{
		"nil graph":    nil,
		"duplicate id": {Nodes: []*gen.GraphNode{{Id: "a"}, {Id: "a"}}},
		"unknown edge": {Nodes: []*gen.GraphNode{{Id: "a"}}, Edges: []*gen.GraphEdge{{From: "a", To: "zz"}}},
	} {
		if _, err := nodes.Orient(ctx, ax, g); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
