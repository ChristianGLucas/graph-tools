package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// TestPageRankCycleClosedForm is an INDEPENDENT ORACLE: on a directed cycle
// every vertex is structurally identical, so the unique stationary distribution
// is exactly 1/n for every vertex, for ANY damping factor. This is derived from
// the PageRank definition, not from the implementation.
func TestPageRankCycleClosedForm(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, n := range []int{2, 3, 5, 8} {
		ids := make([]string, n)
		var edges [][3]any
		for i := 0; i < n; i++ {
			ids[i] = string(rune('A' + i))
		}
		for i := 0; i < n; i++ {
			edges = append(edges, [3]any{ids[i], ids[(i+1)%n], 1})
		}
		g := mkGraph(true, ids, edges)
		got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
		if err != nil || got.Error != "" {
			t.Fatalf("n=%d err=%v nodeErr=%s", n, err, got.Error)
		}
		want := 1 / float64(n)
		for _, s := range got.Scores {
			if !nearly(s.Score, want, 1e-6) {
				t.Errorf("cycle n=%d: score(%s) = %v, closed form = %v", n, s.Node, s.Score, want)
			}
		}
	}
}

// TestPageRankSumsToOne is a definitional invariant of a probability
// distribution, checked on an asymmetric graph.
func TestPageRankSumsToOne(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1}, {"D", "C", 1}, {"A", "C", 1},
	})
	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	sum := 0.0
	for _, s := range got.Scores {
		if s.Score < 0 {
			t.Errorf("score(%s) = %v, must be non-negative", s.Node, s.Score)
		}
		sum += s.Score
	}
	if !nearly(sum, 1, 1e-5) {
		t.Errorf("scores sum to %v, want 1", sum)
	}
	// C is pointed at by A, B and D; D is pointed at by nobody. C must outrank D.
	score := map[string]float64{}
	for _, s := range got.Scores {
		score[s.Node] = s.Score
	}
	if score["C"] <= score["D"] {
		t.Errorf("C (3 inbound links) should outrank D (0 inbound): C=%v D=%v", score["C"], score["D"])
	}
}

func TestPageRankDefaultsAndValidation(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B"}, [][3]any{{"A", "B", 1}, {"B", "A", 1}})

	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	// The documented default damping factor.
	if got.Damping != 0.85 {
		t.Errorf("damping = %v, want the documented default 0.85", got.Damping)
	}

	for _, bad := range []float64{-0.5, 1, 1.5} {
		r, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g, Damping: bad})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if r.Error == "" {
			t.Errorf("damping %v must be rejected, got %+v", bad, r)
		}
	}
}

func TestPageRankErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for name, req := range map[string]*gen.PageRankRequest{
		"nil request": nil,
		"nil graph":   {},
		"empty graph": {Graph: mkGraph(true, nil, nil)},
	} {
		got, err := nodes.PageRank(ctx, ax, req)
		if err != nil {
			t.Fatalf("%s: unexpected Go error %v", name, err)
		}
		if got.Error == "" {
			t.Errorf("%s: expected a structured error, got %+v", name, got)
		}
	}
}

// TestPageRankScoresAreRounded pins the documented 6-decimal contract.
func TestPageRankScoresAreRounded(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C", "D"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1}, {"D", "C", 1}, {"A", "C", 1},
	})
	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	for _, s := range got.Scores {
		scaled := s.Score * 1e6
		if scaled != float64(int64(scaled)) {
			t.Errorf("score(%s) = %v is not rounded to 6 decimal places", s.Node, s.Score)
		}
	}
}

func TestPageRankDeterminism(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "C", "D", "E"}, [][3]any{
		{"A", "B", 1}, {"B", "C", 1}, {"C", "A", 1}, {"D", "C", 1}, {"E", "D", 1}, {"A", "E", 1},
	})
	var first []float64
	for i := 0; i < 200; i++ {
		got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
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
				t.Fatalf("nondeterministic PageRank: run %d gave %v, first gave %v", i, cur, first)
			}
		}
	}
}
