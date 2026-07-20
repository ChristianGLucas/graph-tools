package nodes_test

import (
	"context"
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

	if got, err := nodes.MinimumSpanningTree(ctx, ax, undirected); err != nil || got.Error != "" {
		t.Fatalf("MinimumSpanningTree: err=%v nodeErr=%s", err, got.Error)
	} else if len(got.Tree.Edges) != n-1 {
		t.Errorf("MST has %d edges, want %d", len(got.Tree.Edges), n-1)
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
	mst, err := nodes.MinimumSpanningTree(ctx, ax, u)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if mst.Error == "" {
		t.Errorf("MinimumSpanningTree must report overflow, got total_weight=%v", mst.TotalWeight)
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
	if err != nil || m1.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, m1.Error)
	}
	m2, err := nodes.MinimumSpanningTree(ctx, ax, loopedU)
	if err != nil || m2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, m2.Error)
	}
	if m1.TotalWeight != m2.TotalWeight || len(m1.Tree.Edges) != len(m2.Tree.Edges) {
		t.Errorf("a self-loop changed the spanning tree: %v/%d vs %v/%d",
			m1.TotalWeight, len(m1.Tree.Edges), m2.TotalWeight, len(m2.Tree.Edges))
	}

	c1, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: plainU, Measure: "degree"})
	if err != nil || c1.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, c1.Error)
	}
	c2, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: loopedU, Measure: "degree"})
	if err != nil || c2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, c2.Error)
	}
	for i := range c1.Scores {
		if c1.Scores[i].Score != c2.Scores[i].Score {
			t.Errorf("a self-loop changed degree(%s): %v vs %v",
				c1.Scores[i].Node, c1.Scores[i].Score, c2.Scores[i].Score)
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
