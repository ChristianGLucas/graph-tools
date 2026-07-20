# graph-tools

Composable graph-algorithm nodes for the [Axiom](https://axiom.dev) marketplace,
published as `christiangeorgelucas/graph-tools`.

Ten stateless nodes answer the questions agents actually ask of a graph — *what
is the cheapest route?*, *what order can I build this in?*, *what is reachable
from here?*, *which node matters most?*, *is there a dependency cycle?* — over a
single shared `Graph` envelope.

The algorithms come from [gonum/graph](https://pkg.go.dev/gonum.org/v1/gonum/graph)
(BSD-3-Clause), which owns every algorithmically hard part: Dijkstra,
Bellman-Ford, Tarjan's SCC, Kruskal, Brandes' betweenness and the PageRank power
iteration.

## The `Graph` envelope

Every node consumes `Graph`, and the graph-producing nodes emit it, so they
chain without adapters:

```json
{
  "directed": true,
  "nodes": [{ "id": "build" }, { "id": "test" }, { "id": "deploy" }],
  "edges": [
    { "from": "build", "to": "test", "weight": 1 },
    { "from": "test",  "to": "deploy", "weight": 1 }
  ]
}
```

Vertex ids are your own strings. An edge weight of `0` means 1.0 unless you set
`explicit_zero_weight`, which lets you express genuine zero-cost edges.

## Nodes

| Node | Input → Output | What it does |
|---|---|---|
| `ShortestPath` | `ShortestPathRequest` → `ShortestPathResult` | Cheapest route between two vertices. Dijkstra, switching to Bellman-Ford when weights are negative. |
| `Distances` | `DistancesRequest` → `DistancesResult` | Cost and hop count from one source to every reachable vertex, plus the unreachable set. |
| `TopologicalSort` | `Graph` → `TopoSortResult` | Dependency order for a DAG; reports `is_dag=false` on a cycle. Directed only. |
| `ConnectedComponents` | `Graph` → `ComponentsResult` | Connected components (undirected) or strongly connected components (directed, Tarjan). |
| `MinimumSpanningTree` | `Graph` → `SpanningTreeResult` | Cheapest spanning tree/forest via Kruskal. Emits a `Graph`. Undirected only. |
| `Centrality` | `CentralityRequest` → `CentralityResult` | Per-vertex `degree`, `betweenness`, `closeness`, `harmonic` or `eccentricity`. |
| `PageRank` | `PageRankRequest` → `PageRankResult` | Link-importance ranking; scores sum to 1. |
| `DetectCycle` | `Graph` → `CycleResult` | Whether a cycle exists, how many, and one concrete example cycle. |
| `Describe` | `Graph` → `GraphStats` | Counts, density, mean degree, self-loops, connectivity, components, is-DAG. |
| `Subgraph` | `SubgraphRequest` → `SubgraphResult` | The subgraph induced by a set of vertices. Emits a `Graph`. |

## Behaviour worth knowing

**Deterministic output.** gonum's simple graphs iterate Go maps, so several of
its algorithms are order-sensitive: Dijkstra breaks equal-cost ties by heap
insertion order, Kruskal sorts edges with a non-stable sort, and PageRank seeds
its power iteration from a random vector. Every node here drives gonum through
an ordering wrapper that imposes a total order on each iterator, and PageRank
additionally converges to a pinned `1e-14` tolerance and rounds to 6 decimal
places. The result is that the same input always produces the same output —
which the test suite asserts by re-invoking each node up to 200 times.

**Malformed input is rejected, never guessed.** Empty or duplicate vertex ids,
edges pointing at vertices that do not exist, duplicate edges, non-finite
weights, and edge weights whose magnitudes sum past float64 range all return a
structured `error` string rather than a crash or a silently wrong answer.

**Bounded work.** Limits are checked against the raw input before anything is
allocated: 20 000 vertices and 200 000 edges generally, and 600 vertices for the
all-pairs centrality measures, whose memory cost is quadratic.

**Self-loops** are counted and reported (they make a graph cyclic and non-DAG)
but are excluded from path, spanning-tree and centrality computations, where
they cannot affect the answer.

## Correctness

Every node has golden tests with hand-computed expected values, plus
**independent oracles** that share no code with the implementation:

- exhaustive enumeration of all simple paths, cross-checked against `ShortestPath` and `Distances` for every vertex pair;
- exhaustive search over all `|V|-1` edge subsets, cross-checked against `MinimumSpanningTree`;
- the closed forms for harmonic, closeness and eccentricity centrality on a path graph;
- PageRank's exact `1/n` stationary distribution on a directed cycle, for several cycle lengths;
- Euler's circuit-rank formula `|E| - |V| + components`, cross-checked against `DetectCycle`;
- validation that each returned cycle is a genuine closed walk, and that each topological order really does place every edge's tail before its head.

Run them with `axiom test`.

## Licence

MIT — see [LICENSE](LICENSE). Built for the Axiom marketplace.

The sole runtime dependency is `gonum.org/v1/gonum` (BSD-3-Clause), which itself
pulls in no non-standard-library packages in this build closure.
