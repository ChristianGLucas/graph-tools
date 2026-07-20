package nodes_test

import (
	"context"
	"math"
	"testing"
	"time"

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
		// Compare against an independent re-rounding rather than asserting an
		// exact-equality property of a floating-point product.
		if want := math.Round(s.Score*1e6) / 1e6; s.Score != want {
			t.Errorf("score(%s) = %v is not rounded to 6 decimal places (re-rounds to %v)", s.Node, s.Score, want)
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

// TestPageRankHonoursSelfLoops is the regression guard for a real defect: the
// shared builder strips self-loops before handing the graph to gonum (gonum's
// simple graphs panic on a self-edge), which silently produced materially wrong
// PageRank scores. A self-loop is a rank sink and must be part of the topology.
//
// Oracle: on a -> a, b -> a, c -> a with damping 0.85, b and c are pure sources
// with no in-edges, so each holds exactly the teleport share (1-d)/n = 0.15/3 =
// 0.05, and a holds the remaining 0.9. That is a closed form, derived from the
// PageRank definition rather than from this implementation.
func TestPageRankHonoursSelfLoops(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{
		{"a", "a", 1}, {"b", "a", 1}, {"c", "a", 1},
	})
	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	m := map[string]float64{}
	for _, s := range got.Scores {
		m[s.Node] = s.Score
	}
	for k, w := range map[string]float64{"a": 0.9, "b": 0.05, "c": 0.05} {
		if !nearly(m[k], w, 1e-5) {
			t.Errorf("score(%s) = %v, closed form = %v", k, m[k], w)
		}
	}
}

// A self-loop must actually CHANGE the scores relative to the same graph
// without it — otherwise the loop is still being discarded.
func TestPageRankSelfLoopChangesScores(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	base := mkGraph(true, []string{"a", "b", "c"}, [][3]any{{"b", "a", 1}, {"c", "a", 1}})
	looped := mkGraph(true, []string{"a", "b", "c"}, [][3]any{
		{"a", "a", 1}, {"b", "a", 1}, {"c", "a", 1},
	})
	r1, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: base})
	if err != nil || r1.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, r1.Error)
	}
	r2, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: looped})
	if err != nil || r2.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, r2.Error)
	}
	same := true
	for i := range r1.Scores {
		if r1.Scores[i].Score != r2.Scores[i].Score {
			same = false
		}
	}
	if same {
		t.Errorf("adding a self-loop on `a` did not change any score — the loop is being discarded: %v", r2.Scores)
	}
}

// TestPageRankRejectsNonFiniteDamping is the regression guard for a remote
// denial of service: NaN fails every comparison, so a `damping <= 0 ||
// damping >= 1` range check lets it through, and the power iteration's
// convergence test then never becomes true — an infinite loop, reachable from
// the wire, that permanently burns a core. The test asserts both that the value
// is rejected AND that the call returns promptly.
func TestPageRankRejectsNonFiniteDamping(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b"}, [][3]any{{"a", "b", 1}})

	for _, bad := range []float64{math_NaN(), math_Inf(1), math_Inf(-1)} {
		done := make(chan *gen.PageRankResult, 1)
		go func(d float64) {
			r, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g, Damping: d})
			if err != nil {
				t.Errorf("unexpected Go error: %v", err)
			}
			done <- r
		}(bad)

		select {
		case r := <-done:
			if r.Error == "" {
				t.Errorf("damping %v must be rejected, got %+v", bad, r)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("damping %v did not return within 15s — the power iteration is not terminating", bad)
		}
	}
}

// TestPageRankUndirectedIsBidirectional pins the documented undirected handling.
func TestPageRankUndirectedIsBidirectional(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// An undirected edge a-b makes a and b structurally identical, so a
	// symmetric graph must give every vertex the same score.
	g := mkGraph(false, []string{"a", "b"}, [][3]any{{"a", "b", 1}})
	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if !nearly(got.Scores[0].Score, 0.5, 1e-5) || !nearly(got.Scores[1].Score, 0.5, 1e-5) {
		t.Errorf("an undirected edge must be symmetric, got %v", got.Scores)
	}
}

// TestPageRankRejectsSlowConvergingDamping is the regression guard for an
// unbounded CPU burn. The iteration count is roughly log(tolerance)/log(damping),
// which diverges as damping approaches 1: at 0.99 a full-size graph converges in
// ~0.3s, but at 0.9999999999 a TEN-vertex ring does not return at all. A range
// check of (0,1) is therefore not a cost bound.
func TestPageRankRejectsSlowConvergingDamping(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"a", "b", "c"}, [][3]any{
		{"a", "b", 1}, {"b", "c", 1}, {"c", "a", 1},
	})
	for _, bad := range []float64{0.991, 0.99999, 0.9999999999, 1} {
		done := make(chan *gen.PageRankResult, 1)
		go func(d float64) {
			r, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g, Damping: d})
			if err != nil {
				t.Errorf("unexpected Go error: %v", err)
			}
			done <- r
		}(bad)
		select {
		case r := <-done:
			if r.Error == "" {
				t.Errorf("damping %v must be rejected, got %+v", bad, r)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("damping %v did not return within 15s — the iteration is unbounded", bad)
		}
	}
}

// The largest ACCEPTED damping must still complete quickly at full scale —
// otherwise the bound does not bound the cost.
func TestPageRankMaxDampingStaysFast(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	const n = 20000
	g := &gen.Graph{Directed: true}
	for i := 0; i < n; i++ {
		g.Nodes = append(g.Nodes, &gen.GraphNode{Id: "n" + itoa(i)})
	}
	for i := 0; i < n; i++ {
		g.Edges = append(g.Edges, &gen.GraphEdge{From: "n" + itoa(i), To: "n" + itoa((i+1)%n)})
	}
	start := time.Now()
	got, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g, Damping: 0.99})
	if err != nil || got.Error != "" {
		t.Fatalf("damping 0.99 at the vertex limit must be accepted: err=%v nodeErr=%s", err, got.Error)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("PageRank at max damping and max size took %v — the damping bound does not bound the cost", elapsed)
	}
	// A ring is symmetric, so every score must be 1/n.
	for _, s := range got.Scores {
		if !nearly(s.Score, 1/float64(n), 1e-5) {
			t.Fatalf("score(%s) = %v, closed form = %v", s.Node, s.Score, 1/float64(n))
		}
	}
}

// TestPageRankPeriodicGraphConverges is the regression guard for a hang that
// the damping cap alone did not cover. gonum's power iteration has NO iteration
// cap, and on a PERIODIC graph — a directed ring — the residual oscillates and
// plateaus above a 1e-14 tolerance, so the loop never exits. A ten-vertex ring
// at damping 0.999 spun forever. The fix is the 1e-12 tolerance; this test
// pins it by asserting both promptness and the exact closed-form answer.
func TestPageRankPeriodicGraphConverges(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, n := range []int{2, 3, 10, 50} {
		ids := make([]string, n)
		var edges [][3]any
		for i := 0; i < n; i++ {
			ids[i] = "n" + itoa(i)
		}
		for i := 0; i < n; i++ {
			edges = append(edges, [3]any{ids[i], ids[(i+1)%n], 1})
		}
		g := mkGraph(true, ids, edges)

		// The largest damping the node accepts is the worst case.
		for _, d := range []float64{0.85, 0.99} {
			done := make(chan *gen.PageRankResult, 1)
			go func(damp float64) {
				r, err := nodes.PageRank(ctx, ax, &gen.PageRankRequest{Graph: g, Damping: damp})
				if err != nil {
					t.Errorf("unexpected Go error: %v", err)
				}
				done <- r
			}(d)

			select {
			case r := <-done:
				if r.Error != "" {
					t.Fatalf("ring n=%d damping=%v: %s", n, d, r.Error)
				}
				// A ring is vertex-transitive: every score is exactly 1/n.
				for _, s := range r.Scores {
					if !nearly(s.Score, 1/float64(n), 1e-5) {
						t.Errorf("ring n=%d damping=%v: score(%s) = %v, closed form = %v",
							n, d, s.Node, s.Score, 1/float64(n))
					}
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("ring n=%d damping=%v did not converge within 20s", n, d)
			}
		}
	}
}
