package nodes_test

import (
	"math"
	"sort"

	gen "christiangeorgelucas/graph-tools/gen"
)

// This file holds INDEPENDENT ORACLES: brute-force reference implementations
// written from scratch against the raw gen.Graph input. They share no code with
// the nodes under test and do not use gonum, so agreement between an oracle and
// a node is evidence of correctness, not of self-consistency.

// adjacency builds a plain adjacency list straight off the input message.
func adjacency(g *gen.Graph) map[string]map[string]float64 {
	adj := map[string]map[string]float64{}
	for _, n := range g.Nodes {
		adj[n.Id] = map[string]float64{}
	}
	for _, e := range g.Edges {
		w := e.Weight
		if w == 0 && !e.ExplicitZeroWeight {
			w = 1
		}
		adj[e.From][e.To] = w
		if !g.Directed {
			adj[e.To][e.From] = w
		}
	}
	return adj
}

// bruteForceShortestPath enumerates EVERY simple path from src to dst by
// exhaustive backtracking and returns the minimum total weight, or +Inf.
// Exponential, so only use it on the small fixtures in this suite.
func bruteForceShortestPath(g *gen.Graph, src, dst string) (float64, []string) {
	adj := adjacency(g)
	best := math.Inf(1)
	var bestPath []string
	visited := map[string]bool{}

	var walk func(cur string, acc float64, path []string)
	walk = func(cur string, acc float64, path []string) {
		if cur == dst {
			if acc < best {
				best = acc
				bestPath = append([]string(nil), path...)
			}
			return
		}
		neigh := make([]string, 0, len(adj[cur]))
		for n := range adj[cur] {
			neigh = append(neigh, n)
		}
		sort.Strings(neigh)
		for _, n := range neigh {
			if visited[n] {
				continue
			}
			visited[n] = true
			walk(n, acc+adj[cur][n], append(path, n))
			visited[n] = false
		}
	}
	visited[src] = true
	walk(src, 0, []string{src})
	return best, bestPath
}

// bruteForceMSTWeight enumerates every subset of edges of size |V|-1 and returns
// the minimum weight among those that form a spanning tree. Independent of
// Kruskal, Prim and gonum entirely.
func bruteForceMSTWeight(g *gen.Graph) float64 {
	type edge struct {
		a, b string
		w    float64
	}
	var edges []edge
	for _, e := range g.Edges {
		w := e.Weight
		if w == 0 && !e.ExplicitZeroWeight {
			w = 1
		}
		edges = append(edges, edge{e.From, e.To, w})
	}
	n := len(g.Nodes)
	want := n - 1
	best := math.Inf(1)

	// Iterate every subset via a bitmask.
	for mask := 0; mask < (1 << len(edges)); mask++ {
		if popcount(mask) != want {
			continue
		}
		// Union-find over the chosen subset, written independently here.
		parent := map[string]string{}
		for _, nd := range g.Nodes {
			parent[nd.Id] = nd.Id
		}
		var find func(string) string
		find = func(x string) string {
			for parent[x] != x {
				x = parent[x]
			}
			return x
		}
		total := 0.0
		acyclic := true
		for i, e := range edges {
			if mask&(1<<i) == 0 {
				continue
			}
			ra, rb := find(e.a), find(e.b)
			if ra == rb {
				acyclic = false
				break
			}
			parent[ra] = rb
			total += e.w
		}
		if !acyclic {
			continue
		}
		// Spanning: exactly one root among all vertices.
		roots := map[string]bool{}
		for _, nd := range g.Nodes {
			roots[find(nd.Id)] = true
		}
		if len(roots) != 1 {
			continue
		}
		if total < best {
			best = total
		}
	}
	return best
}

func popcount(x int) int {
	c := 0
	for x != 0 {
		c += x & 1
		x >>= 1
	}
	return c
}

// isWalk verifies, against the raw input edges, that every consecutive pair in
// `seq` is joined by a real edge — used to validate a returned cycle witness.
func isWalk(g *gen.Graph, seq []string) bool {
	if len(seq) < 2 {
		return false
	}
	adj := adjacency(g)
	for i := 0; i+1 < len(seq); i++ {
		if _, ok := adj[seq[i]][seq[i+1]]; !ok {
			return false
		}
	}
	return true
}

// circuitRank computes |E| - |V| + components independently (Euler's formula).
func circuitRank(g *gen.Graph) int {
	parent := map[string]string{}
	for _, n := range g.Nodes {
		parent[n.Id] = n.Id
	}
	var find func(string) string
	find = func(x string) string {
		for parent[x] != x {
			x = parent[x]
		}
		return x
	}
	loops := 0
	for _, e := range g.Edges {
		if e.From == e.To {
			loops++
			continue
		}
		ra, rb := find(e.From), find(e.To)
		if ra != rb {
			parent[ra] = rb
		}
	}
	roots := map[string]bool{}
	for _, n := range g.Nodes {
		roots[find(n.Id)] = true
	}
	return (len(g.Edges) - loops) - len(g.Nodes) + len(roots)
}
