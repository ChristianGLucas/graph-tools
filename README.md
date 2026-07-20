# graph-tools

Composable graph-algorithm nodes for the Axiom marketplace, published as
`christiangeorgelucas/graph-tools`.

Eleven stateless nodes answer the questions agents actually ask of a graph — *what
is the cheapest route?*, *what order can I build this in?*, *what is reachable
from here?*, *which node matters most?*, *is there a dependency cycle?* — over a
single shared `Graph` envelope.

The algorithms come from [gonum/graph](https://pkg.go.dev/gonum.org/v1/gonum/graph)
(BSD-3-Clause), which owns the algorithmically hard parts: Dijkstra,
Bellman-Ford, Tarjan's SCC, Kruskal and Brandes' betweenness. The one exception
is the PageRank recurrence, which this package runs itself so that it can fix
the iteration's starting vector — see *Deterministic output* below.

## The `Graph` envelope

Every node consumes `Graph`:

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

Vertex ids are your own strings. **An edge weight of `0` means 1.0** unless you
set `explicit_zero_weight`, which is how you express a genuine zero-cost edge.

## Nodes

| Node | Input → Output | What it does |
|---|---|---|
| `ShortestPath` | `ShortestPathRequest` → `ShortestPathResult` | Cheapest route between two vertices. Dijkstra, switching to Bellman-Ford when weights are negative. |
| `Distances` | `DistancesRequest` → `DistancesResult` | Cost and hop count from one source to every reachable vertex, plus the unreachable set. |
| `TopologicalSort` | `Graph` → `TopoSortResult` | Dependency order for a DAG; reports `is_dag=false` on a cycle. Directed only. |
| `ConnectedComponents` | `Graph` → `ComponentsResult` | Connected components (undirected) or strongly connected components (directed, Tarjan). |
| `MinimumSpanningTree` | `Graph` → **`Graph`** | Cheapest spanning tree/forest via Kruskal. Undirected only. |
| `Centrality` | `CentralityRequest` → `CentralityResult` | Per-vertex `degree`, `betweenness`, `closeness`, `harmonic` or `eccentricity`. |
| `PageRank` | `PageRankRequest` → `PageRankResult` | Link-importance ranking; scores sum to 1 up to 6-decimal rounding. Damping defaults to 0.85, max 0.99. |
| `DetectCycle` | `Graph` → `CycleResult` | Whether a cycle exists, how many are independent, and one concrete example. |
| `Describe` | `Graph` → `GraphStats` | Counts, density, mean degree, self-loops, total weight, connectivity, components, is-DAG. |
| `Subgraph` | `SubgraphRequest` → **`Graph`** | The subgraph induced by a set of vertices. |
| `Orient` | `OrientRequest` → **`Graph`** | Reinterprets edge direction — the bridge across the directed/undirected split. |

## Composing these nodes in a flow

`MinimumSpanningTree` and `Subgraph` return a **top-level `Graph`**, and
`TopologicalSort`, `ConnectedComponents`, `DetectCycle`, `Describe` and
`MinimumSpanningTree` itself take a top-level `Graph`. Those pairings connect
with an **identity edge and no adapter** — for example
`Subgraph → MinimumSpanningTree → Describe`.

This shape is deliberate. A nested protobuf message field cannot currently be
mapped across a flow edge, so a result type that merely *contained* a `Graph`
would not compose — which is why `MinimumSpanningTree` returns the tree itself
rather than a wrapper carrying its weight. **Pipe it into `Describe` to get the
tree's `total_weight`.**

The nodes that take a request wrapper (`ShortestPath`, `Distances`,
`Centrality`, `PageRank` and `Subgraph`) need a per-call parameter alongside the
graph — a source vertex, a measure, a vertex set — so in a flow their graph comes
from flow input or config rather than from an upstream edge. Concretely, the
pairs that compose with no adapter are
`{MinimumSpanningTree, Subgraph} → {Describe, DetectCycle, ConnectedComponents, TopologicalSort, MinimumSpanningTree}`,
minus `MinimumSpanningTree → TopologicalSort`, since a spanning tree is always
undirected and `TopologicalSort` requires a directed graph.

**Crossing the directed/undirected split.** `MinimumSpanningTree` requires an
undirected graph and `TopologicalSort` requires a directed one, so `Orient`
exists to convert between them mid-flow: `Orient → MinimumSpanningTree` lets a
directed dependency graph reach a spanning tree without leaving the flow.
Converting to undirected collapses opposing edge pairs, keeping the smaller
weight; converting to directed replaces each undirected edge with an opposing
pair, which preserves reachability but makes the result cyclic by construction.

**Two different failure modes.** The eight nodes with a result message report a
rejected request *in band*, as a structured `error` string with an HTTP 200 — so
in a flow the step reports success and you must check `error` yourself. The two
graph-producing nodes return a bare `Graph`, which has no `error` field, so they
fail *out of band* and abort the flow instead. Check `error` first on the former;
expect an aborted run from the latter.

## Behaviour worth knowing

**Deterministic output.** gonum's simple graphs iterate Go maps, so several of
its algorithms are order-sensitive: Dijkstra breaks equal-cost ties by heap
insertion order, Kruskal sorts edges with a non-stable sort, and PageRank seeds
its power iteration from a random vector. Every node here drives gonum through
an ordering wrapper that imposes a total order on each iterator.

PageRank needed more than that. gonum seeds its power iteration from a *random*
vector, so raw scores differ in their last digits between runs — and rounding
the output is not a sufficient fix, because a score whose exact value sits on a
rounding boundary still flips. (A four-vertex graph scoring exactly `0.0534375`
returned both `0.053437` and `0.053438`.) This package therefore runs the
PageRank recurrence itself, from a fixed uniform start vector accumulating in a
fixed order, so the result is bit-for-bit repeatable before rounding as well as
after; it is cross-checked against gonum's own implementation in the tests.

The same input always produces the same output — asserted by re-invoking each
node many times, including across randomly generated graphs and on a known
rounding-boundary graph. Because scores are rounded to 6 decimals, PageRank
scores sum to 1 only up to that rounding — at most `5e-7` of drift per vertex,
so about `1e-6` on a small graph and up to ~`0.01` on a 20 000-vertex one.

**Malformed input is rejected, never guessed.** Empty, over-long, duplicate or
control-character-bearing vertex ids; edges pointing at vertices that do not
exist; duplicate edges; non-finite weights; a weight of negative zero (use
`explicit_zero_weight` if you mean it); a non-finite or out-of-range PageRank
damping factor; and edge weights whose magnitudes sum past float64 range all
return a structured error rather than a crash or a silently wrong answer. Caller
strings echoed back in an error are truncated, so an error response can never
amplify the request.

**Bounded work.** Limits are checked against the raw input before anything is
allocated: 20 000 vertices, 200 000 edges, 3 MiB encoded, 256-byte ids and
1024-byte labels. Two paths get their own tighter bounds, because a caller's
input silently selects a far more expensive algorithm:

- The all-pairs centrality measures are capped at 600 vertices **and** a
  1 200 000 vertex×edge product. Bounding vertices alone is not a cost bound: a
  dense 600-vertex graph passes a vertex cap while costing over a minute.
- A graph carrying any negative weight switches from Dijkstra to Bellman-Ford
  and is capped at 2000 vertices **and** the same 1 200 000 product. gonum's
  Bellman-Ford-Moore costs O(V·E), so the edge count is the dominant term, not
  an irrelevant one.

PageRank's damping factor is capped at 0.99 for the same family of reason: the
iteration count grows without limit as damping approaches 1.

The paths whose cost is not near-linear in the input run under a 20-second
wall-clock budget and return a structured error if it is exceeded, so a bound
that turns out to be mis-calibrated for some input shape degrades into an error
rather than a hang. Those calls also return promptly when the request is
cancelled, rather than waiting for a library call that takes no context and
cannot be interrupted. The budgeted paths are PageRank, the all-pairs centrality
measures, the negative-weight (Bellman-Ford) shortest-path search, and
`Distances`. The remaining nodes are near-linear and bounded by the input caps
alone; every node measures well under a second at the largest admissible
payload, including on large-diameter graphs where a naive hop-count
reconstruction would go quadratic.

**Errors inside a flow.** The eight nodes with a result message report a
rejected request in band, with an HTTP 200 — so a flow step reports success and
you must check `error` yourself. The two graph-producing nodes return a bare
`Graph`, which has no `error` field, so they fail out of band and abort the run;
the flow reports `graph produced no terminal result` without naming the cause,
so invoke the node directly to see the real message.

**Centrality conventions.** `eccentricity` is *outgoing* (how far a vertex can
reach). `closeness` and `harmonic` are *incoming* (summed distance from every
vertex that can reach it), which is the standard convention. `betweenness` is
computed on the unweighted hop-count topology and counts unordered pairs, so it
matches the standard unnormalised Brandes/networkx figure; the weighted variant
is deliberately not used, because it enumerates every shortest path and blows up
exponentially when shortest paths tie. Every centrality score is finite: a
vertex with no reachable peers scores 0, never infinity.

**Self-loops** are counted by `Describe`, make a graph cyclic and non-DAG, add 2
to a vertex's degree, and are part of the topology `PageRank` sees. They are
excluded only from path and spanning-tree computations, where they cannot affect
the answer.

## Correctness

Every node has golden tests with hand-computed expected values, plus
**independent oracles** that share no code with the implementation:

- exhaustive enumeration of all simple paths, cross-checked against `ShortestPath` and `Distances` for every vertex pair;
- exhaustive search over all `|V|-1` edge subsets, cross-checked against `MinimumSpanningTree`;
- the closed forms for harmonic, closeness and eccentricity centrality, on directed and asymmetric graphs as well as symmetric ones, plus betweenness cross-checked against networkx's unnormalised values;
- PageRank's exact `1/n` stationary distribution on a directed cycle, and the exact `0.9 / 0.05 / 0.05` distribution of a self-loop graph;
- Euler's circuit-rank formula `|E| - |V| + components` (counting non-self-loop edges), cross-checked against `DetectCycle`;
- validation that each returned cycle is a genuine closed walk, that each topological order really does place every edge's tail before its head, and that `Describe.average_degree` is exactly the mean of `Centrality`'s per-vertex degree scores.

Run them with `axiom test`.

## Licence

MIT — see [LICENSE](LICENSE). Built for the Axiom marketplace.

Direct runtime dependencies are `gonum.org/v1/gonum` (BSD-3-Clause) and
`google.golang.org/protobuf` (BSD-3-Clause). The deployed binary additionally
links the Axiom sidecar's transport stack: `google.golang.org/grpc` and
`google.golang.org/genproto/googleapis/rpc` (Apache-2.0), and
`golang.org/x/{net,sys,text}` (BSD-3-Clause). There is no copyleft-licensed code
anywhere in the deployed build closure — the packages actually linked into the
shipped binary. Full licence texts, and a note on why the claim is scoped to the
build closure rather than the whole module graph, are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
