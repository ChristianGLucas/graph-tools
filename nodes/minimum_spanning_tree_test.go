package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// mstFixture: 5 vertices, 7 undirected weighted edges. The minimum spanning
// tree is A-B(1), B-C(1), C-D(2), D-E(4) for a total weight of 8.
func mstFixture() *gen.Graph {
	return mkGraph(false, []string{"A", "B", "C", "D", "E"}, [][3]any{
		{"A", "B", 1}, {"A", "C", 3}, {"B", "C", 1}, {"B", "D", 6},
		{"C", "D", 2}, {"D", "E", 4}, {"C", "E", 5},
	})
}

func TestMinimumSpanningTree(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.MinimumSpanningTree(ctx, ax, mstFixture())
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.TotalWeight != 8 {
		t.Errorf("total_weight = %v, want 8", got.TotalWeight)
	}
	if got.ComponentCount != 1 {
		t.Errorf("component_count = %d, want 1", got.ComponentCount)
	}
	if got.Tree == nil {
		t.Fatalf("tree must be emitted")
	}
	if len(got.Tree.Nodes) != 5 {
		t.Errorf("tree has %d nodes, want 5", len(got.Tree.Nodes))
	}
	// A spanning tree of 5 vertices has exactly 4 edges.
	if len(got.Tree.Edges) != 4 {
		t.Errorf("tree has %d edges, want 4: %+v", len(got.Tree.Edges), got.Tree.Edges)
	}
	if got.Tree.Directed {
		t.Errorf("the emitted tree must be undirected")
	}
	// The emitted tree's own weights must sum to the reported total.
	sum := 0.0
	for _, e := range got.Tree.Edges {
		sum += e.Weight
	}
	if sum != got.TotalWeight {
		t.Errorf("emitted tree edge weights sum to %v but total_weight is %v", sum, got.TotalWeight)
	}
}

// TestMinimumSpanningTreeAgainstBruteForceOracle checks the weight against an
// exhaustive search over every |V|-1 subset of edges.
func TestMinimumSpanningTreeAgainstBruteForceOracle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mstFixture()
	want := bruteForceMSTWeight(g)
	got, err := nodes.MinimumSpanningTree(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.TotalWeight != want {
		t.Errorf("total_weight = %v, exhaustive oracle = %v", got.TotalWeight, want)
	}
}

// TestMinimumSpanningTreeComposes feeds the emitted tree straight back into
// another node — the canonical-envelope guarantee.
func TestMinimumSpanningTreeComposes(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	mst, err := nodes.MinimumSpanningTree(ctx, ax, mstFixture())
	if err != nil || mst.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, mst.Error)
	}
	// A spanning tree must be acyclic and connected.
	cyc, err := nodes.DetectCycle(ctx, ax, mst.Tree)
	if err != nil || cyc.Error != "" {
		t.Fatalf("DetectCycle on the emitted tree: err=%v nodeErr=%s", err, cyc.Error)
	}
	if cyc.HasCycle {
		t.Errorf("a spanning tree must be acyclic, got %+v", cyc)
	}
	stats, err := nodes.Describe(ctx, ax, mst.Tree)
	if err != nil || stats.Error != "" {
		t.Fatalf("Describe on the emitted tree: err=%v nodeErr=%s", err, stats.Error)
	}
	if !stats.IsConnected {
		t.Errorf("a spanning tree of a connected graph must be connected, got %+v", stats)
	}
	if stats.EdgeCount != stats.NodeCount-1 {
		t.Errorf("a tree must have |V|-1 edges: %d nodes, %d edges", stats.NodeCount, stats.EdgeCount)
	}
}

// A disconnected input yields a spanning forest, not an error.
func TestMinimumSpanningTreeForest(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"A", "B", "X", "Y"}, [][3]any{{"A", "B", 2}, {"X", "Y", 3}})
	got, err := nodes.MinimumSpanningTree(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.ComponentCount != 2 {
		t.Errorf("component_count = %d, want 2", got.ComponentCount)
	}
	if got.TotalWeight != 5 {
		t.Errorf("total_weight = %v, want 5", got.TotalWeight)
	}
	if len(got.Tree.Edges) != 2 {
		t.Errorf("forest has %d edges, want 2", len(got.Tree.Edges))
	}
}

func TestMinimumSpanningTreeRejectsDirected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.MinimumSpanningTree(ctx, ax, dagFixture())
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected directed input to be rejected, got %+v", got)
	}
}

// TestMinimumSpanningTreeDeterminism uses an all-equal-weight graph, where many
// distinct spanning trees are equally minimal.
func TestMinimumSpanningTreeDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "D", 1}, {"D", "A", 1}, {"A", "C", 1}, {"B", "D", 1},
	})
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.MinimumSpanningTree(ctx, ax, g)
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		var edges []string
		for _, e := range got.Tree.Edges {
			edges = append(edges, e.From+"-"+e.To)
		}
		if i == 0 {
			first = edges
			continue
		}
		if !eqStrings(edges, first) {
			t.Fatalf("nondeterministic spanning tree: run %d gave %v, first gave %v", i, edges, first)
		}
	}
}

func TestMinimumSpanningTreeNilGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.MinimumSpanningTree(ctx, ax, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a structured error for a nil graph")
	}
}
