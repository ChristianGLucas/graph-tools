package nodes_test

import (
	"context"
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
	got, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: g, Directed: false})
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
	got, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: g, Directed: true})
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
	und, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: directed, Directed: false})
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

	// An UNDIRECTED graph can now reach TopologicalSort (it will be cyclic by
	// construction, which the node documents).
	undirected := mkGraph(false, []string{"x", "y"}, [][3]any{{"x", "y", 1}})
	if r, err := nodes.TopologicalSort(ctx, ax, undirected); err != nil || r.Error == "" {
		t.Fatalf("precondition: TopologicalSort must reject undirected input")
	}
	dir, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: undirected, Directed: true})
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
	got, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: g, Directed: false})
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

func TestOrientIsIdentityWhenDirectionAlreadyMatches(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, directed := range []bool{true, false} {
		g := mkGraph(directed, []string{"a", "b"}, [][3]any{{"a", "b", 2}})
		got, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: g, Directed: directed})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Directed != directed || len(got.Edges) != 1 || got.Edges[0].Weight != 2 {
			t.Errorf("directed=%v: expected an unchanged graph, got %+v", directed, got)
		}
	}
}

func TestOrientDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b", "c", "d"}, [][3]any{
		{"a", "b", 5}, {"b", "a", 2}, {"c", "d", 1}, {"d", "c", 1}, {"a", "c", 3},
	})
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.Orient(ctx, ax, &gen.OrientRequest{Graph: g, Directed: false})
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
	for name, req := range map[string]*gen.OrientRequest{
		"nil request": nil,
		"nil graph":   {Directed: true},
		"bad graph":   {Graph: &gen.Graph{Nodes: []*gen.GraphNode{{Id: "a"}, {Id: "a"}}}, Directed: true},
	} {
		if _, err := nodes.Orient(ctx, ax, req); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
