package nodes_test

import (
	"context"
	"testing"

	"christiangeorgelucas/graph-tools/nodes"
)

func TestConnectedComponents(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// Three undirected islands: {A,B,C}, {X,Y}, {Z}.
	g := mkGraph(false, []string{"A", "B", "C", "X", "Y", "Z"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"X", "Y", 1},
	})
	got, err := nodes.ConnectedComponents(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Count != 3 {
		t.Fatalf("count = %d, want 3: %+v", got.Count, got.Components)
	}
	if got.StronglyConnected {
		t.Errorf("undirected input must not report strongly_connected")
	}
	want := [][]string{{"A", "B", "C"}, {"X", "Y"}, {"Z"}}
	for i, w := range want {
		if !eqStrings(got.Components[i].Nodes, w) {
			t.Errorf("component %d = %v, want %v", i, got.Components[i].Nodes, w)
		}
	}
}

// A directed graph reports STRONGLY connected components: A<->B is one
// component, while C is its own even though it is weakly attached to B.
func TestConnectedComponentsStronglyConnected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", 1}, {"B", "A", 1}, {"B", "C", 1},
	})
	got, err := nodes.ConnectedComponents(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.StronglyConnected {
		t.Errorf("directed input must report strongly_connected")
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2: %+v", got.Count, got.Components)
	}
	want := [][]string{{"A", "B"}, {"C"}}
	for i, w := range want {
		if !eqStrings(got.Components[i].Nodes, w) {
			t.Errorf("component %d = %v, want %v", i, got.Components[i].Nodes, w)
		}
	}
}

// Every vertex must land in exactly one component — an independent partition check.
func TestConnectedComponentsPartitionsEveryVertex(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mstFixture()
	got, err := nodes.ConnectedComponents(ctx, ax, g)
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	seen := map[string]int{}
	for _, c := range got.Components {
		for _, id := range c.Nodes {
			seen[id]++
		}
	}
	if len(seen) != len(g.Nodes) {
		t.Errorf("components cover %d vertices, graph has %d", len(seen), len(g.Nodes))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("vertex %s appears in %d components", id, n)
		}
	}
}

func TestConnectedComponentsEmptyGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.ConnectedComponents(ctx, ax, mkGraph(false, nil, nil))
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Count != 0 || len(got.Components) != 0 {
		t.Errorf("empty graph should have no components, got %+v", got)
	}
}

func TestConnectedComponentsDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"e", "d", "c", "b", "a"}, [][3]any{{"a", "b", 1}, {"c", "d", 1}})
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.ConnectedComponents(ctx, ax, g)
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		var flat []string
		for _, c := range got.Components {
			flat = append(flat, c.Nodes...)
			flat = append(flat, "|")
		}
		if i == 0 {
			first = flat
			continue
		}
		if !eqStrings(flat, first) {
			t.Fatalf("nondeterministic components: run %d gave %v, first gave %v", i, flat, first)
		}
	}
}

func TestConnectedComponentsNilGraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.ConnectedComponents(ctx, ax, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected a structured error for a nil graph")
	}
}
