package nodes_test

import (
	"context"
	"math"
	"testing"
	"time"

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

// ── Directed / asymmetric coverage ───────────────────────────────────────────
//
// Every all-pairs oracle above runs on an UNDIRECTED path, which is
// direction-symmetric — so a transposed measure produces identical numbers and
// the bug cancels. That blind spot hid a real defect: eccentricity was computed
// with gonum's incoming convention while the docs promised the outgoing one.
// These tests use directed, asymmetric graphs where the two differ.

// directedPath is a -> b -> c, unweighted. Out-distances: a reaches b (1) and
// c (2); b reaches c (1); c reaches nothing.
func directedPath() *gen.Graph {
	return mkGraph(true, []string{"a", "b", "c"}, [][3]any{{"a", "b", 1}, {"b", "c", 1}})
}

// TestCentralityEccentricityIsOutgoingOnDirectedGraphs is the regression guard.
// Eccentricity is the distance to the FARTHEST VERTEX THIS VERTEX CAN REACH, so
// on a -> b -> c the source a scores 2 and the sink c scores 0. The transposed
// (incoming) reading would give exactly the reverse, which is what shipped.
func TestCentralityEccentricityIsOutgoingOnDirectedGraphs(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: directedPath(), Measure: "eccentricity"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	for k, w := range map[string]float64{"a": 2, "b": 1, "c": 0} {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("eccentricity(%s) = %v, want %v (a reaches the whole graph; c reaches nothing)", k, m[k], w)
		}
	}
}

// TestCentralityEccentricityWeightedDirected uses distinct weights so the
// answer cannot be produced by a symmetric accident.
func TestCentralityEccentricityWeightedDirected(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// a -> b costs 0.5, b -> c costs 10. a reaches c at 10.5.
	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{{"a", "b", 0.5}, {"b", "c", 10}})
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "eccentricity"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	for k, w := range map[string]float64{"a": 10.5, "b": 10, "c": 0} {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("eccentricity(%s) = %v, want %v", k, m[k], w)
		}
	}
}

// TestCentralityClosenessIsIncomingOnDirectedGraphs pins the OTHER convention,
// so the two can never silently drift into agreement. Closeness is the
// reciprocal of the summed distance FROM every vertex that can reach it: on
// a -> b -> c, c is reached from a (2) and b (1), giving 1/3.
func TestCentralityClosenessIsIncomingOnDirectedGraphs(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: directedPath(), Measure: "closeness"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	if !nearly(m["c"], 1.0/3, 1e-9) {
		t.Errorf("closeness(c) = %v, want 1/3 (reached from a at 2 and b at 1)", m["c"])
	}
	if !nearly(m["b"], 1.0/1, 1e-9) {
		t.Errorf("closeness(b) = %v, want 1 (reached from a at 1)", m["b"])
	}
	// `a` is reached by nobody: infinite farness, reported as a finite 0.
	if m["a"] != 0 {
		t.Errorf("closeness(a) = %v, want 0 (nothing reaches a)", m["a"])
	}
}

// TestCentralityScoresAreAlwaysFinite is the regression guard for a real
// defect: an unreachable vertex produced +Inf, which serialises out of a proto
// `double` field as the STRING "Infinity" and poisons downstream arithmetic.
func TestCentralityScoresAreAlwaysFinite(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	fixtures := map[string]*gen.Graph{
		"directed source with no in-edges": directedPath(),
		"isolated vertex":                  mkGraph(false, []string{"a", "b", "z"}, [][3]any{{"a", "b", 1}}),
		"all isolated":                     mkGraph(false, []string{"x", "y"}, nil),
		"disconnected halves": mkGraph(false, []string{"a", "b", "x", "y"},
			[][3]any{{"a", "b", 1}, {"x", "y", 1}}),
	}
	for name, g := range fixtures {
		for _, measure := range []string{"degree", "betweenness", "closeness", "harmonic", "eccentricity"} {
			got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: measure})
			if err != nil || got.Error != "" {
				t.Fatalf("%s/%s: err=%v nodeErr=%s", name, measure, err, got.Error)
			}
			for _, s := range got.Scores {
				if math.IsInf(s.Score, 0) || math.IsNaN(s.Score) {
					t.Errorf("%s/%s: score(%s) = %v, must be finite", name, measure, s.Node, s.Score)
				}
			}
		}
	}
}

// TestCentralityDegreeCountsSelfLoops pins the standard convention.
func TestCentralityDegreeCountsSelfLoops(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// Undirected: a has a self-loop (+2) and one edge to b (+1) = 3; b = 1.
	g := mkGraph(false, []string{"a", "b"}, [][3]any{{"a", "a", 1}, {"a", "b", 1}})
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "degree"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	if m["a"] != 3 {
		t.Errorf("degree(a) = %v, want 3 (self-loop counts 2, plus the edge to b)", m["a"])
	}
	if m["b"] != 1 {
		t.Errorf("degree(b) = %v, want 1", m["b"])
	}
}

// The all-pairs measures must refuse a graph whose VERTEX*EDGE product is too
// large even when the vertex count alone is within bounds — a dense 600-vertex
// graph passes the vertex cap while costing over a minute of CPU.
func TestCentralityQuadraticProductBound(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// 600 vertices, densely connected: well past the product budget.
	var ids []string
	for i := 0; i < 600; i++ {
		ids = append(ids, "n"+itoa(i))
	}
	var edges [][3]any
	for i := 0; i < 600 && len(edges) < 5000; i++ {
		for j := i + 1; j < 600 && len(edges) < 5000; j++ {
			edges = append(edges, [3]any{ids[i], ids[j], 1})
		}
	}
	g := mkGraph(false, ids, edges)
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "betweenness"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error == "" {
		t.Errorf("expected the nodes*edges product bound to fire on 600 nodes * %d edges", len(edges))
	}
}

// TestCentralityBetweennessMatchesStandardConvention pins the unnormalised
// Brandes / networkx value. gonum accumulates over ORDERED pairs, so an
// undirected graph came out at exactly twice the conventional figure; a caller
// cross-checking against any standard tool would have seen a silent factor of 2.
func TestCentralityBetweennessMatchesStandardConvention(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// Path plus chord: a-b, b-c, c-d, d-e, b-d.
	// networkx betweenness_centrality(normalized=False) = {a:0,b:3,c:0,d:3,e:0}.
	g := mkGraph(false, []string{"a", "b", "c", "d", "e"}, [][3]any{
		{"a", "b", 1}, {"b", "c", 1}, {"c", "d", 1}, {"d", "e", 1}, {"b", "d", 1},
	})
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "betweenness"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := scoreMap(got)
	for k, w := range map[string]float64{"a": 0, "b": 3, "c": 0, "d": 3, "e": 0} {
		if !nearly(m[k], w, 1e-9) {
			t.Errorf("betweenness(%s) = %v, networkx unnormalised = %v", k, m[k], w)
		}
	}

	// A star's centre lies on all 3 unordered leaf pairs.
	star := mkGraph(false, []string{"c", "l1", "l2", "l3"}, [][3]any{
		{"c", "l1", 1}, {"c", "l2", 1}, {"c", "l3", 1},
	})
	got2, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: star, Measure: "betweenness"})
	if err != nil || got2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got2.Error)
	}
	if m2 := scoreMap(got2); !nearly(m2["c"], 3, 1e-9) {
		t.Errorf("betweenness(star centre) = %v, want 3", m2["c"])
	}

	// Directed already matches networkx and must NOT be halved: on a->b->c,
	// b lies on the single ordered pair (a,c).
	dg := mkGraph(true, []string{"a", "b", "c"}, [][3]any{{"a", "b", 1}, {"b", "c", 1}})
	got3, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: dg, Measure: "betweenness"})
	if err != nil || got3.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got3.Error)
	}
	if m3 := scoreMap(got3); !nearly(m3["b"], 1, 1e-9) {
		t.Errorf("directed betweenness(b) = %v, want 1", m3["b"])
	}
}

// TestCentralityBetweennessStaysFastOnTiedWeights is the regression guard for
// an exponential blow-up: the WEIGHTED betweenness path enumerated every
// shortest path, so a graph with many tied shortest paths did not finish in 45s
// and allocated gigabytes. Brandes on the unweighted topology has no such term.
func TestCentralityBetweennessStaysFastOnTiedWeights(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// 400 vertices, ~1600 uniform-weight edges: maximally tied shortest paths.
	var ids []string
	for i := 0; i < 400; i++ {
		ids = append(ids, "n"+itoa(i))
	}
	var edges [][3]any
	seen := map[[2]int]bool{}
	for s := 1; len(edges) < 1600 && s < 400; s++ {
		for i := 0; i < 400 && len(edges) < 1600; i++ {
			j := (i + s) % 400
			if i >= j || seen[[2]int{i, j}] {
				continue
			}
			seen[[2]int{i, j}] = true
			edges = append(edges, [3]any{ids[i], ids[j], 2.0})
		}
	}
	g := mkGraph(false, ids, edges)

	start := time.Now()
	got, err := nodes.Centrality(ctx, ax, &gen.CentralityRequest{Graph: g, Measure: "betweenness"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected node error: %s", got.Error)
	}
	if elapsed > 30*time.Second {
		t.Errorf("betweenness on %d tied-weight edges took %v — the path-enumeration blow-up is back", len(edges), elapsed)
	}
	if len(got.Scores) != 400 {
		t.Errorf("got %d scores, want 400", len(got.Scores))
	}
	t.Logf("betweenness over %d uniform-weight edges: %v", len(edges), elapsed.Round(time.Millisecond))
}
