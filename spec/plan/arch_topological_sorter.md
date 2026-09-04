# TopologicalSorter

Orders the create ops within a batch so every in-batch dependency comes before its dependent, and so every op sits after the layer its deps name.

## Ordering Rules

[[483fdc961d3e|Order is decided on two levels at once]], the outer one by node kind and the inner one by the spec graph:

1. **Layer** (outer): the proposal epic first; then one layer per plan-relevant node type, in the order the resolved profile's plan-relevant list declares them — `data_flow`, `component`, `test_section` under the default profile; then the cleanups last. The epic and the cleanup layer are placed by rule, outside the list: the epic is every create's parent, and a cleanup deletes what the batch's other work stops calling. The sorter tells a cleanup by the `Code cleanup:` prefix on its reason — the same discriminator ChangesetBuilder keys the cleanup op shape on, since a cleanup action arrives carrying the removed node's own type — and the epic by its `proposal_epic` kind; every other create is placed by its node type. Retarget and close ops sit after all creates in one fixed order — retargets first, then closes, each block in the classifier's deterministic action order — so the adapter builds, then adjusts, then cancels.
2. **Spec-graph deps** (inner, within each layer): if create op B declares a dep on the spec_node_id of op A, A comes before B.

The layer order is what [[4c1146bb7287|ChangesetBuilder]] turns into `ref:op` deps from each create to the previous non-empty layer's creates, so file order and tracker order say the same thing: an op's every `ref:op` — spec-graph or layer — names an op that precedes it in the file, which is what the adapter's single-pass substitution table needs.

A retarget is not layered — it targets a task that already exists, so nothing forward-references it — but its recomputed deps can point at in-batch creates, which is why retargets sit after the creates: by the time the adapter reaches one, every `ref:op` in its deps is already in the substitution table.

The creates-before-closes order carries no lineage. No create in a batch names a close's target — the modify pair that once tied a successor create to the close of its predecessor is gone, and a close is issued only for a task that is live and being cancelled — so the block order is purely the "build, adjust, cancel" reading: nothing a close does is a precondition of any create, and nothing a create does is undone by a close.

## Algorithm

Kahn's algorithm with deterministic tiebreak:

1. Build a DAG over the create ops in the current batch. Node = op; edge A → B if B declares a dep on A's spec_node_id AND A is in the same batch.
2. Partition nodes by layer. Process layers in order (epic → the list's kinds in declared order → cleanups); within a layer run Kahn. The DAG is rebuilt per layer, so a dep pointing at an earlier layer — or outside the batch — is invisible to the sort and is left for Resolver to classify. A dep pointing at a *later* layer's op would resolve to a forward `ref:op` the adapter cannot follow, so it is refused: the profile's order has to agree with the edges the classifier collects — a data_flow before the components it names, components before the test sections that describe them — and an order that inverts one is an error naming both ops, never a silently reordered file.
3. Kahn with priority queue: at each step, among nodes with zero remaining incoming edges, pick the one with the smallest `spec_node_id` lex order (tiebreak). This makes the output deterministic across runs.
4. Emit the ops in that order. The `op_id` each one carries is not a position: ChangesetBuilder derives it from the op's canonical key — its kind and the node or task it acts on — so an op has the same id whatever else the batch holds and however the profile orders the layers. Those op_ids outlive the changeset: ingest derives each journal event's id from `(git_head, op_id)`, so the derivation, not the sort, is what pins journal event identity. The sort decides only where in the file an op sits, and a reordering bug here produces a forward reference the adapter refuses rather than a renamed event.

## Why order is this module's to decide

The parent hierarchy the project requirement calls for is expressed as refs, and a ref is only meaningful once the op it points at is known to come earlier. Ordering is therefore not a presentation choice — it is what makes `ref:op` resolvable at all. TopologicalSorter runs **before** Resolver for exactly this reason: it fixes the file order every `ref:op` relies on, and Resolver then classifies each dep against the batch map built from the same set of ops. An op referencing an op that has not yet been emitted is a forward reference the adapter cannot resolve, because the adapter's substitution table is populated as it executes, strictly in file order.

This is also why the adapter is allowed to be a single-pass sequential loop with no planner of its own. The changeset arrives already ordered; the adapter executes `ops` top to bottom and never reorders, retries out of sequence, or looks ahead. All ordering intelligence lives here, on the deterministic side of the pipeline, so two runs over the same inputs produce byte-identical changesets and the adapter has nothing left to decide.

## Cycle Handling

If Kahn terminates with nodes still holding incoming edges, there is a cycle. Return an error naming every node that still carries an unsatisfied in-batch dep, listed in lex order so the same input always produces the same message. That set contains the cycle and anything left stranded behind it, which is what a reader needs to find the offending `uses` edge. The cycle is a spec bug (invalid `uses` graph) that validator's `dag_checker` should have caught; this component is the last line of defense and fails fast rather than emitting a partially-ordered changeset.

## Interface

Given the batch's create actions and the resolved profile's plan-relevant list, the sorter answers with those same actions in emitted order. Op ids are not its to hand out: ChangesetBuilder derives each from the op's canonical key and builds the spec_node_id-to-op_id map from those, so nothing provisional is issued here and nothing is renumbered later.

Three kinds of batch yield an error and no ordering at all: one holding a cycle, one holding a create whose spec node kind the plan-relevant list does not place — the epic and cleanup kinds being placed by rule — and one holding a dep that points at a later layer's op.

## Determinism

- Topological sort with smallest-lex-spec_node_id tiebreak → one valid order for any input.
- The file order is the layer order, epic first and cleanups last, so reading the changeset top to bottom reads the creates in the order the adapter needs them, and every `ref:op` in the file points upward.
- The same batch with the same declared deps under the same profile produces the same ordering on every run; reordering the profile's list reorders the file and the layer edges, and moves no op_id.

## Test surface

TopologicalSorter has no public API surface independent of
`ChangesetBuilder` — only Builder consumes it. Cross-component integration
coverage (Sorter paired with Resolver, Labeler, and Builder, including the
"in-batch dep ordering preserves Resolver's ref:op classification"
scenario, the "layer edges follow the profile's order" scenario and the
"cycle detection surfaces through Builder.Build error" scenario) lives in
`test_changeset_builder`'s `describes` array, exercised through
`Builder.Build()`'s public API. Per-method unit tests for the ordering rules
and Kahn's-algorithm tiebreaks live in `plan/sorter_test.go` and ship with
this component's implementation task.
