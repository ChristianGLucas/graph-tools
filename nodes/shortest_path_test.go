package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// dagFixture is a directed weighted graph whose shortest paths are easy to
// verify by hand: A->B->C->D->E costs 1+2+1+3 = 7, beating A->C (4) and
// B->D (5).
func dagFixture() *gen.Graph {
	return mkGraph(true, []string{"A", "B", "C", "D", "E"}, [][3]any{
		{"A", "B", 1}, {"A", "C", 4}, {"B", "C", 2},
		{"B", "D", 5}, {"C", "D", 1}, {"D", "E", 3},
	})
}

func TestShortestPath(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := dagFixture()

	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected node error: %s", got.Error)
	}
	// Golden: hand-computed.
	if !got.Found {
		t.Fatalf("expected a path from A to E")
	}
	if want := []string{"A", "B", "C", "D", "E"}; !eqStrings(got.Path, want) {
		t.Errorf("path = %v, want %v", got.Path, want)
	}
	if got.TotalWeight != 7 {
		t.Errorf("total_weight = %v, want 7", got.TotalWeight)
	}
	if got.HopCount != 4 {
		t.Errorf("hop_count = %d, want 4", got.HopCount)
	}
}

// TestShortestPathAgainstBruteForceOracle checks the node against an exhaustive
// enumeration of every simple path, for every ordered vertex pair.
func TestShortestPathAgainstBruteForceOracle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := dagFixture()
	ids := []string{"A", "B", "C", "D", "E"}

	for _, from := range ids {
		for _, to := range ids {
			if from == to {
				continue
			}
			wantW, wantPath := bruteForceShortestPath(g, from, to)
			got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: from, To: to})
			if err != nil || got.Error != "" {
				t.Fatalf("%s->%s: err=%v nodeErr=%s", from, to, err, got.Error)
			}
			reachable := !isInf(wantW)
			if got.Found != reachable {
				t.Errorf("%s->%s: found = %v, oracle says reachable = %v", from, to, got.Found, reachable)
				continue
			}
			if !reachable {
				continue
			}
			if got.TotalWeight != wantW {
				t.Errorf("%s->%s: weight = %v, oracle = %v (oracle path %v, got %v)",
					from, to, got.TotalWeight, wantW, wantPath, got.Path)
			}
			if int(got.HopCount) != len(got.Path)-1 {
				t.Errorf("%s->%s: hop_count %d inconsistent with path %v", from, to, got.HopCount, got.Path)
			}
		}
	}
}

func isInf(f float64) bool { return f > 1e300 }

// TestShortestPathNegativeWeights exercises the documented Bellman-Ford switch.
func TestShortestPathNegativeWeights(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// A->B costs 5 directly, but A->C->B costs 2 + (-4) = -2.
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 5}, {"A", "C", 2}, {"C", "B", -4},
	})
	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "B"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.TotalWeight != -2 {
		t.Errorf("total_weight = %v, want -2", got.TotalWeight)
	}
	if want := []string{"A", "C", "B"}; !eqStrings(got.Path, want) {
		t.Errorf("path = %v, want %v", got.Path, want)
	}
}

// TestShortestPathNegativeCycle asserts the documented structured error rather
// than a panic or a nonsense result.
func TestShortestPathNegativeCycle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1}, {"B", "C", -3}, {"C", "B", 1},
	})
	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "C"})
	if err != nil {
		t.Fatalf("expected a structured error, got a Go error: %v", err)
	}
	if got.Error == "" {
		t.Fatalf("expected a negative-cycle error, got %+v", got)
	}
}

func TestShortestPathUnreachable(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "X"}, [][3]any{{"A", "B", 1}})
	got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "X"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Found {
		t.Errorf("expected X to be unreachable, got %+v", got)
	}
	if len(got.Path) != 0 || got.TotalWeight != 0 {
		t.Errorf("unreachable result should be empty, got %+v", got)
	}
}

// TestShortestPathErrorPaths covers every rejection the node contract promises.
func TestShortestPathErrorPaths(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	cases := []struct {
		name string
		req  *gen.ShortestPathRequest
	}{
		{"nil request", nil},
		{"nil graph", &gen.ShortestPathRequest{From: "A", To: "B"}},
		{"unknown source", &gen.ShortestPathRequest{Graph: dagFixture(), From: "ZZ", To: "B"}},
		{"unknown target", &gen.ShortestPathRequest{Graph: dagFixture(), From: "A", To: "ZZ"}},
		{"empty node id", &gen.ShortestPathRequest{
			Graph: &gen.Graph{Nodes: []*gen.GraphNode{{Id: ""}}}, From: "A", To: "B"}},
		{"duplicate node id", &gen.ShortestPathRequest{
			Graph: &gen.Graph{Nodes: []*gen.GraphNode{{Id: "A"}, {Id: "A"}}}, From: "A", To: "A"}},
		{"edge to unknown node", &gen.ShortestPathRequest{
			Graph: &gen.Graph{
				Nodes: []*gen.GraphNode{{Id: "A"}},
				Edges: []*gen.GraphEdge{{From: "A", To: "ghost"}},
			}, From: "A", To: "A"}},
		{"duplicate edge", &gen.ShortestPathRequest{
			Graph: &gen.Graph{
				Nodes: []*gen.GraphNode{{Id: "A"}, {Id: "B"}},
				Edges: []*gen.GraphEdge{{From: "A", To: "B"}, {From: "A", To: "B"}},
			}, From: "A", To: "B"}},
		{"nil node entry", &gen.ShortestPathRequest{
			Graph: &gen.Graph{Nodes: []*gen.GraphNode{{Id: "A"}, nil}}, From: "A", To: "A"}},
		{"nil edge entry", &gen.ShortestPathRequest{
			Graph: &gen.Graph{Nodes: []*gen.GraphNode{{Id: "A"}}, Edges: []*gen.GraphEdge{nil}},
			From:  "A", To: "A"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nodes.ShortestPath(ctx, ax, tc.req)
			if err != nil {
				t.Fatalf("node returned a Go error instead of a structured one: %v", err)
			}
			if got.Error == "" {
				t.Fatalf("expected a structured error, got %+v", got)
			}
		})
	}
}

// TestShortestPathNonFiniteWeights: NaN/Inf must be rejected, not propagated.
func TestShortestPathNonFiniteWeights(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, w := range []float64{math_NaN(), math_Inf(1), math_Inf(-1)} {
		g := &gen.Graph{
			Directed: true,
			Nodes:    []*gen.GraphNode{{Id: "A"}, {Id: "B"}},
			Edges:    []*gen.GraphEdge{{From: "A", To: "B", Weight: w}},
		}
		got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "B"})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if got.Error == "" {
			t.Errorf("weight %v: expected a structured error, got %+v", w, got)
		}
	}
}

// TestShortestPathDeterminism: the same input must give a byte-identical result.
func TestShortestPathDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// A graph with two equal-cost paths A->B->D and A->C->D, where a
	// nondeterministic tie-break would show up as a flapping path.
	g := mkGraph(true, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"A", "C", 1}, {"B", "D", 1}, {"C", "D", 1},
	})
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "A", To: "D"})
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		if got.TotalWeight != 2 {
			t.Fatalf("weight = %v, want 2", got.TotalWeight)
		}
		if i == 0 {
			first = got.Path
			continue
		}
		if !eqStrings(got.Path, first) {
			t.Fatalf("nondeterministic tie-break: run %d gave %v, first run gave %v", i, got.Path, first)
		}
	}
}
