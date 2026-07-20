package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
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
	// Directed density: 6 / (5*4) = 0.3.
	if !nearly(got.Density, 0.3, 1e-12) {
		t.Errorf("density = %v, want 0.3", got.Density)
	}
	// average_degree is the mean TOTAL degree (in+out), so 2*6/5 = 2.4.
	if !nearly(got.AverageDegree, 2.4, 1e-12) {
		t.Errorf("average_degree = %v, want 2.4", got.AverageDegree)
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

// TestDescribeAverageDegreeAgreesWithCentrality is the regression guard for a
// real inconsistency: Describe counted self-loops in average_degree while
// degree centrality ignored them, so the two nodes disagreed about the same
// graph. average_degree must always be the mean of Centrality's per-vertex
// degree scores.
func TestDescribeAverageDegreeAgreesWithCentrality(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	fixtures := map[string]*gen.Graph{
		"undirected simple": mstFixture(),
		"directed simple":   dagFixture(),
		"undirected with self-loops": mkGraph(false, []string{"a", "b"},
			[][3]any{{"a", "a", 1}, {"a", "b", 1}}),
		"directed with self-loops": mkGraph(true, []string{"a", "b"},
			[][3]any{{"a", "a", 1}, {"a", "b", 1}, {"b", "b", 1}}),
		"isolated vertices": mkGraph(false, []string{"a", "b", "c"}, nil),
	}
	for name, g := range fixtures {
		t.Run(name, func(t *testing.T) {
			stats, err := nodes.Describe(ctx, ax, g)
			if err != nil || stats.Error != "" {
				t.Fatalf("Describe: err=%v nodeErr=%s", err, stats.Error)
			}
			cent, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "degree"})
			if err != nil || cent.Error != "" {
				t.Fatalf("Centrality: err=%v nodeErr=%s", err, cent.Error)
			}
			sum := 0.0
			for _, s := range cent.Scores {
				sum += s.Score
			}
			want := 0.0
			if len(cent.Scores) > 0 {
				want = sum / float64(len(cent.Scores))
			}
			if !nearly(stats.AverageDegree, want, 1e-12) {
				t.Errorf("Describe.average_degree = %v but the mean of Centrality's degree scores is %v",
					stats.AverageDegree, want)
			}
		})
	}
}

// TestDescribeTotalWeight pins the field that lets a MinimumSpanningTree result
// be weighed by piping it into Describe.
func TestDescribeTotalWeight(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// mstFixture weights: 1+3+1+6+2+4+5 = 22.
	got, err := nodes.Describe(ctx, ax, mstFixture())
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.TotalWeight != 22 {
		t.Errorf("total_weight = %v, want 22", got.TotalWeight)
	}
	// The zero-means-one defaulting rule must be reflected here too.
	g := mkGraph(false, []string{"a", "b", "c"}, [][3]any{{"a", "b", 0}, {"b", "c", 0}})
	got2, err := nodes.Describe(ctx, ax, g)
	if err != nil || got2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got2.Error)
	}
	if got2.TotalWeight != 2 {
		t.Errorf("total_weight = %v, want 2 (each omitted weight defaults to 1)", got2.TotalWeight)
	}
}
