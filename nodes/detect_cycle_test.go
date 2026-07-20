package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

func TestDetectCycle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)

	// A directed 3-cycle.
	cyclic := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1},
	})
	got, err := nodes.DetectCycle(ctx, ax, cyclic)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.HasCycle {
		t.Fatalf("a directed 3-cycle must be detected: %+v", got)
	}
	if got.CycleCount != 1 {
		t.Errorf("cycle_count = %d, want 1", got.CycleCount)
	}
	// The witness must be a real closed walk in the graph.
	if !isWalk(cyclic, got.Cycle) {
		t.Errorf("cycle %v is not a walk in the graph", got.Cycle)
	}
	if len(got.Cycle) < 2 || got.Cycle[0] != got.Cycle[len(got.Cycle)-1] {
		t.Errorf("cycle %v must repeat its first vertex to close", got.Cycle)
	}

	// The acyclic fixture.
	got2, err := nodes.DetectCycle(ctx, ax, dagFixture())
	if err != nil || got2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got2.Error)
	}
	if got2.HasCycle {
		t.Errorf("dagFixture is acyclic: %+v", got2)
	}
	if got2.CycleCount != 0 {
		t.Errorf("cycle_count = %d, want 0", got2.CycleCount)
	}
	if len(got2.Cycle) != 0 {
		t.Errorf("acyclic graph must return no witness, got %v", got2.Cycle)
	}
}

// TestDetectCycleUndirectedAgainstEulerOracle checks cycle_count against the
// circuit rank |E| - |V| + components, computed independently.
func TestDetectCycleUndirectedAgainstEulerOracle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	fixtures := map[string]*gen.Graph{
		"mst fixture (7 edges, 5 nodes, connected)": mstFixture(),
		"tree": mkGraph(false, []string{"A", "B", "C"}, [][3]any{{"A", "B", 1}, {"B", "C", 1}}),
		"triangle plus isolated": mkGraph(false, []string{"A", "B", "C", "Z"},
			[][3]any{{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1}}),
		"two triangles": mkGraph(false, []string{"A", "B", "C", "X", "Y", "Z"}, [][3]any{
			{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1},
			{"X", "Y", 1}, {"Y", "Z", 1}, {"Z", "X", 1},
		}),
		"empty": mkGraph(false, nil, nil),
	}
	for name, g := range fixtures {
		t.Run(name, func(t *testing.T) {
			want := circuitRank(g)
			got, err := nodes.DetectCycle(ctx, ax, g)
			if err != nil || got.Error != "" {
				t.Fatalf("err=%v nodeErr=%s", err, got.Error)
			}
			if int(got.CycleCount) != want {
				t.Errorf("cycle_count = %d, Euler oracle = %d", got.CycleCount, want)
			}
			if got.HasCycle != (want > 0) {
				t.Errorf("has_cycle = %v but circuit rank is %d", got.HasCycle, want)
			}
			if got.HasCycle && !isWalk(g, got.Cycle) {
				t.Errorf("cycle %v is not a walk in the graph", got.Cycle)
			}
			if got.HasCycle && got.Cycle[0] != got.Cycle[len(got.Cycle)-1] {
				t.Errorf("cycle %v is not closed", got.Cycle)
			}
		})
	}
}

func TestDetectCycleSelfLoop(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, directed := range []bool{true, false} {
		g := mkGraph(directed, []string{"A", "B"}, [][3]any{{"A", "B", 1}, {"B", "B", 1}})
		got, err := nodes.DetectCycle(ctx, ax, g)
		if err != nil || got.Error != "" {
			t.Fatalf("directed=%v err=%v nodeErr=%s", directed, err, got.Error)
		}
		if !got.HasCycle {
			t.Errorf("directed=%v: a self-loop is a cycle: %+v", directed, got)
		}
		if want := []string{"B", "B"}; !eqStrings(got.Cycle, want) {
			t.Errorf("directed=%v: cycle = %v, want %v", directed, got.Cycle, want)
		}
	}
}

// A directed graph with a diamond shape has NO directed cycle, even though the
// underlying undirected graph does — the two must not be confused.
func TestDetectCycleDirectedDiamondIsAcyclic(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"A", "C", 1}, {"B", "D", 1}, {"C", "D", 1},
	})
	got, err := nodes.DetectCycle(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.HasCycle {
		t.Errorf("a directed diamond has no directed cycle: %+v", got)
	}
	if got.CycleCount != 0 {
		t.Errorf("cycle_count = %d, want 0", got.CycleCount)
	}
}

// Two disjoint directed cycles are counted separately.
func TestDetectCycleMultipleDirectedComponents(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "X", "Y"}, [][3]any{
		{"A", "B", 1}, {"B", "A", 1}, {"X", "Y", 1}, {"Y", "X", 1},
	})
	got, err := nodes.DetectCycle(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.CycleCount != 2 {
		t.Errorf("cycle_count = %d, want 2", got.CycleCount)
	}
	if !isWalk(g, got.Cycle) {
		t.Errorf("cycle %v is not a walk", got.Cycle)
	}
}

func TestDetectCycleDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, g := range []*gen.Graph{
		mstFixture(),
		mkGraph(true, []string{"A", "B", "C", "D"}, [][3]any{
			{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1}, {"C", "D", 1}, {"D", "A", 1},
		}),
	} {
		var first []string
		for i := 0; i < 40; i++ {
			got, err := nodes.DetectCycle(ctx, ax, g)
			if err != nil || got.Error != "" {
				t.Fatalf("err=%v nodeErr=%s", err, got.Error)
			}
			if i == 0 {
				first = got.Cycle
				continue
			}
			if !eqStrings(got.Cycle, first) {
				t.Fatalf("nondeterministic witness: run %d gave %v, first gave %v", i, got.Cycle, first)
			}
		}
	}
}

func TestDetectCycleNilGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.DetectCycle(ctx, ax, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a structured error for a nil graph")
	}
}
