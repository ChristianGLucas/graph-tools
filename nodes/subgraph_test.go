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
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if len(got.Graph.Nodes) != 3 {
		t.Fatalf("kept %d nodes, want 3", len(got.Graph.Nodes))
	}
	// Induced edges among {A,B,C}: A-B, A-C, B-C.
	if len(got.Graph.Edges) != 3 {
		t.Errorf("kept %d edges, want 3: %+v", len(got.Graph.Edges), got.Graph.Edges)
	}
	for _, e := range got.Graph.Edges {
		if e.From == "D" || e.To == "D" || e.From == "E" || e.To == "E" {
			t.Errorf("edge %s-%s references a dropped vertex", e.From, e.To)
		}
	}
	if got.DroppedNodeCount != 2 {
		t.Errorf("dropped_node_count = %d, want 2", got.DroppedNodeCount)
	}
	if got.DroppedEdgeCount != 4 {
		t.Errorf("dropped_edge_count = %d, want 4", got.DroppedEdgeCount)
	}
	if got.Graph.Directed != g.Directed {
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
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Graph.Nodes[0].Label != "alpha" || got.Graph.Nodes[1].Label != "beta" {
		t.Errorf("labels not preserved: %+v", got.Graph.Nodes)
	}
	if len(got.Graph.Edges) != 1 || got.Graph.Edges[0].Weight != 2.5 {
		t.Errorf("edge weight not preserved: %+v", got.Graph.Edges)
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
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !got.Graph.Edges[0].ExplicitZeroWeight {
		t.Fatalf("explicit_zero_weight was dropped: %+v", got.Graph.Edges[0])
	}
	// Prove it downstream: the path A->B must still cost 0, not 1.
	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: got.Graph, From: "A", To: "B"})
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
	if err != nil || sub.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, sub.Error)
	}
	stats, err := nodes.Describe(ctx, ax, sub.Graph)
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
	cyc, err := nodes.DetectCycle(ctx, ax, sub.Graph)
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
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if len(got.Graph.Nodes) != 0 || len(got.Graph.Edges) != 0 {
		t.Errorf("empty selection must yield an empty graph, got %+v", got.Graph)
	}
	if got.DroppedNodeCount != 5 || got.DroppedEdgeCount != 7 {
		t.Errorf("dropped counts = %d/%d, want 5/7", got.DroppedNodeCount, got.DroppedEdgeCount)
	}
}

func TestSubgraphErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for name, req := range map[string]*gen.SubgraphRequest{
		"nil request":  nil,
		"nil graph":    {Nodes: []string{"A"}},
		"unknown node": {Graph: mstFixture(), Nodes: []string{"A", "nope"}},
	} {
		got, err := nodes.Subgraph(ctx, ax, req)
		if err != nil {
			t.Fatalf("%s: unexpected Go error %v", name, err)
		}
		if got.Error == "" {
			t.Errorf("%s: expected a structured error, got %+v", name, got)
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
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		var cur []string
		for _, n := range got.Graph.Nodes {
			cur = append(cur, n.Id)
		}
		for _, e := range got.Graph.Edges {
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
