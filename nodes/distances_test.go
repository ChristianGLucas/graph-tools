package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

func TestDistances(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: dagFixture(), From: "A"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	// Golden: hand-computed from dagFixture.
	want := map[string]struct {
		w float64
		h int32
	}{
		"A": {0, 0}, "B": {1, 1}, "C": {3, 2}, "D": {4, 3}, "E": {7, 4},
	}
	if len(got.Distances) != len(want) {
		t.Fatalf("got %d distances, want %d: %+v", len(got.Distances), len(want), got.Distances)
	}
	for _, d := range got.Distances {
		w, ok := want[d.Node]
		if !ok {
			t.Errorf("unexpected node %s", d.Node)
			continue
		}
		if d.Weight != w.w {
			t.Errorf("%s: weight = %v, want %v", d.Node, d.Weight, w.w)
		}
		if d.HopCount != w.h {
			t.Errorf("%s: hop_count = %d, want %d", d.Node, d.HopCount, w.h)
		}
	}
	if len(got.Unreachable) != 0 {
		t.Errorf("unreachable = %v, want none", got.Unreachable)
	}
}

// TestDistancesAgainstBruteForceOracle cross-checks every distance against the
// exhaustive simple-path enumeration.
func TestDistancesAgainstBruteForceOracle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := dagFixture()
	for _, src := range []string{"A", "B", "C", "D", "E"} {
		got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: src})
		if err != nil || got.Error != "" {
			t.Fatalf("err=%v nodeErr=%s", err, got.Error)
		}
		seen := map[string]bool{}
		for _, d := range got.Distances {
			seen[d.Node] = true
			if d.Node == src {
				if d.Weight != 0 {
					t.Errorf("%s: self distance = %v, want 0", src, d.Weight)
				}
				continue
			}
			wantW, _ := bruteForceShortestPath(g, src, d.Node)
			if d.Weight != wantW {
				t.Errorf("%s->%s: weight = %v, oracle = %v", src, d.Node, d.Weight, wantW)
			}
		}
		for _, u := range got.Unreachable {
			if w, _ := bruteForceShortestPath(g, src, u); !isInf(w) {
				t.Errorf("%s->%s reported unreachable but oracle found weight %v", src, u, w)
			}
		}
		// Every vertex must be classified exactly once.
		if len(got.Distances)+len(got.Unreachable) != 5 {
			t.Errorf("%s: %d reachable + %d unreachable != 5", src, len(got.Distances), len(got.Unreachable))
		}
	}
}

func TestDistancesUnreachable(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	g := mkGraph(true, []string{"A", "B", "X", "Y"}, [][3]any{{"A", "B", 2}, {"X", "Y", 1}})
	got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: "A"})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v nodeErr=%s", err, got.Error)
	}
	if want := []string{"X", "Y"}; !eqStrings(got.Unreachable, want) {
		t.Errorf("unreachable = %v, want %v", got.Unreachable, want)
	}
	if len(got.Distances) != 2 {
		t.Errorf("distances = %+v, want A and B only", got.Distances)
	}
}

func TestDistancesErrors(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for name, req := range map[string]*gen.DistancesRequest{
		"nil request":    nil,
		"nil graph":      {From: "A"},
		"unknown source": {Graph: dagFixture(), From: "nope"},
	} {
		got, err := nodes.Distances(ctx, ax, req)
		if err != nil {
			t.Fatalf("%s: unexpected Go error %v", name, err)
		}
		if got.Error == "" {
			t.Errorf("%s: expected a structured error, got %+v", name, got)
		}
	}
}
