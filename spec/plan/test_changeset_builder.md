# Changeset builder tests

Module integration tests covering the four-component composition that produces `changeset.json`:
`ChangesetBuilder` (id `4c1146bb7287`), `Resolver` (id `e9a3b1b85953`), `TopologicalSorter`
(id `659abe167891`), and `IdempotencyLabeler` (id `6efd7f8ebdb2`).

The three subordinates have no public surface independent of `Builder.Build()` — the module's
`uses` graph shows ChangesetBuilder is the only consumer of the other three. Cross-component
coverage therefore lives here, exercised through `Builder.Build()`'s public API rather than
against each subordinate component in isolation. Per-method unit tests for the individual
components live in `plan/{resolver,sorter,labeler}_test.go` and are bundled with each component's
implementation task.

## Setup

- Use the in-code fixtures described under Fixtures below: a builder environment wrapping a fake
  journal fold, a per-scenario registration (an eid, or absent), a fake spec graph, and a
  plan-relevant list — the default profile's unless the scenario says otherwise.
- Set `--git-head` to a fixed SHA string for byte-identical output assertions.

## Scenarios

### Canonical schema and field order

- Build a changeset from a single-create action batch. Assert the output has `"version": 4` at
  the top, `git_head` set to the fixed SHA, and canonical field order on every op (`op_id`,
  `type`, `spec_node_kind`, `spec_node_id`, `spec_hash`, `idempotency`, `parent`, `deps`,
  `priority`, `title`, `body`, `target`, `labels`, `reason`) — the one sequence every op kind
  writes its own subset of. `spec_hash`'s position in the sequence is fixed, but no create shape
  populates it — only retarget ops do; assert the single create's JSON carries no `spec_hash`
  key.
- Run the same assertion over a batch carrying one create, one retarget and one close, and assert
  the retarget's `deps` are serialized **before** its `target`: one order governs every kind, and
  a retarget is where a per-kind order would show itself.
- Assert each ref object's own key names on the wire — `{"ref":"op","op_id":…}` and
  `{"ref":"task","task_id":…}` — and that no ref carries any further key: the edge-type field
  left with the lineage edge, and a `type` key on a dep would be a v3 shape leaking through. The
  adapter reads `.op_id` off an in-batch dep to resolve it against the ops it has already
  applied, so renaming the key silently resolves every in-batch dep to nothing rather than
  failing loudly.
- Same inputs produce byte-identical output across two runs.

### Proposal epic parents every non-epic create

- An action batch with one proposal, one component create, one data_flow create; the run's
  registration carries the proposal's `registered` event (`eid: "<reg-head>:<slug>"`) and the fold
  pairs no epic task with it. Assert: first op is `type: create` with
  `spec_node_kind: proposal_epic` and `idempotency.label: "spex:<reg-head>:<slug>"` — the
  registered event's eid, not the changeset's own git_head; following two creates' `parent` fields
  are `{"ref":"op","op_id":"<epic op_id>"}`.
- With the journal fold already pairing an epic task with the registered event, no epic create is
  emitted and every create's parent is `{"ref":"task","task_id":"<epic task>"}`.
- With the fold pairing an epic task and **no registration at all** — the legacy shape, an epic
  whose lifecycle predates the `registered` event — the same assertion holds: parents are
  `{"ref":"task","task_id":"<epic task>"}` and no error is raised. The fold is asked first, so a
  live epic task settles the question before the registration is consulted.
- With no epic pairing and no registration for the proposal, `Builder.Build()` returns an error
  naming the slug — registration opens the lifecycle, so plan refuses to synthesize an epic
  referent.
- The first and last cases share an empty fold and differ only in the registration, which is the
  point of asserting them as a pair: an absent epic pairing means "no epic task yet" and never
  "never registered", so the fold's silence alone must not decide either verdict.

### In-batch dep chain resolves to ref:op

- Three new components A, B, C where B uses A and C uses B, each with DepSpecNodeIDs collected.
  Assert:
  - A's create has empty `deps`.
  - B's create has `deps: [{"ref":"op","op_id":"<A op>"}]`.
  - C's create has `deps: [{"ref":"op","op_id":"<B op>"}]`.
  - The batch holds components only, so no layer edge joins them: the deps are the spec graph's
    alone.

### Layer edges follow the profile's order

- Under the default profile, a batch of: the proposal epic create, two data_flow creates F1 and
  F2, three component creates A, B, C where B uses A and only A appears in F1's `uses`, and one
  multi-component test_section create T describing B. Assert:
  - the file order is the epic, then F1 and F2 (lex order), then A, B, C, then T.
  - F1 and F2 carry no deps: the epic is no layer's predecessor, so the first non-empty layer
    has spec-graph deps alone.
  - A's `deps` is exactly `[{"ref":"op","op_id":"<F1>"}, {"ref":"op","op_id":"<F2>"}]` — F1
    once, although the add-on and the layer edge both name it; B's is F1, F2, A; C's is F1, F2:
    every create of the previous layer, in file order, plus the spec graph's own.
  - T's `deps` is A, B, C as `ref:op` — the whole component layer, not only the B it describes —
    and names neither flow: adjacent layers only.
- Rerun with F1 and F2 removed from the batch and assert A, B and C carry spec-graph deps alone
  while T still carries A, B, C: the previous *non-empty* layer is the predecessor, and an empty
  layer is skipped, not waited on.
- Rerun under a profile whose plan-relevant list reads `component, data_flow, test_section` and
  assert `Build()` returns an error naming A and F1: the add-on makes A depend on F1, which now
  sits in a later layer, and a forward `ref:op` is refused rather than emitted.
- Rerun under a profile that appends an `endpoint` type to the list, with one `endpoint` create
  E in the batch, and assert E is emitted last among the non-cleanup creates with
  `spec_node_kind: "endpoint"` and deps naming T as `ref:op`; then strike `endpoint` from the
  list, keep the create, and assert `Build()` errors naming the kind — the profile places kinds,
  and an unplaced one is refused.

### Live-task dep resolves to ref:task

- New component X uses existing component Y, whose journal fold entry pairs it with a task the
  task-state artifact lists as open. Assert X's create has
  `deps: [{"ref":"task","task_id":"<Y's task>"}]`. Repeat with the task listed as `in_progress`
  and assert the same ref — a claimed dependency is still live work to wait on.

### Finished-task dep is dropped

- New component X uses Y; Y's fold entry pairs it with a task the artifact does not list. Assert
  X's `deps` is empty: a dependency whose task is absent from the artifact is satisfied, and no
  edge is written. Nothing in the fixture's journal says the task closed — no `task_closed` line
  exists for it — because absence from the artifact is the only signal the builder reads.

### Unresolvable dep is a plan error

- New component X uses spec_node Z; Z has no in-batch op and no fold entry. Assert
  `Builder.Build()` returns an error naming Z. There is no `ref:spec_node` fallback — the
  adapter reads no spex-owned file, so a dep the builder cannot resolve is a hard error at build
  time, where the operator can still fix the input.

### Retarget op shape

- A retarget action for component X (open task `spexmachina-hun`), whose recomputed
  DepSpecNodeIDs name one in-batch create and one fold-paired live task. Assert the resulting op
  carries:
  - `type: "retarget"`.
  - `spec_node_id`: X's identity hash.
  - `spec_hash`: X's new content hash — the state the task now targets.
  - `deps`: the two recomputed refs, one `ref:op` and one `ref:task`.
  - `target: {"ref":"task","task_id":"spexmachina-hun"}` — serialized after `deps`, per the one
    canonical order.
  - `labels: ["spex:<git_head>:<its op_id>"]` — the eid of this run's `modified` event for X,
    derived from `(git_head, op_id)` like every node-bearing create's label; no
    `idempotency` field, because updates need no probe.
  - no `parent`, no `priority`, no `body` — the task already sits where it sits.
- Two retargets in one batch carry distinct labels (each embeds its own op_id).
- A batch of only retargets emits no close ops and no epic op when the fold already pairs one.

### Absorbed array

- Build with one composed absorbed entry for node N (reason R) alongside an unrelated action
  batch — the entry arrives finished from the command layer, and no action for N is in the
  batch, because absorption withheld N's change upstream of classification. Assert:
  - No op in `ops` names N, and the builder consulted no absorb rule — it received entries,
    not a list to filter.
  - The top-level `absorbed` array carries N's entry verbatim: node, before hash, after hash,
    reason R, in canonical field order.
  - With no absorbed entries the `absorbed` key is an empty array (or omitted — assert
    whichever the canonical form fixes) and byte-identical output still holds.

### Priority propagation

- Component implements two module requirements with preq priorities `2` and `1`. Assert the
  create op's `priority` is `1` (lowest wins).
- Component implements a module requirement with no priority chain (no preq_id found upstream).
  Assert priority defaults per the documented fallback (`plan.FallbackPriority`).
- Component implements three requirements where one is broken — its `preq_id` names no project
  requirement — and the other two resolve to `2` and `4`. Assert `priority` is `2`: the minimum
  runs over the reachable requirements, so one unreachable entry is skipped rather than
  collapsing the whole walk to the fallback. The fallback answers only when *no* entry is
  reachable, which is the case the scenario above pins.

### A modified node's create carries no lineage

- Modified component Q whose fold pairing names task `spexmachina-abc`, absent from the
  task-state artifact — the classifier emitted one plain create for it. Assert:
  - The changeset carries no close op at all: `spexmachina-abc` is named nowhere in the
    document.
  - The create op's `deps` carries only what Q's spec-graph edges resolve to — no
    `{"ref":"task","task_id":"spexmachina-abc"}` entry, typed or otherwise.
  - The create op's `idempotency.label` is `spex:<git_head>:<its op_id>` — the eid of this run's
    `modified` event, derived with no lookup and distinct from any label the original create
    carried, because each change in the lineage references its own event. Nothing in the fixture
    needs seeding to make this true; assert it holds with an empty fold as well as a populated
    one.
- Rerun the fixture with the pairing's task listed as open and assert a single retarget op —
  again with no close and no lineage dep. The two paths differ in whether the task moves or a
  successor is born; neither writes history into the tracker, because the journal holds it.

### Cleanup create

- Action with `Reason: "Code cleanup: m/X"`, `SpecNodeID: "abc123def456"`, from a removed node
  whose task the artifact does not list. Assert the resulting create op carries:
  - `spec_node_kind: "cleanup"`.
  - `title: "Code cleanup: m/X"` (the Reason verbatim, NOT the conventional `"<module>: <node>"`
    form).
  - `deps` empty — the fixture holds nothing else for the layer edges to name; the "Cleanup layer
    waits" scenario below covers them — and in any batch nothing naming the finished task: the
    cleanup's tie to the node is its label and its `task_created` referent, not a tracker edge.
    No `labels` key: what marks the task as
    cleanup tracker-side is nothing at all, because cleanup classification is answered by the
    journal (the task's `task_created` references a `removed` event), and `Op.Labels` is
    populated only on retargets.
  - `idempotency.label: "spex:<git_head>:<its own op_id>"` — the eid of the `removed` event this
    cleanup op will itself mint at ingest, derived exactly as a node-bearing create's label is.
    No close op accompanies a cleanup (the task the removed node had is finished, so there is
    nothing to close), so nothing else in the batch could mint the removal, and the cleanup's
    `task_created` referent and its label are the same event by the same derivation.
  - The changeset carries no close op naming the removed node at all.
  - `priority: 3` (`plan.FallbackPriority`).

### Cleanup layer waits for the batch's last layer and its retargets

- A batch of: the proposal epic create, two component creates, one multi-component test_section
  create T, one retarget of open task `spexmachina-hun`, and two cleanup actions
  (`Code cleanup: m/X`, `Code cleanup: m/Y`). Assert:
  - the two cleanup ops are the last creates, their order decided by the lex tiebreak on
    `spec_node_id`.
  - each cleanup's `deps` is exactly
    `[{"ref":"op","op_id":"<T>"}, {"ref":"task","task_id":"spexmachina-hun"}]` — the previous
    non-empty layer is the test_section layer alone, the components being two layers back, and
    the retarget's target rides beside it.
  - neither cleanup names the other: cleanups delete independently.
- Rerun with T removed and assert each cleanup names the two component creates instead: the
  layer before the cleanups is whichever non-empty layer is nearest.
- Rerun with a batch holding one cleanup action and nothing else, and assert its `deps` is empty:
  what landed in an earlier run is outside the batch and is not named.

### Cleanup create for a prior-batch removal

- The journal's latest change event for the node is a `removed` event (eid `E1`, from an earlier
  run whose cleanup errored); the batch carries the cleanup create again.
- Assert the cleanup op's `idempotency.label` is `spex:E1` — read from the fold, not derived from
  this op — so a re-run at a moved HEAD still carries the label of the removal it answers, and
  label and `task_created` referent stay one fact across runs.
  - `deps` names nothing for the finished task — empty here because the cleanup is the whole
    batch.
  - `priority: 3` (`plan.FallbackPriority`).
- Vary the fixture so the node's `removed` event `E1` is followed by an `added` event (the node
  was re-added and is now removed a second time): assert the label is this op's own
  `spex:<git_head>:<op_id>`, not `spex:E1`. The fold answers only when the removal is the
  node's latest state; an older removal in the lineage is history, not a referent.

### Op ids are canonical keys across a batch mixing every kind

- One batch carrying all of it together: a component create and a data_flow create, one cleanup
  create for a removed node whose task is finished, one retarget, and two closes — a removal
  close for a node whose task is open, and a fold-back close on a live test_section.
- Assert every op_id is `op-<kind>-<key>`: `op-component-<hash>` and `op-data_flow-<hash>` for
  the conventional creates, `op-cleanup-<hash>` for the cleanup, `op-retarget-<hash>` for the
  retarget, `op-close-<task_id>` for each close — and `op-proposal_epic-<proposal ref>` for the
  epic when one is emitted. No id encodes a position, and no two ids collide.
- Assert the file order is still creates → retargets → closes, the creates layered, so the ids
  and the order are two facts, not one.
- Add a second component create to the batch and rebuild: every op already present keeps its
  op_id byte for byte, so the eids ingest derives from `(git_head, op_id)` still name the same
  events.
- Assert the cleanup create's `idempotency.label` embeds the cleanup op's own `op_id` as read out
  of the emitted document, and that no label anywhere in the document embeds a close op's id:
  every label is derived from the op that carries it or read from the fold, never predicted
  from another op.

### Cross-component scenario: Resolver + Sorter + Labeler + Builder produce byte-identical output across runs

- **Setup**: identical action batch (multi-create with at least one in-batch dep, one
  live-task dep, and one retarget), identical journal, identical spec graph, identical
  `--git-head`. Run the builder twice in two separate processes.
- **Components exercised together**: `Resolver` classifies each dep into ref:op or ref:task;
  `TopologicalSorter` orders the create ops — proposal epic first, then layer by layer in the
  profile's order, in-batch dep predecessors first inside each, with lex-tiebreak among
  independent ops; `IdempotencyLabeler` stamps each create's
  label with its referent event's eid; `ChangesetBuilder` composes the final changeset with
  canonical field order, retarget ops and the absorbed array included.
- **Assertions**:
  - First and second `changeset.json` outputs are byte-identical (SHA-256 equal).
  - Each create's `idempotency.label` matches `spex:<eid>` of its referent event —
    `<git_head>:<its own op_id>` for node-bearing creates, the registered event's eid for the
    epic — and the retarget's `labels` entry follows the same derivation.
  - Each op's `deps` carries the expected ref shape.
  - Each op appears in topologically valid order with lex-tiebreak respected.
- **Rationale**: determinism is the headline contract, and under label-from-the-op it no longer
  depends on any store state — the same input is byte-identical whatever ran before. A single
  byte difference would indicate leaked nondeterminism (map iteration, unordered set, time-based
  field).

### Cross-component scenario: dep classification round-trip through Builder

- **Setup**: a multi-create action batch with deps exercising both live code paths — in-batch
  (`ref:op`) and live-task (`ref:task`) — plus one dep constructed to be unresolvable.
- **Components exercised together**: Resolver classifies, Sorter orders so the in-batch
  predecessor is sequenced before its dependent, Builder composes or refuses.
- **Assertions**:
  - With the unresolvable dep removed: the dependent create's `deps` array contains exactly the
    two ref shapes, each with the correct fields; the in-batch predecessor op appears earlier in
    the changeset than its dependent; neither shape is flattened or coerced.
  - With the unresolvable dep present: `Builder.Build()` errors naming the spec_node_id, and no
    partial changeset is written.

### Cross-component scenario: cycle detection surfaces through Builder.Build error

- **Setup**: a constructed action batch with an in-batch dep cycle (A's DepSpecNodeIDs includes
  B; B's includes A). This is an invalid spec, fabricated for the test.
- **Components exercised together**: Resolver classifies both deps as `ref:op`, Sorter detects
  the cycle when running Kahn's algorithm, Builder receives the structured error and propagates
  it.
- **Assertions**:
  - `Builder.Build()` returns a non-nil error naming both spec_node_ids in the cycle.
  - No partial `changeset.json` is written (Builder either writes the full file or no file).

### Cross-component scenario: spec_node_kind and layer per declared type come from the profile

- **Setup**: an action batch carrying one create per plan-relevant node type, built once under
  the default profile and once under a profile declaring an additional plan-relevant `endpoint`
  type, appended to its list, with one `endpoint` create in the batch.
- **Components exercised together**: ChangesetBuilder fills each op's `spec_node_kind` from the
  action's node type and adds the layer edges; TopologicalSorter layers the creates by the
  profile's list; the profile declares which types are plan-relevant and in what order — no
  tracker types.
- **Assertions**:
  - Under the default profile, every op's `spec_node_kind` matches today's vocabulary exactly,
    and the layers run data_flow, component, test_section.
  - Under the extended profile, `Build()` succeeds: the `endpoint` create is emitted with
    `spec_node_kind: "endpoint"`, last among the non-cleanup creates, its deps naming every
    test_section create as `ref:op`. ChangesetBuilder copies `spec_node_kind` from the action's
    node type verbatim and validates nothing against a vocabulary of its own — the placement is
    the list's.
  - Under the extended profile with `endpoint` struck from the list but the create still in the
    batch, `Build()` returns an error naming the kind and writes no changeset: the sorter
    refuses a create the list does not place, surfaced through `Build()`.

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The tests in
`plan/builder_test.go` compose a builder environment (a fake journal-fold double plus a fake spec
graph), sample actions, per-scenario fold states — mixes of open, in_progress and unlisted task
pairings, plus nodes with no pairing at all, seeded directly on the fake fold — and per-scenario
registrations, seeded independently of the fold so the two epic verdicts can be told apart.
Canonical-output and determinism scenarios assert against JSON marshalled in-test rather than a
golden file.

## Edge cases

- Empty action batch → changeset with only the proposal epic op (if any) or an empty op list.
- A batch with only closes (no creates) → no parent/dep resolution needed.
- DepSpecNodeIDs with a spec_node_id that is both a finished task in the fold AND a fresh create in
  the same batch → the in-batch op wins (ref:op) over the unlisted pairing (dropped).
