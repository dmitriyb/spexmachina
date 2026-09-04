# Classification Tests

Integration and acceptance tests for ActionClassifier and Resolver. These tests verify that matched, unmatched, and orphaned node results are correctly classified into create, close and retarget actions from one bounded input — the task-state artifact — and that the Resolver turns each action's spec-graph references into refs and priorities consistently with what the classifier decided. The retarget path in particular is a two-component contract: the classifier decides that a task moves, the Resolver recomputes what it now depends on.

## Setup

All scenarios build on the output of NodeMatcher. Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HUNK_HASH`, etc.) so the test data stays readable. A change identifies its node by the identity hash in `Key`, not by a content path, and a matched pairing repeats that hash as its node key — the join NodeMatcher already made. Each pairing carries the live status TaskReader's output joined onto it, or none: the values `open` and `in_progress`, plus absence, are the three things the transition table splits on. There is no `closed` value anywhere in the fixtures, because the artifact has none.

**Matched entries (modified spec nodes with existing tasks), one per split:**

- `SCHK_HASH`, modified, pairing spex-001 with no live status — the task is absent from the artifact: the plain-create path
- `HUNK_HASH`, modified, pairing spex-003 with status `open` — the retarget path
- `CLMD_HASH`, modified, pairing spex-007 with status `in_progress` — the refusal path

**Unmatched entries (new spec nodes without pairings):** one `added` component `CSCH_HASH`.

**Orphaned entries (pairings whose spec node was removed):** one pairing `LEGACY_HASH` / spex-010 carrying `NodeType: "component"` — an orphan carries the node type alongside the pairing, preserved from the removed change, because an identity hash does not embed the node type and ActionClassifier needs it downstream.

## Scenarios

### S1: The status split — one modified fixture, three verdicts

Classify the three matched entries above (absent, `open`, `in_progress`) with the unmatched and orphaned entries removed:

- `SCHK_HASH` (absent from the artifact): one plain **create** — no close against spex-001, no old task id on the action, no lineage of any kind. The predecessor's completion is not the classifier's to record; the journal pairing plus the absence is the whole story.
- `HUNK_HASH` (open pairing): one **retarget** action targeting spex-003, carrying the node's identity hash, its new content hash, and freshly recomputed `DepSpecNodeIDs`. No close, no create.
- `CLMD_HASH` (in_progress pairing): classification refuses the whole run with an error naming spex-007 — a claimed task's target never moves under it. No action list is returned at all; a partial classification never leaks.

The refusal is total, not per-entry: rerun with `CLMD_HASH` and a second in_progress entry, and assert the error names **every** claimed task whose node changed, so the operator sees the full set at once.

### S1b: A create for a modified node is the same create a fresh node gets

Compare the action `SCHK_HASH` yields in S1 with the one an unmatched `added` component yields: same action type, same field set, no field naming a prior task. The reason differs (`Spec node modified (new)` against `New spec node`) and nothing else. This pins the death of the recreate path: a classifier that still emitted a close beside the create, or stamped an old task id on it, fails here.

### S2: Already-tracked change yields no action, checked before the status split

A matched `added` or `modified` change whose pairing's sourcing event records an `after` hash equal to the change's current hash yields no action — the journal already pairs a task with exactly this state, and the change resurfaced only because a partial run left the snapshot unsaved. Vary the fixture: the same pairing with a differing `after` produces the retarget (open) or the plain create (absent). The no-action cell is consulted first — an already-tracked node is never retargeted and never re-created, whatever its status and even when the artifact no longer lists its task.

### S3: Added node with an existing task follows the same split

A Match entry whose change type is `added` but whose pairing records a different tracked hash behaves exactly as a modified one: open pairing → retarget, absent → plain create, in_progress → refuse. The change type does not bypass the status split.

### S4: Unmatched and orphaned paths

- Unmatched `added` component `CSCH_HASH` → one **create**, as before.
- Unmatched `removed` change → no action (nothing to close).
- Orphaned pairing with status `open` → one **close** action targeting spex-010 with reason `Spec node removed: <module>/<node>` — cancelling live work is a real action. No cleanup: nothing shipped.
- Orphaned pairing with status `in_progress` → the run is refused, naming spex-010, exactly as a claimed modified node refuses it. Assert the refusal, not a silent close: removing a node under a claimed task is the largest move a target can make.
- Orphaned pairing absent from the artifact → one cleanup **create** with reason `Code cleanup: <module>/<node>`, and no close — there is no live task to close, and the classifier does not invent one. The cleanup action carries no old task id and no dependency on the finished task.
- Retarget never applies to a removed node — there is no new state to move the task to.

### S5: Node-type gate is unchanged

An unmatched `added` api yields zero actions; a test_section with `len(describes) == 1` is skipped; a data_flow always produces a task action. The status split lives inside the matched and orphaned paths only and the gate in front of the unmatched path is exactly what it was.

### S5b: The gate's admitted set is the profile's plan-relevant declaration

The task-producing set the node-type gate consults is read from the resolved profile rather than from a compiled-in constant. Two arms:

- Under the default profile, S5's verdicts hold byte-for-byte — the default declares today's plan-relevant list, so api stays outside it, data_flow always produces, and the describes-length rule binds test_section.
- Under a profile declaring an `endpoint` type as plan-relevant, an unmatched `added` endpoint produces one create action carrying the type name; whether a changeset can be built from that action is the sorter's contract, not the classifier's (see `test_changeset_builder`). A profile leaving `endpoint` outside the list yields zero actions for the same change. The classifier branches on the declaration, never on the type name.

### S6: Resolver recomputes a retarget's deps add-only

Given a retarget action for component X whose `uses` now names Y (in-batch create) and Z (a task the artifact lists as open):

- The retarget op's `deps` carries `{"ref":"op","op_id":"<Y op>"}` and `{"ref":"task","task_id":"<Z's task>"}` — the same two shapes create ops use, classified by the same precedence (in-batch wins over fold).
- A dep whose fold pairing names a task absent from the artifact is dropped, exactly as on a create — the work is satisfied.
- A dep that is neither in-batch nor in the fold is a plan error naming the spec_node_id — the retarget path gets no laxer resolution than the create path.
- Nothing in the op expresses dep removal: deps the task already carries in the tracker and no longer needs are untouched (add-only is the adapter's application rule, and the op simply lists the current set).

### S7: Retarget carries the run's modified-event label

The retarget op's `labels` array holds exactly one entry: `spex:<eid>` of this run's `modified` event for the node, derived from `(git_head, op_id)` exactly as a node-bearing create's `idempotency.label` is. Assert the eid embeds this op's own op_id — two retargets in one batch carry distinct labels.

### S8: Priority and parent do not apply to retargets

A retarget op carries no `parent` and no `priority` — the task already sits under its epic with its priority; only its target state and deps move. Assert both fields are absent from the op, and that create ops in the same batch still resolve parent and priority exactly as before (minimum across the implements → preq_id → priority chain, fallback 3).

### S8b: Names are resolved from the graph, node types are carried through

- A matched component, a matched data_flow and a matched test_section, each classified with the
  graph declaring all three. Assert every resulting action's reason names the node's **declared
  name**, not its identity hash, for all three kinds — one arm per kind, since a resolver that
  handled components alone would satisfy a component-only fixture.
- Assert each action's node type equals the change's node type on the matched path, not only on
  the unmatched path where the gate reads it: the builder fills `spec_node_kind` from this field
  later, so a matched create that lost its type would file the node under the wrong tracker type.
- Two fallback cases, both resolving to the identity hash itself and neither an error: a change
  whose module the graph does not hold, and a change whose hash that module declares under no
  section of its type. Assert the reason reads `<module>/<hash>` and classification continues.
- An orphaned pairing takes its module and name from the **journal pairing**, never from the
  graph — the removed node is gone from the graph by the time the run reads it, so a cleanup's
  `Code cleanup: <module>/<node>` reason stays legible where a graph lookup would print a hash.

### S9: Deterministic classification

Classify the full fixture twice, shuffling pairing order between runs. Assert the action lists are identical in content and order — the sort by (Type, Module, Node, TaskID) covers all three action types, with retargets ordered inside the same scheme.

### S10: Resolver precedence over a test task's `describes` deps

The four fates of a described component's hash, one fixture arm each — the same precedence create deps already take, with no Resolver change:

- **In-batch create**: test_section T describes components X and Y, all three fresh creates in one batch. T's create op carries `{"ref":"op"}` deps on X's and Y's ops, and the sorter emits both component ops before T's — the test task is not actionable until its components exist.
- **Live fold pairing**: Y instead has a task the artifact lists as open → T's dep on Y resolves to `ref:task`; the test task waits for the in-flight component work.
- **Pairing absent from the artifact**: Y's task is not listed → the dep is dropped, no error, and T's op carries no ref for it — the steady state, where a test against existing code stays immediately actionable.
- **No pairing at all**: Y has never been tracked by any journal event → the existing plan error naming the spec_node_id, exactly as a component `uses` dep resolves. Assert the error, not a silent drop.

A retargeted test section takes the same path: its recomputed `DepSpecNodeIDs` include `describes`, so a section retargeted in a batch that re-mints a described component gains a `ref:op` dep on the successor, applied add-only per S6.

## Dependency Collection Scenarios

For each create or retarget action, ActionClassifier walks the spec graph and records identity hashes of spec nodes the task will depend on in `DepSpecNodeIDs`. No journal lookup and no status filtering happens here — the Resolver classifies each identity hash into a `ref:op` or `ref:task` at changeset-build time, with full knowledge of the batch's op ids.

### D1: Component `uses` edge collects the sibling's identity hash

Component X `uses: [Y]` in the same module: X's `DepSpecNodeIDs` contains `id_Y`. A self-reference is filtered out. Holds identically when X's action is a retarget.

### D2: Live status is irrelevant to collection

The artifact does not list Y's task: X's `DepSpecNodeIDs` still contains `id_Y`. Filtering already-satisfied deps is the Resolver's responsibility, not the classifier's.

### D3: Transitive `requires_module` collects every reachable module's components

A `requires_module: [B]`, B `requires_module: [C]`: a create in A collects both `id_CompB` and `id_CompC`. Cycles in `requires_module` terminate via visited-set tracking and collect each reachable module once.

### D4: Component `uses` edges are NOT transitive

X `uses: [Y]`, Y `uses: [Z]`: X collects only `id_Y`. Only `requires_module` is walked transitively.

### D5: Mixed `uses` and `requires_module` are merged and deduplicated

Duplicates collapse to one entry; the output is a set.

### D6: No edges yields empty `DepSpecNodeIDs`

Length zero, on creates and retargets alike.

### D7: Data_flow add-on — component gains the flow's identity hash when both are in the same batch

Data_flow F with `uses: [X]`, both F and X in the batch: X's `DepSpecNodeIDs` contains `id_F`, so the layer order places F's op before X's and X gains a `ref:op` dep on F. Components the flow does not list gain nothing, and a flow outside the batch adds nothing — pre-existing flow deps resolve to `ref:task` from the fold, or drop when the flow's task is finished.

### D8: Non-component creates do not walk `uses` / `requires_module`

Data_flow creates carry no spec-graph deps from classification; their place in the batch is the data_flow layer's, ahead of the components, and the add-on gives the components on the other side their edges. A test_section create is not dep-free — it collects its `describes` array (D10) — but it walks no `uses` and no `requires_module`; those two walks stay component-only.

### D9: Close actions never carry `DepSpecNodeIDs`

Dependency information belongs on creates and retargets only — a close cancels an existing task and inherits nothing.

### D10: Test_section `describes` collects each described component's identity hash

Section T `describes: [X, Y]`: T's `DepSpecNodeIDs` contains `id_X` and `id_Y`, on creates and retargets alike. Collection is unconditional — no journal lookup, no status filtering, no batch-awareness; D2's division of labour holds here too, and whether each hash becomes a ref, a drop or an error is S10's business.

## Edge Cases

### E1: Empty inputs produce an empty action list

Classify with all three lists empty. Assert an empty action list, not an error.

### E2: Duplicate entries are preserved, not deduplicated

If the same spec node change appears in both `matches` and `unmatched` due to upstream bugs, both entries produce actions. Deduplication is not the classifier's responsibility — it faithfully translates its inputs.

### E3: Refusal is total over what the classifier receives

A batch holding one in_progress-claimed change and several cleanly classifiable ones refuses entirely — the claimed task's error is not deferred while the rest proceeds. Marked cosmetic changes never appear in the classifier's input at all: PlanCommand withholds them from matching before any pairing is consulted, so no absorb rule exists at this layer to test — that path is covered in the plan command tests, where the absorb file lives.

### E4: Fold-back beats the status split, and takes the removal rules

A matched `modified` test_section whose current `describes` holds one component, in three variants by its pairing's status:

- `open` → exactly one **close** action with reason `Spec node modified: <module>/<node>`, no retarget, no successor create: the node no longer owes a task of its own, so the open one is cancelled.
- `in_progress` → the run is refused naming the task, the same protection every claimed task has.
- absent from the artifact → no action at all: the section's task is finished and the node owes nothing further.

A fourth variant with `describes` still ≥ 2 and status `open` yields the retarget, proving the precedence is the describes length, not the node type. The fold-back is consulted before the status split, and it reads the split the way a removal does — because for the section's own task, dropping to one component is a removal.
