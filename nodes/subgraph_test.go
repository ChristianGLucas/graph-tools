package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

func TestSubgraph(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mstFixture() // A,B,C,D,E with 7 edges
	got, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{Graph: g, Nodes: []string{"A", "B", "C"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Nodes) != 3 {
		t.Fatalf("kept %d nodes, want 3", len(got.Nodes))
	}
	// Induced edges among {A,B,C}: A-B, A-C, B-C.
	if len(got.Edges) != 3 {
		t.Errorf("kept %d edges, want 3: %+v", len(got.Edges), got.Edges)
	}
	for _, e := range got.Edges {
		if e.From == "D" || e.To == "D" || e.From == "E" || e.To == "E" {
			t.Errorf("edge %s-%s references a dropped vertex", e.From, e.To)
		}
	}
	if got.Directed != g.Directed {
		t.Errorf("subgraph must preserve directedness")
	}
}

// Weights and labels must survive the projection unchanged.
func TestSubgraphPreservesWeightsAndLabels(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{
		Directed: true,
		Nodes: []*gen.GraphNode{
			{Id: "A", Label: "alpha"}, {Id: "B", Label: "beta"}, {Id: "C", Label: "gamma"},
		},
		Edges: []*gen.GraphEdge{
			{From: "A", To: "B", Weight: 2.5},
			{From: "B", To: "C", Weight: 7},
			{From: "A", To: "C", Weight: 0, ExplicitZeroWeight: true},
		},
	}
	got, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{Graph: g, Nodes: []string{"A", "B"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Nodes[0].Label != "alpha" || got.Nodes[1].Label != "beta" {
		t.Errorf("labels not preserved: %+v", got.Nodes)
	}
	if len(got.Edges) != 1 || got.Edges[0].Weight != 2.5 {
		t.Errorf("edge weight not preserved: %+v", got.Edges)
	}
}

// The zero-weight flag must survive, otherwise a zero-cost edge silently
// becomes a unit-cost edge downstream.
func TestSubgraphPreservesExplicitZeroWeight(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := &gen.Graph{
		Nodes: []*gen.GraphNode{{Id: "A"}, {Id: "B"}},
		Edges: []*gen.GraphEdge{{From: "A", To: "B", Weight: 0, ExplicitZeroWeight: true}},
	}
	got, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{Graph: g, Nodes: []string{"A", "B"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Edges[0].ExplicitZeroWeight {
		t.Fatalf("explicit_zero_weight was dropped: %+v", got.Edges[0])
	}
	// Prove it downstream: the path A->B must still cost 0, not 1.
	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: got, From: "A", To: "B"})
	if err != nil || sp.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, sp.Error)
	}
	if sp.TotalWeight != 0 {
		t.Errorf("zero-weight edge became weight %v after the round trip", sp.TotalWeight)
	}
}

// TestSubgraphComposes runs the emitted Graph straight through other nodes.
func TestSubgraphComposes(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	sub, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{
		Graph: mstFixture(), Nodes: []string{"A", "B", "C"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stats, err := nodes.Describe(ctx, ax, sub)
	if err != nil || stats.Error != "" {
		t.Fatalf("Describe: err=%v nodeErr=%s", err, stats.Error)
	}
	if stats.NodeCount != 3 || stats.EdgeCount != 3 {
		t.Errorf("subgraph stats = %d nodes / %d edges, want 3 / 3", stats.NodeCount, stats.EdgeCount)
	}
	// A triangle is a complete graph: density 1.
	if !nearly(stats.Density, 1, 1e-12) {
		t.Errorf("density = %v, want 1 for a triangle", stats.Density)
	}
	cyc, err := nodes.DetectCycle(ctx, ax, sub)
	if err != nil || cyc.Error != "" {
		t.Fatalf("DetectCycle: err=%v nodeErr=%s", err, cyc.Error)
	}
	if !cyc.HasCycle {
		t.Errorf("a triangle contains a cycle, got %+v", cyc)
	}
}

func TestSubgraphEmptySelection(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{Graph: mstFixture()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Nodes) != 0 || len(got.Edges) != 0 {
		t.Errorf("empty selection must yield an empty graph, got %+v", got)
	}
}

func TestSubgraphErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for name, req := range map[string]*gen.SubgraphRequest{
		"nil request":  nil,
		"nil graph":    {Nodes: []string{"A"}},
		"unknown node": {Graph: mstFixture(), Nodes: []string{"A", "nope"}},
	} {
		if _, err := nodes.Subgraph(ctx, ax, req); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSubgraphDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	var first []string
	for i := 0; i < 25; i++ {
		got, err := nodes.Subgraph(ctx, ax, &gen.SubgraphRequest{
			Graph: mstFixture(), Nodes: []string{"E", "C", "A", "B"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var cur []string
		for _, n := range got.Nodes {
			cur = append(cur, n.Id)
		}
		for _, e := range got.Edges {
			cur = append(cur, e.From+"-"+e.To)
		}
		if i == 0 {
			first = cur
			continue
		}
		if !eqStrings(cur, first) {
			t.Fatalf("nondeterministic subgraph: run %d gave %v, first gave %v", i, cur, first)
		}
	}
}
