package nodes_test

import (
	"context"
	"testing"

	"christiangeorgelucas/graph-tools/nodes"
)

func TestTopologicalSort(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.TopologicalSort(ctx, ax, dagFixture())
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.IsDag {
		t.Fatalf("dagFixture is acyclic; got is_dag=false")
	}
	// Golden: the only ordering consistent with the fixture's edges.
	if want := []string{"A", "B", "C", "D", "E"}; !eqStrings(got.Order, want) {
		t.Errorf("order = %v, want %v", got.Order, want)
	}
}

// TestTopologicalSortInvariant independently verifies the defining property of
// a topological order: every edge points forwards.
func TestTopologicalSortInvariant(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true,
		[]string{"deploy", "build", "test", "lint", "checkout", "docs"},
		[][3]any{
			{"checkout", "build", 1}, {"checkout", "lint", 1}, {"build", "test", 1},
			{"test", "deploy", 1}, {"lint", "deploy", 1}, {"checkout", "docs", 1},
		})
	got, err := nodes.TopologicalSort(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.IsDag {
		t.Fatalf("expected a DAG")
	}
	if len(got.Order) != 6 {
		t.Fatalf("order = %v, want all 6 vertices", got.Order)
	}
	idx := map[string]int{}
	for i, id := range got.Order {
		if _, dup := idx[id]; dup {
			t.Fatalf("vertex %s appears twice in %v", id, got.Order)
		}
		idx[id] = i
	}
	for _, e := range g.Edges {
		if idx[e.From] >= idx[e.To] {
			t.Errorf("edge %s->%s violates the ordering %v", e.From, e.To, got.Order)
		}
	}
}

func TestTopologicalSortCyclic(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1},
	})
	got, err := nodes.TopologicalSort(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.IsDag {
		t.Errorf("a 3-cycle is not a DAG: %+v", got)
	}
	if len(got.Order) != 0 {
		t.Errorf("order must be empty for a cyclic graph, got %v", got.Order)
	}
}

// A self-loop is a cycle even though it is held outside the gonum structure.
func TestTopologicalSortSelfLoop(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B"}, [][3]any{{"A", "B", 1}, {"B", "B", 1}})
	got, err := nodes.TopologicalSort(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.IsDag {
		t.Errorf("a self-loop makes the graph cyclic: %+v", got)
	}
}

func TestTopologicalSortRejectsUndirected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"A", "B"}, [][3]any{{"A", "B", 1}})
	got, err := nodes.TopologicalSort(ctx, ax, g)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected undirected input to be rejected, got %+v", got)
	}
}

func TestTopologicalSortDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// Six mutually-unordered vertices: every permutation is a valid topological
	// order, so only an explicit tie-break makes the output stable.
	g := mkGraph(true, []string{"f", "e", "d", "c", "b", "a"}, nil)
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.TopologicalSort(ctx, ax, g)
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		if i == 0 {
			first = got.Order
			continue
		}
		if !eqStrings(got.Order, first) {
			t.Fatalf("nondeterministic order: run %d gave %v, first gave %v", i, got.Order, first)
		}
	}
}

func TestTopologicalSortNilGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.TopologicalSort(ctx, ax, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a structured error for a nil graph")
	}
}
