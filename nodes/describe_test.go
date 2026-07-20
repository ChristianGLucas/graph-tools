package nodes_test

import (
	"context"
	"testing"

	"christiangeorgelucas/graph-tools/nodes"
)

func TestDescribe(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// mstFixture: undirected, 5 vertices, 7 edges, connected.
	got, err := nodes.Describe(ctx, ax, mstFixture())
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.NodeCount != 5 || got.EdgeCount != 7 {
		t.Errorf("counts = %d nodes / %d edges, want 5 / 7", got.NodeCount, got.EdgeCount)
	}
	if got.Directed {
		t.Errorf("mstFixture is undirected")
	}
	// Hand-computed: 7 / (5*4/2) = 0.7
	if !nearly(got.Density, 0.7, 1e-12) {
		t.Errorf("density = %v, want 0.7", got.Density)
	}
	// Hand-computed: 2*7/5 = 2.8
	if !nearly(got.AverageDegree, 2.8, 1e-12) {
		t.Errorf("average_degree = %v, want 2.8", got.AverageDegree)
	}
	if !got.IsConnected || got.ComponentCount != 1 {
		t.Errorf("expected a single connected component, got connected=%v count=%d",
			got.IsConnected, got.ComponentCount)
	}
	if got.IsDag {
		t.Errorf("is_dag must be false for undirected input")
	}
	if got.SelfLoopCount != 0 {
		t.Errorf("self_loop_count = %d, want 0", got.SelfLoopCount)
	}
}

func TestDescribeDirectedDag(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Describe(ctx, ax, dagFixture())
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.Directed || !got.IsDag {
		t.Errorf("dagFixture is a directed acyclic graph, got %+v", got)
	}
	// Directed density: 6 / (5*4) = 0.3; average degree 6/5 = 1.2.
	if !nearly(got.Density, 0.3, 1e-12) {
		t.Errorf("density = %v, want 0.3", got.Density)
	}
	if !nearly(got.AverageDegree, 1.2, 1e-12) {
		t.Errorf("average_degree = %v, want 1.2", got.AverageDegree)
	}
	if !got.IsConnected {
		t.Errorf("dagFixture is weakly connected")
	}
}

func TestDescribeCyclicDirectedIsNotDag(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B"}, [][3]any{{"A", "B", 1}, {"B", "A", 1}})
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.IsDag {
		t.Errorf("a 2-cycle is not a DAG: %+v", got)
	}
}

func TestDescribeSelfLoopIsNotDag(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B"}, [][3]any{{"A", "B", 1}, {"A", "A", 1}})
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.SelfLoopCount != 1 {
		t.Errorf("self_loop_count = %d, want 1", got.SelfLoopCount)
	}
	if got.IsDag {
		t.Errorf("a self-loop makes the graph cyclic: %+v", got)
	}
	// Density excludes the self-loop: 1 non-loop edge / (2*1) = 0.5.
	if !nearly(got.Density, 0.5, 1e-12) {
		t.Errorf("density = %v, want 0.5", got.Density)
	}
}

func TestDescribeDisconnected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"A", "B", "X", "Y", "Z"}, [][3]any{{"A", "B", 1}, {"X", "Y", 1}})
	got, err := nodes.Describe(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.IsConnected {
		t.Errorf("graph has three components, is_connected must be false")
	}
	if got.ComponentCount != 3 {
		t.Errorf("component_count = %d, want 3", got.ComponentCount)
	}
}

// An empty graph must produce zeroes, not a division by zero or a NaN.
func TestDescribeEmptyGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Describe(ctx, ax, mkGraph(false, nil, nil))
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.NodeCount != 0 || got.EdgeCount != 0 {
		t.Errorf("expected empty counts, got %+v", got)
	}
	if got.Density != 0 || got.AverageDegree != 0 {
		t.Errorf("empty graph must report zero density/degree, got %+v", got)
	}
	if got.IsConnected {
		t.Errorf("an empty graph is not connected")
	}
}

// A single vertex has no possible edges; density must be 0, not NaN.
func TestDescribeSingleNode(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Describe(ctx, ax, mkGraph(false, []string{"A"}, nil))
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Density != 0 {
		t.Errorf("density = %v, want 0", got.Density)
	}
	if !got.IsConnected || got.ComponentCount != 1 {
		t.Errorf("a single vertex is one connected component, got %+v", got)
	}
}

func TestDescribeNilGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Describe(ctx, ax, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a structured error for a nil graph")
	}
}
