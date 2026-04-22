# TopologicalSorter

Orders the create ops within a batch so every in-batch dependency comes before its dependent. Assigns each create op an `op_id` in the resulting order so Resolver can reference them.

## Ordering Rules

Two-level ordering:

1. **Type tier** (outer): proposal epic first, then features + data_flow tasks, then multi-component test tasks. Close ops can be freely interleaved but are conventionally placed after all creates for a given proposal wave so the adapter builds-then-closes rather than juggling.
2. **Spec-graph deps** (inner, within each tier): if create op B's `DepSpecNodeIDs` includes the spec_node_id of op A, A comes before B.

## Algorithm

Kahn's algorithm with deterministic tiebreak:

1. Build a DAG over the create ops in the current batch. Node = op; edge A → B if B's `DepSpecNodeIDs` contains A's spec_node_id AND A is in the same batch.
2. Partition nodes by type tier. Process tiers in order (epic → features/flows → test tasks); within a tier run Kahn.
3. Kahn with priority queue: at each step, among nodes with zero remaining incoming edges, pick the one with the smallest `spec_node_id` lex order (tiebreak). This makes the output deterministic across runs.
4. Assign `op_id` as a zero-padded sequential string (`op-0001`, `op-0002`, …) in the emitted order.

## Cycle Handling

If Kahn terminates with nodes still having incoming edges, there's a cycle. Return an error naming every node in the cycle's strongly-connected component — the cycle is a spec bug (invalid `uses` graph) that validator's `dag_checker` should have caught; emit is the last line of defense and fails fast rather than emitting a partially-ordered changeset.

## Interface

```go
type Sorter struct{}

func (s *Sorter) Sort(ops []CreateOp) ([]OrderedOp, error)
```

`OrderedOp` carries the assigned `op_id` plus the original `CreateOp`. Batch-local spec_node_id → op_id map is also returned (for Resolver to consume).

## Determinism

- Topological sort with smallest-lex-spec_node_id tiebreak → one valid order for any input.
- `op_id` format is stable: `op-0001` through `op-NNNN` padded to the batch's max digit count.
- Same `(batch, DepSpecNodeIDs)` produces the same ordering on every run.
