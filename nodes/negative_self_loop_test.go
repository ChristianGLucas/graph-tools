package nodes_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	gen "christiangeorgelucas/graph-tools/gen"
	"christiangeorgelucas/graph-tools/nodes"
)

// A negative-weight SELF-LOOP on a vertex the source can reach IS a reachable
// negative cycle: traverse it repeatedly and the cost falls without bound, so
// shortest paths are undefined. Self-loops are held outside the gonum
// structures, so gonum's own Bellman-Ford cannot see one — this package must
// detect it explicitly. Before this was fixed, ShortestPath and Distances
// returned confidently wrong FINITE numbers for these graphs.
//
// The package's own semantics already agree: DetectCycle reports a self-loop as
// a cycle. This is the shortest-path side of that same fact.

// ── golden, hand-checkable ───────────────────────────────────────────────────

func TestNegativeSelfLoopIsAReachableNegativeCycle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// a -> b (cost 1), and b carries a self-loop of -1. b is reachable from a,
	// so the a->b distance is -Inf, not 1.
	g := &gen.Graph{
		Directed: true,
		Nodes:    []*gen.GraphNode{{Id: "a"}, {Id: "b"}},
		Edges: []*gen.GraphEdge{
			{From: "a", To: "b", Weight: 1},
			{From: "b", To: "b", Weight: -1},
		},
	}

	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "a", To: "b"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !strings.Contains(sp.Error, "negative-weight cycle") {
		t.Fatalf("ShortestPath must report the negative cycle; got error=%q found=%v weight=%v",
			sp.Error, sp.Found, sp.TotalWeight)
	}
	if sp.Found {
		t.Errorf("a rejected request must not report found=true")
	}

	d, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: "a"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !strings.Contains(d.Error, "negative-weight cycle") {
		t.Fatalf("Distances must report the negative cycle; got error=%q distances=%+v", d.Error, d.Distances)
	}
	if len(d.Distances) != 0 {
		t.Errorf("a rejected request must not also return distances: %+v", d.Distances)
	}
}

// A negative self-loop on a vertex the source CANNOT reach leaves shortest
// paths well-defined, and must NOT be rejected. This is the test that keeps the
// fix from degenerating into "ban negative self-loops".
func TestUnreachableNegativeSelfLoopIsNotACycleError(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	// a -> b costs 2. c is isolated and carries the negative self-loop.
	g := &gen.Graph{
		Directed: true,
		Nodes:    []*gen.GraphNode{{Id: "a"}, {Id: "b"}, {Id: "c"}},
		Edges: []*gen.GraphEdge{
			{From: "a", To: "b", Weight: 2},
			{From: "c", To: "c", Weight: -1},
		},
	}
	sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "a", To: "b"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if sp.Error != "" {
		t.Fatalf("an UNREACHABLE negative self-loop must not be reported: %s", sp.Error)
	}
	if !sp.Found || sp.TotalWeight != 2 {
		t.Errorf("want found a->b at weight 2, got found=%v weight=%v", sp.Found, sp.TotalWeight)
	}
}

// A NON-negative self-loop is never a negative cycle and must not be rejected.
func TestNonNegativeSelfLoopIsNotACycleError(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	for _, w := range []float64{1, 5} {
		g := &gen.Graph{
			Directed: true,
			Nodes:    []*gen.GraphNode{{Id: "a"}, {Id: "b"}},
			Edges: []*gen.GraphEdge{
				{From: "a", To: "b", Weight: -1}, // forces the Bellman-Ford path
				{From: "b", To: "b", Weight: w},
			},
		}
		sp, err := nodes.ShortestPath(ctx, ax, &gen.ShortestPathRequest{Graph: g, From: "a", To: "b"})
		if err != nil {
			t.Fatalf("unexpected transport error: %v", err)
		}
		if sp.Error != "" {
			t.Fatalf("self-loop of weight %v is not a negative cycle: %s", w, sp.Error)
		}
		if !sp.Found || sp.TotalWeight != -1 {
			t.Errorf("w=%v: want found a->b at weight -1, got found=%v weight=%v", w, sp.Found, sp.TotalWeight)
		}
	}
}

// ── independent oracle over a randomized sweep ───────────────────────────────

// oracleReachableNegativeCycle is a from-scratch Bellman-Ford written against
// the RAW edge list — self-loops included — with no reference to the package's
// internals. It answers: from `src`, is there a reachable negative cycle?
//
// Standard formulation: relax every edge |V| times, then check whether any edge
// still relaxes. Edges leaving an unreached (+Inf) vertex never relax, so a
// cycle in an unreachable region is correctly ignored.
func oracleReachableNegativeCycle(g *gen.Graph, src string) bool {
	arcs := oracleArcs(g)
	dist := map[string]float64{}
	for _, n := range g.Nodes {
		dist[n.Id] = math.Inf(1)
	}
	dist[src] = 0

	relaxAll := func() bool {
		changed := false
		for _, a := range arcs {
			if math.IsInf(dist[a.from], 1) {
				continue // edges leaving an unreached vertex never relax
			}
			if c := dist[a.from] + a.w; c < dist[a.to] {
				dist[a.to] = c
				changed = true
			}
		}
		return changed
	}
	for i := 0; i < len(g.Nodes); i++ {
		if !relaxAll() {
			return false // settled early: no negative cycle
		}
	}
	// Still relaxing after |V| full passes: a reachable negative cycle.
	return relaxAll()
}

// oracleArc is one directed relaxation step.
type oracleArc struct {
	from, to string
	w        float64
}

// oracleArcs flattens the caller's edge list into directed arcs, applying the
// package's documented weight default. An undirected edge becomes an arc in
// each direction — which is also why any negative undirected edge is itself a
// negative cycle: traverse it and come straight back.
func oracleArcs(g *gen.Graph) []oracleArc {
	var arcs []oracleArc
	for _, e := range g.Edges {
		w := e.Weight
		if w == 0 && !e.ExplicitZeroWeight {
			w = 1
		}
		arcs = append(arcs, oracleArc{e.From, e.To, w})
		if !g.Directed && e.From != e.To {
			arcs = append(arcs, oracleArc{e.To, e.From, w})
		}
	}
	return arcs
}

// TestNegativeCycleDetectionAgainstOracle is the regression sweep for the
// round-6 finding: 933 of 4000 random 3-vertex digraphs had a reachable
// negative cycle the package failed to report, 100% of them via a negative
// self-loop. Every disagreement with the oracle is a wrong answer on realistic
// input, in both directions — a missed cycle AND a false alarm.
func TestNegativeCycleDetectionAgainstOracle(t *testing.T) {
	ctx, ax := context.Background(), newTestContext(t)
	rng := rand.New(rand.NewSource(20260720)) // fixed seed: reproducible

	ids := []string{"a", "b", "c"}
	weights := []float64{-3, -1, 1, 2}
	sweeps, withCycle, selfLoopCycles := 0, 0, 0

	for iter := 0; iter < 4000; iter++ {
		g := &gen.Graph{Directed: rng.Intn(2) == 0}
		for _, id := range ids {
			g.Nodes = append(g.Nodes, &gen.GraphNode{Id: id})
		}
		seen := map[string]bool{}
		hasNegSelfLoop := false
		for e := 0; e < 4; e++ {
			from, to := ids[rng.Intn(3)], ids[rng.Intn(3)]
			k := from + "|" + to
			if !g.Directed && to < from {
				k = to + "|" + from
			}
			if seen[k] {
				continue // the package rejects duplicate edges
			}
			seen[k] = true
			w := weights[rng.Intn(len(weights))]
			if from == to && w < 0 {
				hasNegSelfLoop = true
			}
			g.Edges = append(g.Edges, &gen.GraphEdge{From: from, To: to, Weight: w})
		}
		if len(g.Edges) == 0 {
			continue
		}

		src := ids[rng.Intn(3)]
		want := oracleReachableNegativeCycle(g, src)
		sweeps++
		if want {
			withCycle++
			if hasNegSelfLoop {
				selfLoopCycles++
			}
		}

		got, err := nodes.Distances(ctx, ax, &gen.DistancesRequest{Graph: g, From: src})
		if err != nil {
			t.Fatalf("iter %d: unexpected transport error: %v\n%s", iter, err, dumpGraph(g, src))
		}
		reported := strings.Contains(got.Error, "negative-weight cycle")
		if reported != want {
			t.Fatalf("iter %d: reachable-negative-cycle mismatch: node reported %v, oracle says %v (error=%q)\n%s",
				iter, reported, want, got.Error, dumpGraph(g, src))
		}
		// When there is no cycle, every reported distance must match the oracle's
		// — a cycle check that fires correctly but corrupts the numbers is no fix.
		if !want && got.Error == "" {
			for _, d := range got.Distances {
				if w := oracleDistance(g, src, d.Node); w != d.Weight {
					t.Fatalf("iter %d: distance to %s = %v, oracle says %v\n%s",
						iter, d.Node, d.Weight, w, dumpGraph(g, src))
				}
			}
		}
	}

	t.Logf("swept %d graphs; %d had a reachable negative cycle, %d of those via a negative self-loop",
		sweeps, withCycle, selfLoopCycles)
	// Guard against the sweep silently going vacuous (e.g. a generator change
	// that stops producing the very case this test exists for).
	if selfLoopCycles < 100 {
		t.Fatalf("sweep is not exercising the self-loop case: only %d of %d cycles came via a negative self-loop",
			selfLoopCycles, withCycle)
	}
}

// oracleDistance is the same from-scratch relaxation, returning the settled
// distance to one vertex. Only called on graphs the oracle deems cycle-free.
func oracleDistance(g *gen.Graph, src, to string) float64 {
	dist := map[string]float64{}
	for _, n := range g.Nodes {
		dist[n.Id] = math.Inf(1)
	}
	dist[src] = 0
	arcs := oracleArcs(g)
	for i := 0; i < len(g.Nodes)+1; i++ {
		for _, a := range arcs {
			if math.IsInf(dist[a.from], 1) {
				continue
			}
			if c := dist[a.from] + a.w; c < dist[a.to] {
				dist[a.to] = c
			}
		}
	}
	return dist[to]
}

func dumpGraph(g *gen.Graph, src string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  directed=%v source=%s\n", g.Directed, src)
	for _, e := range g.Edges {
		fmt.Fprintf(&b, "  %s -> %s  w=%v\n", e.From, e.To, e.Weight)
	}
	return b.String()
}
