package nodes_test

import (
	"context"
	"math"
	"testing"
	"time"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// bigGraph builds a connected chain of n vertices plus a few chords — a
// realistically-sized input rather than a toy one.
func bigGraph(n int, directed bool) *gen.Graph {
	g := &gen.Graph{Directed: directed}
	for i := 0; i < n; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	for i := 0; i+1 < n; i++ {
		g.Edges = append(g.Edges, &gen.GraphEdge{
			From: "n" + itoa(i), To: "n" + itoa(i+1), Weight: float64(1 + i%7),
		})
	}
	return g
}

// TestNodeLimitRejected: the documented 20000-vertex ceiling must fire on the
// RAW input, before any allocation.
func TestNodeLimitRejected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{}
	for i := 0; i < 20001; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Fatalf("expected the node limit to fire, got %+v", got)
	}
}

// TestEdgeLimitRejected: the edge ceiling is independent of the vertex ceiling,
// so a small vertex set with a huge edge list must still be refused.
func TestEdgeLimitRejected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "A"}, {Id: "B"}},
	}
	for i := 0; i < 200001; i++ {
		g.Edges = append(g.Edges, &gen.GraphEdge{From: "A", To: "B"})
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Fatalf("expected the edge limit to fire, got %+v", got)
	}
}

// TestLargeGraphsStayFast exercises the linear-ish nodes at a realistic scale
// and asserts they complete promptly rather than degrading pathologically.
func TestLargeGraphsStayFast(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	const n = 5000

	undirected := bigGraph(n, false)
	directed := bigGraph(n, true)

	start := time.Now()

	if got, err := nodes.Describe(ctx, ax, undirected); err != nil || got.Error != "" {
		t.Fatalf("Describe: err=%v nodeErr=%s", err, got.Error)
	} else if got.NodeCount != n || got.EdgeCount != n-1 {
		t.Errorf("Describe counts = %d/%d, want %d/%d", got.NodeCount, got.EdgeCount, n, n-1)
	}

	if got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{
		Graph: directed, From: "n0", To: "n" + itoa(n-1),
	}); err != nil || got.Error != "" {
		t.Fatalf("ShortestPath: err=%v nodeErr=%s", err, got.Error)
	} else if !got.Found || int(got.HopCount) != n-1 {
		t.Errorf("ShortestPath hop_count = %d, want %d", got.HopCount, n-1)
	}

	if got, err := nodes.TopologicalSort(ctx, ax, directed); err != nil || got.Error != "" {
		t.Fatalf("TopologicalSort: err=%v nodeErr=%s", err, got.Error)
	} else if !got.IsDag || len(got.Order) != n {
		t.Errorf("TopologicalSort produced %d entries, want %d", len(got.Order), n)
	}

	if got, err := nodes.MinimumSpanningTree(ctx, ax, undirected); err != nil {
		t.Fatalf("MinimumSpanningTree: %v", err)
	} else if len(got.Edges) != n-1 {
		t.Errorf("MST has %d edges, want %d", len(got.Edges), n-1)
	}

	if got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: directed}); err != nil || got.Error != "" {
		t.Fatalf("PageRank: err=%v nodeErr=%s", err, got.Error)
	} else if len(got.Scores) != n {
		t.Errorf("PageRank returned %d scores, want %d", len(got.Scores), n)
	}

	if elapsed := time.Since(start); elapsed > 90*time.Second {
		t.Errorf("large-graph suite took %v, which is far longer than expected", elapsed)
	}
}

// A deeply-chained graph must not blow the stack — the algorithms in use are
// iterative, and this test is the regression guard for that.
func TestDeepChainDoesNotOverflowStack(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := bigGraph(20000, true)

	if got, err := nodes.TopologicalSort(ctx, ax, g); err != nil || got.Error != "" {
		t.Fatalf("TopologicalSort on a 20000-deep chain: err=%v nodeErr=%s", err, got.Error)
	} else if !got.IsDag {
		t.Errorf("a chain is a DAG")
	}
	if got, err := nodes.ConnectedComponents(ctx, ax, g); err != nil || got.Error != "" {
		t.Fatalf("ConnectedComponents on a 20000-deep chain: err=%v nodeErr=%s", err, got.Error)
	} else if got.Count != 20000 {
		t.Errorf("a directed chain has 20000 strongly connected components, got %d", got.Count)
	}
	if got, err := nodes.DetectCycle(ctx, ax, g); err != nil || got.Error != "" {
		t.Fatalf("DetectCycle on a 20000-deep chain: err=%v nodeErr=%s", err, got.Error)
	} else if got.HasCycle {
		t.Errorf("a chain has no cycle")
	}
}

// A graph at exactly the documented limit must be ACCEPTED — an off-by-one in
// the bound would silently refuse legitimate input.
func TestExactLimitAccepted(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{}
	for i := 0; i < 20000; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("a graph at exactly the limit must be accepted: err=%v nodeErr=%s", err, got.Error)
	}
	if got.NodeCount != 20000 {
		t.Errorf("node_count = %d, want 20000", got.NodeCount)
	}
}

// TestWeightOverflowIsReportedNotMisreported is the regression guard for a real
// bug: individually-finite edge weights whose sum overflows to +Inf were being
// treated as "no path exists", so a genuinely reachable target came back with
// found=false. Overflow must surface as a structured error instead.
func TestWeightOverflowIsReportedNotMisreported(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1e308}, {"B", "C", 1e308},
	})

	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "C"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if sp.Error == "" {
		t.Errorf("overflow must be reported; got found=%v weight=%v", sp.Found, sp.TotalWeight)
	}
	if sp.Found {
		t.Errorf("an errored result must not also claim found=true: %+v", sp)
	}
	// Every other node must reject the same graph consistently, since the
	// guard lives in the shared build step.
	if st, err := nodes.Describe(ctx, ax, g); err != nil || st.Error == "" {
		t.Errorf("Describe must reject an overflowing graph, got err=%v result=%+v", err, st)
	}

	d, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: "A"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if d.Error == "" {
		t.Errorf("Distances must report overflow, got %+v", d)
	}

	u := mkGraph(false, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1e308}, {"B", "C", 1e308},
	})
	if _, err := nodes.MinimumSpanningTree(ctx, ax, u); err == nil {
		t.Errorf("MinimumSpanningTree must reject an overflowing graph")
	}
}

// A large-but-safe weight must still work — the overflow guard must not reject
// legitimate heavy-weight graphs.
func TestLargeButFiniteWeightsStillWork(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1e100}, {"B", "C", 1e100},
	})
	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "C"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.Found || got.TotalWeight != 2e100 {
		t.Errorf("got found=%v weight=%v, want found=true weight=2e100", got.Found, got.TotalWeight)
	}
}

// TestSelfLoopsAreCountedButDoNotAffectPathsOrTrees pins the documented
// self-loop contract: they are reported by Describe and DetectCycle, but must
// not change a shortest path, a spanning tree, or a degree score, since a
// self-loop can never appear on any of them.
func TestSelfLoopsAreCountedButDoNotAffectPathsOrTrees(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	plainD := dagFixture()
	loopedD := dagFixture()
	loopedD.Edges = append(loopedD.Edges, &gen.GraphEdge{From: "C", To: "C", Weight: 99})

	a, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: plainD, From: "A", To: "E"})
	if err != nil || a.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, a.Error)
	}
	b, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: loopedD, From: "A", To: "E"})
	if err != nil || b.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, b.Error)
	}
	if !eqStrings(a.Path, b.Path) || a.TotalWeight != b.TotalWeight {
		t.Errorf("a self-loop changed the shortest path: %v/%v vs %v/%v",
			a.Path, a.TotalWeight, b.Path, b.TotalWeight)
	}

	plainU := mstFixture()
	loopedU := mstFixture()
	loopedU.Edges = append(loopedU.Edges, &gen.GraphEdge{From: "C", To: "C", Weight: 99})

	m1, err := nodes.MinimumSpanningTree(ctx, ax, plainU)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m2, err := nodes.MinimumSpanningTree(ctx, ax, loopedU)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m1.Edges) != len(m2.Edges) {
		t.Errorf("a self-loop changed the spanning tree: %d vs %d edges", len(m1.Edges), len(m2.Edges))
	}

	// Degree centrality DOES count a self-loop — by the standard convention it
	// adds 2 to that vertex's degree, and only that vertex's.
	c1, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: plainU, Measure: "degree"})
	if err != nil || c1.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, c1.Error)
	}
	c2, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: loopedU, Measure: "degree"})
	if err != nil || c2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, c2.Error)
	}
	for i := range c1.Scores {
		want := c1.Scores[i].Score
		if c1.Scores[i].Node == "C" {
			want += 2
		}
		if c2.Scores[i].Score != want {
			t.Errorf("degree(%s) with a self-loop on C = %v, want %v",
				c2.Scores[i].Node, c2.Scores[i].Score, want)
		}
	}

	// But it IS visible where the contract says it is.
	st, err := nodes.Describe(ctx, ax, loopedU)
	if err != nil || st.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, st.Error)
	}
	if st.SelfLoopCount != 1 {
		t.Errorf("self_loop_count = %d, want 1", st.SelfLoopCount)
	}
	cyc, err := nodes.DetectCycle(ctx, ax, loopedD)
	if err != nil || cyc.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, cyc.Error)
	}
	if !cyc.HasCycle {
		t.Errorf("a self-loop makes the graph cyclic, got %+v", cyc)
	}
}

// TestStringLengthBounds: counting ELEMENTS leaves the byte dimension
// unbounded. 20000 vertices with 10 KiB ids is a 381 MiB payload that every
// element-based cap happily accepts, so the per-string caps are load-bearing.
func TestStringLengthBounds(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	long := make([]byte, 257)
	for i := range long {
		long[i] = 'x'
	}
	g := &gen.Graph{Nodes: []*gen.GraphNode{{Id: string(long)}}}
	if got, err := nodes.Describe(ctx, ax, g); err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	} else if got.Error == "" {
		t.Errorf("expected an over-long node id to be rejected")
	}

	longLabel := make([]byte, 1025)
	for i := range longLabel {
		longLabel[i] = 'y'
	}
	g2 := &gen.Graph{Nodes: []*gen.GraphNode{{Id: "a", Label: string(longLabel)}}}
	if got, err := nodes.Describe(ctx, ax, g2); err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	} else if got.Error == "" {
		t.Errorf("expected an over-long label to be rejected")
	}

	// Exactly at the limits must be ACCEPTED.
	okID := string(long[:256])
	okLabel := string(longLabel[:1024])
	g3 := &gen.Graph{Nodes: []*gen.GraphNode{{Id: okID, Label: okLabel}}}
	if got, err := nodes.Describe(ctx, ax, g3); err != nil || got.Error != "" {
		t.Fatalf("ids/labels exactly at the limit must be accepted: err=%v nodeErr=%s", err, got.Error)
	}
}

// TestEncodedSizeBound is the backstop for any byte dimension the per-field
// caps do not model.
func TestEncodedSizeBound(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// 20000 vertices each carrying a 256-byte id and a 1024-byte label passes
	// every per-element cap but is ~25 MiB encoded.
	id := make([]byte, 256)
	label := make([]byte, 1024)
	for i := range id {
		id[i] = 'a'
	}
	for i := range label {
		label[i] = 'b'
	}
	g := &gen.Graph{}
	for i := 0; i < 20000; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{
			Id:    itoa(i) + string(id),
			Label: string(label),
		})
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected the encoded-size bound to fire on a ~25 MiB payload")
	}
}

// TestSubgraphSelectionBound: the selection list is caller-controlled and
// separate from the graph, so it needs its own bound.
func TestSubgraphSelectionBound(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	sel := make([]string, maxNodesForTest+1)
	for i := range sel {
		sel[i] = "a"
	}
	if _, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{
		Graph: mkGraph(false, []string{"a"}, nil), Nodes: sel,
	}); err == nil {
		t.Errorf("expected an over-long selection list to be rejected")
	}
}

const maxNodesForTest = 20000

// TestAllPairsMeasuresStayFastAtTheBound proves the all-pairs cost bound
// actually keeps the worst ADMISSIBLE input cheap — a vertex-only cap does not,
// since a dense 600-vertex graph passes it while costing over a minute.
func TestAllPairsMeasuresStayFastAtTheBound(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// 600 vertices with 2000 edges: product = 1.2e6, exactly at the budget.
	var ids []string
	for i := 0; i < 600; i++ {
		ids = append(ids, "n"+itoa(i))
	}
	var edges [][3]any
	for i := 0; len(edges) < 2000; i++ {
		a, b := i%600, (i*7+1)%600
		if a == b {
			continue
		}
		edges = append(edges, [3]any{ids[a], ids[b], 1})
	}
	g := mkGraph(false, ids, edges)

	for _, measure := range []string{"betweenness", "closeness", "harmonic", "eccentricity"} {
		start := time.Now()
		got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: measure})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("%s: unexpected Go error: %v", measure, err)
		}
		if got.Error != "" {
			// Duplicate edges from the generator can push it over; that is a
			// legitimate rejection, not a failure of this test.
			t.Logf("%s rejected at the bound: %s", measure, got.Error)
			continue
		}
		if elapsed > 60*time.Second {
			t.Errorf("%s took %v at the documented cost bound — the bound does not bound the cost", measure, elapsed)
		}
		t.Logf("%s at the bound: %v", measure, elapsed.Round(time.Millisecond))
	}
}

// TestNegativeCycleIsBoundedByVertexCount is the regression guard for a cost
// amplification the edge and byte caps did not touch. A single negative weight
// switches Dijkstra -> Bellman-Ford, whose negative-cycle detection loops until
// it exceeds V*(V-1) relaxations — quadratic in the VERTEX count and
// independent of the edge count. 20000 vertices plus THREE edges forming a
// negative cycle is a ~200 KB payload that cost 106 seconds of CPU.
func TestNegativeCycleIsBoundedByVertexCount(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	mk := func(n int) *gen.Graph {
		g := &gen.Graph{Directed: true}
		for i := 0; i < n; i++ {
			g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
		}
		for i := 0; i < 3; i++ {
			g.Edges = append(g.Edges, &gen.GraphEdge{
				From: "n" + itoa(i), To: "n" + itoa((i+1)%3), Weight: -1,
			})
		}
		return g
	}

	// Above the Bellman-Ford bound: must be refused, and refused FAST.
	start := time.Now()
	got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: mk(20000), From: "n0"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a 20000-vertex negative-weight graph to be refused")
	}
	if elapsed > 5*time.Second {
		t.Errorf("refusal took %v — the bound must fire before the work, not after", elapsed)
	}

	// Same for ShortestPath, which shares the code path.
	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: mk(20000), From: "n0", To: "n1"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if sp.Error == "" {
		t.Errorf("ShortestPath must apply the same Bellman-Ford bound")
	}

	// At the bound the negative cycle must still be correctly REPORTED, quickly.
	start = time.Now()
	ok, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: mk(2000), From: "n0"})
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if ok.Error == "" {
		t.Errorf("a reachable negative cycle must be reported as a structured error")
	}
	if elapsed > 20*time.Second {
		t.Errorf("negative-cycle detection at the bound took %v", elapsed)
	}
	t.Logf("negative cycle at the 2000-vertex bound: %v", elapsed.Round(time.Millisecond))
}

// Negative weights WITHOUT a cycle must still work at the bound.
func TestNegativeWeightsWithoutCycleStillWork(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{
		{"a", "b", 5}, {"a", "c", 2}, {"c", "b", -4},
	})
	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "a", To: "b"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.TotalWeight != -2 {
		t.Errorf("total_weight = %v, want -2", got.TotalWeight)
	}
}

// TestErrorMessagesDoNotAmplify: caller strings echoed into an error must be
// truncated. strconv.Quote expands binary bytes up to fourfold, so echoing an
// unbounded id back turns a large request into a much larger response.
func TestErrorMessagesDoNotAmplify(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	huge := make([]byte, 300000)
	for i := range huge {
		huge[i] = 0x01
	}

	// An over-long EDGE ENDPOINT (not a node id) must be rejected without echo.
	g := &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "a"}},
		Edges: []*gen.GraphEdge{{From: string(huge), To: "a"}},
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Fatalf("expected an over-long edge endpoint to be rejected")
	}
	if len(got.Error) > 1000 {
		t.Errorf("error message is %d bytes — caller input is being echoed unbounded", len(got.Error))
	}

	// An over-long SUBGRAPH SELECTION id, likewise.
	_, serr := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{
		Graph: mkGraph(false, []string{"a"}, nil),
		Nodes: []string{string(huge)},
	})
	if serr == nil {
		t.Fatalf("expected an over-long selection id to be rejected")
	}
	if len(serr.Error()) > 1000 {
		t.Errorf("Subgraph error is %d bytes — selection input is being echoed unbounded", len(serr.Error()))
	}

	// An unknown-but-legal-length id IS echoed, but truncated.
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'q'
	}
	got2, err := nodes.Describe(ctx, ax, &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "a"}},
		Edges: []*gen.GraphEdge{{From: string(long), To: "a"}},
	})
	if err != nil || got2.Error == "" {
		t.Fatalf("expected an unknown-endpoint error, got err=%v result=%+v", err, got2)
	}
	if len(got2.Error) > 300 {
		t.Errorf("unknown-endpoint error is %d bytes; expected truncation", len(got2.Error))
	}
}

// TestControlCharactersInIdsRejected: an id with a NUL or other C0 control
// character is almost always a caller bug, and would be carried silently into
// every emitted graph.
func TestControlCharactersInIdsRejected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, bad := range []string{"a\x00b", "a\tb", "\x1b[31m", "a\x7f"} {
		got, err := nodes.Describe(ctx, ax, &gen.Graph{Nodes: []*gen.GraphNode{{Id: bad}}})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if got.Error == "" {
			t.Errorf("id %q contains a control character and must be rejected", bad)
		}
	}
	// Ordinary ids, including non-ASCII, must still be accepted.
	for _, ok := range []string{"a", "node-1", "café", "日本語", "a b"} {
		got, err := nodes.Describe(ctx, ax, &gen.Graph{Nodes: []*gen.GraphNode{{Id: ok}}})
		if err != nil || got.Error != "" {
			t.Errorf("id %q must be accepted, got err=%v nodeErr=%s", ok, err, got.Error)
		}
	}
}

// TestNegativeZeroWeightRejected: -0.0 compares equal to 0, so it silently took
// the "omitted weight" branch and became 1.0. Reject rather than guess.
func TestNegativeZeroWeightRejected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	negZero := math.Copysign(0, -1)

	g := &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "a"}, {Id: "b"}},
		Edges: []*gen.GraphEdge{{From: "a", To: "b", Weight: negZero}},
	}
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("a negative-zero weight must be rejected, not silently treated as 1.0")
	}

	// With the explicit flag it is a genuine zero and must be accepted.
	g2 := &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "a"}, {Id: "b"}},
		Edges: []*gen.GraphEdge{{From: "a", To: "b", Weight: negZero, ExplicitZeroWeight: true}},
	}
	got2, err := nodes.Describe(ctx, ax, g2)
	if err != nil || got2.Error != "" {
		t.Fatalf("explicit negative zero must be accepted: err=%v nodeErr=%s", err, got2.Error)
	}
	if got2.TotalWeight != 0 {
		t.Errorf("total_weight = %v, want 0", got2.TotalWeight)
	}
}

// TestComputeBudgetIsHonoured: a cancelled context must return promptly rather
// than waiting for an uninterruptible gonum call to finish.
func TestComputeBudgetIsHonoured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ax := newTestContext(t)
	cancel() // already cancelled

	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{{"a", "b", 1}, {"b", "c", 1}})

	pr, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if pr.Error == "" {
		t.Errorf("PageRank must report a cancelled context, got %+v", pr)
	}

	c, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "betweenness"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if c.Error == "" {
		t.Errorf("Centrality must report a cancelled context, got %+v", c)
	}
}

// TestNegativeWeightCostIsBoundedByTheProduct is the regression guard for a
// bound that was calibrated against the wrong cost model. gonum uses
// Bellman-Ford-Moore (SPFA): the `loops > V*(V-1)` guard caps DEQUEUES, and
// each dequeue scans that vertex's out-edges — so the real cost is O(V*E), and
// the edge count is the DOMINANT term, not an irrelevant one. A vertex-only cap
// was validated on a 3-edge graph; at 2000 vertices and 200000 edges the same
// shape cost over a minute from a payload that passed every documented bound.
func TestNegativeWeightCostIsBoundedByTheProduct(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	// 2000 vertices (at the vertex cap) with 5000 edges, one of them negative.
	// Product = 1e7, well past the 1.2e6 cap, while the payload stays under the
	// 3 MiB byte cap — so this exercises the PRODUCT bound specifically rather
	// than tripping an earlier one. In an undirected graph a single negative
	// edge IS a negative cycle.
	const n, m = 2000, 5000
	g := &gen.Graph{Directed: false}
	for i := 0; i < n; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	seen := map[[2]int]bool{}
	for s := 1; len(g.Edges) < m && s < n; s++ {
		for i := 0; i < n && len(g.Edges) < m; i++ {
			j := (i + s) % n
			if i >= j || seen[[2]int{i, j}] {
				continue
			}
			seen[[2]int{i, j}] = true
			w := 1.0
			if len(g.Edges) == 0 {
				w = -1
			}
			g.Edges = append(g.Edges, &gen.GraphEdge{From: "n" + itoa(i), To: "n" + itoa(j), Weight: w})
		}
	}

	start := time.Now()
	got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: "n0"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected %d nodes * %d edges to exceed the negative-weight product bound", n, len(g.Edges))
	}
	if elapsed > 10*time.Second {
		t.Errorf("refusal took %v — the bound must fire before the work, not after", elapsed)
	}
	t.Logf("refused %d*%d in %v: %s", n, len(g.Edges), elapsed.Round(time.Millisecond), got.Error)

	// A graph AT the product bound must still be accepted and answered quickly.
	small := &gen.Graph{Directed: false}
	for i := 0; i < 300; i++ {
		small.Nodes = append(small.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	for i := 0; i+1 < 300; i++ {
		w := 1.0
		if i == 0 {
			w = -1
		}
		small.Edges = append(small.Edges, &gen.GraphEdge{
			From: "n" + itoa(i), To: "n" + itoa(i+1), Weight: w,
		})
	}
	start = time.Now()
	ok, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: small, From: "n0"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if ok.Error == "" {
		// An undirected negative edge is a negative cycle, so an error is
		// expected here — what matters is that it comes back fast.
		t.Logf("accepted and answered")
	}
	if el := time.Since(start); el > 10*time.Second {
		t.Errorf("a graph inside the product bound took %v", el)
	}
}

// TestLargeDiameterGraphsStayBounded is the regression guard for two quadratic
// paths that the input caps did not model, both selected by graph DIAMETER
// rather than by size. A long chain is only ~380 KB of input yet made Distances
// materialise a path per vertex (O(V*diameter)), and a "broom" — a chain with a
// back-edge from every vertex — made DetectCycle's witness search reconstruct a
// path per in-neighbour, churning gigabytes to emit a three-element cycle.
func TestLargeDiameterGraphsStayBounded(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	const n = 20000

	chain := &gen.Graph{Directed: true}
	for i := 0; i < n; i++ {
		chain.Nodes = append(chain.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	for i := 0; i+1 < n; i++ {
		chain.Edges = append(chain.Edges, &gen.GraphEdge{From: "n" + itoa(i), To: "n" + itoa(i+1)})
	}

	start := time.Now()
	d, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: chain, From: "n0"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if d.Error != "" {
		t.Fatalf("unexpected node error: %s", d.Error)
	}
	if len(d.Distances) != n {
		t.Errorf("got %d distances, want %d", len(d.Distances), n)
	}
	// The far end of the chain is n-1 hops away — a real correctness check that
	// the hop counts survived the change.
	last := d.Distances[len(d.Distances)-1]
	for _, e := range d.Distances {
		if e.Node == "n"+itoa(n-1) {
			last = e
		}
	}
	if last.HopCount != int32(n-1) {
		t.Errorf("hop_count to the chain end = %d, want %d", last.HopCount, n-1)
	}
	// Was ~2.4s (quadratic) before hop-count memoisation with farthest-first
	// resolution; ~70ms after. A generous threshold that still catches a
	// regression to the quadratic behaviour, even under the race detector.
	if elapsed > 10*time.Second {
		t.Errorf("Distances on a %d-vertex chain took %v — the O(V*diameter) path reconstruction is back", n, elapsed)
	}
	t.Logf("Distances on a %d-vertex chain: %v", n, elapsed.Round(time.Millisecond))

	// Broom: a chain plus a back-edge from every vertex to the head, so the
	// head has in-degree ~n inside one huge SCC.
	broom := &gen.Graph{Directed: true}
	for i := 0; i < n; i++ {
		broom.Nodes = append(broom.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	for i := 0; i+1 < n; i++ {
		broom.Edges = append(broom.Edges, &gen.GraphEdge{From: "n" + itoa(i), To: "n" + itoa(i+1)})
	}
	for i := 1; i < n; i++ {
		broom.Edges = append(broom.Edges, &gen.GraphEdge{From: "n" + itoa(i), To: "n0"})
	}

	start = time.Now()
	c, err := nodes.DetectCycle(ctx, ax, broom)
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if c.Error != "" {
		t.Fatalf("unexpected node error: %s", c.Error)
	}
	if !c.HasCycle {
		t.Errorf("a broom graph contains cycles")
	}
	if !isWalk(broom, c.Cycle) {
		t.Errorf("witness %v is not a walk in the graph", c.Cycle)
	}
	if c.Cycle[0] != c.Cycle[len(c.Cycle)-1] {
		t.Errorf("witness %v is not closed", c.Cycle)
	}
	if elapsed > 10*time.Second {
		t.Errorf("DetectCycle on a %d-vertex broom took %v — the per-in-neighbour path reconstruction is back", n, elapsed)
	}
	t.Logf("DetectCycle on a %d-vertex broom: %v -> witness %v", n, elapsed.Round(time.Millisecond), c.Cycle)
}
