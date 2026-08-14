# TopologicalSorter

Orders the create ops within a batch so every in-batch dependency comes before its dependent.

## Ordering Rules

[[483fdc961d3e|Order is decided on two levels at once]], the outer one by node kind and the inner one by the spec graph:

1. **Type tier** (outer): proposal epic first, then features + data_flow tasks, then multi-component test tasks. Retarget and close ops sit after all creates in one fixed order — retargets first, then closes, each block in the classifier's deterministic action order — so the adapter builds, then adjusts, then closes, and two runs number the same ops identically.
2. **Spec-graph deps** (inner, within each tier): if create op B declares a dep on the spec_node_id of op A, A comes before B.

A retarget is not tiered — it targets a task that already exists, so nothing forward-references it — but its recomputed deps can point at in-batch creates, which is why retargets sit after the creates: by the time the adapter reaches one, every `ref:op` in its deps is already in the substitution table.

## Algorithm

Kahn's algorithm with deterministic tiebreak:

1. Build a DAG over the create ops in the current batch. Node = op; edge A → B if B declares a dep on A's spec_node_id AND A is in the same batch.
2. Partition nodes by type tier. Process tiers in order (epic → features/flows → test tasks); within a tier run Kahn. The DAG is rebuilt per tier, so a dep pointing outside the tier — or outside the batch — is invisible to the sort and is left for Resolver to classify.
3. Kahn with priority queue: at each step, among nodes with zero remaining incoming edges, pick the one with the smallest `spec_node_id` lex order (tiebreak). This makes the output deterministic across runs.
4. Emit the ops in that order. The `op_id` each one carries in the changeset is stamped downstream by ChangesetBuilder, in this order and numbered from 1. Those op_ids now outlive the changeset: ingest derives each journal event's id from `(git_head, op_id)`, so the sorter's deterministic ordering is also what pins journal event identity — a reordering bug here would mint new event ids for old work and break ingest's append-nothing re-runs.

## Why order is this module's to decide

The parent hierarchy and lineage the project requirement calls for are expressed as refs, and a ref is only meaningful once the op it points at is known to come earlier. Ordering is therefore not a presentation choice — it is what makes `ref:op` resolvable at all. TopologicalSorter runs **before** Resolver for exactly this reason: it fixes the order the op_ids are handed out in, and Resolver then classifies each dep against the batch map built from that order. An op referencing an op_id that has not yet been emitted is a forward reference the adapter cannot resolve, because the adapter's substitution table is populated as it executes, strictly in file order.

This is also why the adapter is allowed to be a single-pass sequential loop with no planner of its own. The changeset arrives already ordered; the adapter executes `ops` top to bottom and never reorders, retries out of sequence, or looks ahead. All ordering intelligence lives here, on the deterministic side of the pipeline, so two runs over the same inputs produce byte-identical changesets and the adapter has nothing left to decide.

## Cycle Handling

If Kahn terminates with nodes still holding incoming edges, there is a cycle. Return an error naming every node that still carries an unsatisfied in-batch dep, listed in lex order so the same input always produces the same message. That set contains the cycle and anything left stranded behind it, which is what a reader needs to find the offending `uses` edge. The cycle is a spec bug (invalid `uses` graph) that validator's `dag_checker` should have caught; this component is the last line of defense and fails fast rather than emitting a partially-ordered changeset.

## Interface

Given the batch's create actions, the sorter answers with those same actions in emitted order, plus a provisional op_id per action and a spec_node_id-to-op_id map built from it. ChangesetBuilder keeps the order and discards both: it renumbers every op itself once the close and retarget ops are counted, and rebuilds the map from its own numbering.

Two kinds of batch yield an error and no ordering at all: one holding a cycle, and one holding a create whose spec node kind belongs to no tier.

## Determinism

- Topological sort with smallest-lex-spec_node_id tiebreak → one valid order for any input.
- Ops are numbered from 1 in emitted order, so reading the changeset top to bottom reads the creates in sort order. The id strings and their zero-padding width are ChangesetBuilder's: the width covers the close and retarget ops too, and the sorter is handed only the creates.
- The same batch with the same declared deps produces the same ordering on every run.

## Test surface

TopologicalSorter has no public API surface independent of
`ChangesetBuilder` — only Builder consumes it. Cross-component integration
coverage (Sorter paired with Resolver, Labeler, and Builder, including the
"in-batch dep ordering preserves Resolver's ref:op classification"
scenario and the "cycle detection surfaces through Builder.Build error"
scenario) lives in `test_changeset_builder`'s `describes` array, exercised
through `Builder.Build()`'s public API. Per-method unit tests for the
ordering rules and Kahn's-algorithm tiebreaks live in
`plan/sorter_test.go` and ship with this component's implementation bead.
