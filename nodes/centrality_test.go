package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// pathGraph is the undirected unweighted path A-B-C-D, whose centrality values
// all have exact closed forms.
func pathGraph() *gen.Graph {
	return mkGraph(false, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "D", 1},
	})
}

func scoreMap(r *gen.CentralityResult) map[string]float64 {
	m := map[string]float64{}
	for _, s := range r.Scores {
		m[s.Node] = s.Score
	}
	return m
}

func TestCentrality(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// Star: centre C joined to three leaves. Degrees are 3, 1, 1, 1.
	g := mkGraph(false, []string{"C", "L1", "L2", "L3"}, [][3]any{
		{"C", "L1", 1}, {"C", "L2", 1}, {"C", "L3", 1},
	})
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if got.Measure != "degree" {
		t.Errorf("measure = %q, want the documented default %q", got.Measure, "degree")
	}
	m := scoreMap(got)
	if m["C"] != 3 {
		t.Errorf("degree(C) = %v, want 3", m["C"])
	}
	for _, l := range []string{"L1", "L2", "L3"} {
		if m[l] != 1 {
			t.Errorf("degree(%s) = %v, want 1", l, m[l])
		}
	}
}

// TestCentralityHarmonicClosedForm is an INDEPENDENT ORACLE: harmonic
// centrality is by definition the sum of the reciprocals of the distances to
// every other vertex. On the path A-B-C-D those distances are known exactly.
func TestCentralityHarmonicClosedForm(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: pathGraph(), Measure: "harmonic"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	want := map[string]float64{
		"A": 1.0/1 + 1.0/2 + 1.0/3, // distances 1,2,3
		"B": 1.0/1 + 1.0/1 + 1.0/2, // distances 1,1,2
		"C": 1.0/1 + 1.0/1 + 1.0/2,
		"D": 1.0/1 + 1.0/2 + 1.0/3,
	}
	for k, w := range want {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("harmonic(%s) = %v, closed form = %v", k, m[k], w)
		}
	}
}

// TestCentralityEccentricityClosedForm: eccentricity is the distance to the
// farthest reachable vertex — 3, 2, 2, 3 on the path A-B-C-D.
func TestCentralityEccentricityClosedForm(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: pathGraph(), Measure: "eccentricity"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	for k, w := range map[string]float64{"A": 3, "B": 2, "C": 2, "D": 3} {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("eccentricity(%s) = %v, want %v", k, m[k], w)
		}
	}
}

// TestCentralityClosenessClosedForm: closeness is the reciprocal of the summed
// distance to every other vertex — 1/6, 1/4, 1/4, 1/6 on the path A-B-C-D.
func TestCentralityClosenessClosedForm(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: pathGraph(), Measure: "closeness"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	for k, w := range map[string]float64{
		"A": 1.0 / 6, "B": 1.0 / 4, "C": 1.0 / 4, "D": 1.0 / 6,
	} {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("closeness(%s) = %v, closed form = %v", k, m[k], w)
		}
	}
}

// TestCentralityBetweennessStructure checks the properties betweenness must
// have by definition: endpoints of a path lie on no shortest path between
// others, the two interior vertices are symmetric, and a star's centre lies on
// every leaf-to-leaf path.
func TestCentralityBetweennessStructure(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: pathGraph(), Measure: "betweenness"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	if m["A"] != 0 || m["D"] != 0 {
		t.Errorf("path endpoints must have zero betweenness: A=%v D=%v", m["A"], m["D"])
	}
	if m["B"] <= 0 || m["C"] <= 0 {
		t.Errorf("path interior must have positive betweenness: B=%v C=%v", m["B"], m["C"])
	}
	if !nearly(m["B"], m["C"], 1e-9) {
		t.Errorf("B and C are symmetric but got B=%v C=%v", m["B"], m["C"])
	}

	star := mkGraph(false, []string{"C", "L1", "L2", "L3"}, [][3]any{
		{"C", "L1", 1}, {"C", "L2", 1}, {"C", "L3", 1},
	})
	got2, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: star, Measure: "betweenness"})
	if err != nil || got2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got2.Error)
	}
	m2 := scoreMap(got2)
	if m2["C"] <= 0 {
		t.Errorf("star centre must have positive betweenness, got %v", m2["C"])
	}
	for _, l := range []string{"L1", "L2", "L3"} {
		if m2[l] != 0 {
			t.Errorf("star leaf %s must have zero betweenness, got %v", l, m2[l])
		}
	}
}

// Directed degree centrality counts both in- and out-edges.
func TestCentralityDirectedDegree(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: dagFixture(), Measure: "degree"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	// dagFixture: A->B, A->C, B->C, B->D, C->D, D->E.
	for k, w := range map[string]float64{"A": 2, "B": 3, "C": 3, "D": 3, "E": 1} {
		if m[k] != w {
			t.Errorf("degree(%s) = %v, want %v", k, m[k], w)
		}
	}
}

func TestCentralityErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	cases := map[string]*gen.CentralityRequest{
		"nil request":     nil,
		"nil graph":       {Measure: "degree"},
		"unknown measure": {Graph: pathGraph(), Measure: "eigenvector"},
	}
	for name, req := range cases {
		got, err := nodes.Centrality(ctx, ax, req)
		if err != nil {
			t.Fatalf("%s: unexpected Go error %v", name, err)
		}
		if got.Error == "" {
			t.Errorf("%s: expected a structured error, got %+v", name, got)
		}
	}
}

// Negative weights make the all-pairs measures undefined and must be rejected,
// not silently mis-computed by Dijkstra.
func TestCentralityRejectsNegativeWeightsForAllPairsMeasures(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(false, []string{"A", "B", "C"}, [][3]any{
		{"A", "B", -1}, {"B", "C", 2},
	})
	for _, m := range []string{"betweenness", "closeness", "harmonic", "eccentricity"} {
		got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: m})
		if err != nil {
			t.Fatalf("%s: unexpected Go error %v", m, err)
		}
		if got.Error == "" {
			t.Errorf("%s: negative weights must be rejected, got %+v", m, got)
		}
	}
	// Degree centrality is purely structural and stays available.
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "degree"})
	if err != nil || got.Error != "" {
		t.Fatalf("degree should still work with negative weights: err=%v nodeErr=%s", err, got.Error)
	}
}

// The all-pairs measures must refuse graphs above the documented size bound
// rather than attempting an O(V^2)-memory computation.
func TestCentralityQuadraticBound(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	var ids []string
	for i := 0; i < 700; i++ {
		ids = append(ids, "n"+itoa(i))
	}
	g := mkGraph(false, ids, nil)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "betweenness"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected the 600-node all-pairs bound to fire, got %d scores", len(got.Scores))
	}
	// Degree centrality has no such bound.
	got2, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "degree"})
	if err != nil || got2.Error != "" {
		t.Fatalf("degree must still work on a large graph: err=%v nodeErr=%s", err, got2.Error)
	}
	if len(got2.Scores) != 700 {
		t.Errorf("got %d degree scores, want 700", len(got2.Scores))
	}
}

func TestCentralityDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, measure := range []string{"degree", "betweenness", "closeness", "harmonic", "eccentricity"} {
		var first []float64
		for i := 0; i < 25; i++ {
			got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: mstFixture(), Measure: measure})
			if err != nil || got.Error != "" {
				t.Fatalf("%s: err=%v nodeErr=%s", measure, err, got.Error)
			}
			var cur []float64
			for _, s := range got.Scores {
				cur = append(cur, s.Score)
			}
			if i == 0 {
				first = cur
				continue
			}
			for j := range cur {
				if cur[j] != first[j] {
					t.Fatalf("%s nondeterministic: run %d gave %v, first gave %v", measure, i, cur, first)
				}
			}
		}
	}
}
