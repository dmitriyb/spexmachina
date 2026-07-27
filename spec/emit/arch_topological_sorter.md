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

## Why order is emit's to decide

The parent hierarchy and lineage the project requirement calls for are expressed as refs, and a ref is only meaningful once the op it points at is known to come earlier. Ordering is therefore not a presentation choice — it is what makes `ref:op` resolvable at all. TopologicalSorter runs **before** Resolver for exactly this reason: it assigns every op_id, and Resolver then classifies each dep against the batch map it produces. An op referencing an op_id that has not yet been emitted is a forward reference the adapter cannot resolve, because the adapter's substitution table is populated as it executes, strictly in file order.

This is also why the adapter is allowed to be a single-pass sequential loop with no planner of its own. The changeset arrives already ordered; the adapter executes `ops` top to bottom and never reorders, retries out of sequence, or looks ahead. All ordering intelligence lives here, on the deterministic side of the pipeline, so two runs over the same inputs produce byte-identical changesets and the adapter has nothing left to decide.

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

## Test surface

TopologicalSorter has no public API surface independent of
`ChangesetBuilder` — only Builder consumes it. Cross-component integration
coverage (Sorter paired with Resolver, Labeler, and Builder, including the
"in-batch dep ordering preserves Resolver's ref:op classification"
scenario and the "cycle detection surfaces through Builder.Build error"
scenario) lives in `test_changeset_builder`'s `describes` array, exercised
through `Builder.Build()`'s public API. Per-method unit tests for the
ordering rules and Kahn's-algorithm tiebreaks live in
`emit/sorter_test.go` and ship with this component's implementation bead.
