# Changeset builder tests

Module integration tests covering the four-component composition that produces `changeset.json`:
`ChangesetBuilder` (id `4c1146bb7287`), `Resolver` (id `e9a3b1b85953`), `TopologicalSorter`
(id `659abe167891`), and `IdempotencyLabeler` (id `6efd7f8ebdb2`).

The three subordinates have no public surface independent of `Builder.Build()` — the module's
`uses` graph shows ChangesetBuilder is the only consumer of the other three. Cross-component
coverage therefore lives here, exercised through `Builder.Build()`'s public API rather than
against each subordinate component in isolation. Per-method unit tests for the individual
components live in `plan/{resolver,sorter,labeler}_test.go` and are bundled with each component's
implementation bead.

## Setup

- Use the in-code fixtures described under Fixtures below: a builder environment wrapping a fake
  journal fold, a per-scenario registration (an eid, or absent) and a fake spec graph.
- Set `--git-head` to a fixed SHA string for byte-identical output assertions.

## Scenarios

### Canonical schema and field order

- Build a changeset from a single-create action batch. Assert the output has `"version": 3` at
  the top, `git_head` set to the fixed SHA, and canonical field order on every op (`op_id`,
  `type`, `spec_node_kind`, `spec_node_id`, `spec_hash`, `idempotency`, `parent`, `deps`,
  `priority`, `title`, `body`, `target`, `labels`, `reason`) — the one sequence every op kind
  writes its own subset of. `spec_hash`'s position in the sequence is fixed, but no create shape
  populates it — only retarget ops do; assert the single create's JSON carries no `spec_hash`
  key.
- Run the same assertion over a batch carrying one create, one retarget and one close, and assert
  the retarget's `deps` are serialized **before** its `target`: one order governs every kind, and
  a retarget is where a per-kind order would show itself.
- Assert each ref object's own key names on the wire — `{"ref":"op","op_id":…}`,
  `{"ref":"bead","bead_id":…}`, and `type` on a lineage edge. The adapter reads `.op_id` off an
  in-batch dep to resolve it against the ops it has already applied, so renaming the key silently
  resolves every in-batch dep to nothing rather than failing loudly.
- Same inputs produce byte-identical output across two runs.

### Proposal epic parents every non-epic create

- An action batch with one proposal, one component create, one data_flow create; the run's
  registration carries the proposal's `registered` event (`eid: "<reg-head>:<slug>"`) and the fold
  pairs no epic task with it. Assert: first op is `type: create` with
  `spec_node_kind: proposal_epic` and `idempotency.label: "spex:<reg-head>:<slug>"` — the
  registered event's eid, not the changeset's own git_head; following two creates' `parent` fields
  are `{"ref":"op","op_id":"<epic op_id>"}`.
- With the journal fold already pairing an epic task with the registered event, no epic create is
  emitted and every create's parent is `{"ref":"bead","bead_id":"<epic task>"}`.
- With the fold pairing an epic task and **no registration at all** — the legacy shape, an epic
  whose lifecycle predates the `registered` event — the same assertion holds: parents are
  `{"ref":"bead","bead_id":"<epic task>"}` and no error is raised. The fold is asked first, so a
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

### Existing-task dep resolves to ref:bead

- New component X uses existing component Y, whose journal fold entry pairs it with an open task.
  Assert X's create has `deps: [{"ref":"bead","bead_id":"<Y's task>"}]`.

### Closed-task dep is dropped

- New component X uses Y; Y's fold entry shows its task closed (`task_closed` after its last
  `task_created`). Assert X's `deps` is empty (a closed dependency is satisfied — no edge).

### Unresolvable dep is a plan error

- New component X uses spec_node Z; Z has no in-batch op and no fold entry. Assert
  `Builder.Build()` returns an error naming Z. There is no `ref:spec_node` fallback — the
  adapter reads no spex-owned file, so a dep the builder cannot resolve is a hard error at build
  time, where the operator can still fix the input.

### Retarget op shape

- A retarget action for component X (open task `spexmachina-hun`), whose recomputed
  DepSpecNodeIDs name one in-batch create and one fold-paired open task. Assert the resulting op
  carries:
  - `type: "retarget"`.
  - `spec_node_id`: X's identity hash.
  - `spec_hash`: X's new content hash — the state the task now targets.
  - `deps`: the two recomputed refs, one `ref:op` and one `ref:bead`.
  - `target: {"ref":"bead","bead_id":"spexmachina-hun"}` — serialized after `deps`, per the one
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

### Obsolete + create lineage

- Modified component Q with a **closed** pairing: old task `spexmachina-abc`, new create op.
  Assert:
  - Close op carries `target: {"ref":"bead","bead_id":"spexmachina-abc"}` and no `labels`
    key — a close op is target and reason alone; the retired `spex:obsolete` / `commit:<HEAD>`
    markers are not emitted, because close idempotency keys on the tracker's own status.
  - Create op for the replacement includes `deps: [{"ref":"bead","bead_id":"spexmachina-abc","type":"blocks"}]`
    for lineage.
  - Create op's `idempotency.label` is `spex:<git_head>:<its op_id>` — the eid of this run's
    `modified` event, derived with no lookup and distinct from any label the original create
    carried, because each change in the lineage references its own event. Nothing in the fixture
    needs seeding to make this true; assert it holds with an empty fold as well as a populated
    one.
- The lineage pair is minted only on the closed path: rerun the fixture with the pairing open and
  assert a single retarget op with no `blocks` dep anywhere — history for that path lives in the
  journal.

### Cleanup-bead create

- Action with `Reason: "Code cleanup: m/X"`, `OldBeadID: "spexmachina-old"`,
  `SpecNodeID: "abc123def456"`. Assert the resulting create op carries:
  - `spec_node_kind: "cleanup"`.
  - `title: "Code cleanup: m/X"` (the Reason verbatim, NOT the conventional `"<module>: <node>"`
    form).
  - no `labels` key — the retired `spex:cleanup` discriminator is not emitted; what marks the
    task as cleanup tracker-side is nothing at all, because cleanup classification is answered by
    the journal (the task's `task_created` references a `removed` event), and `Op.Labels` is
    populated only on retargets.
  - `idempotency.label: "spex:<git_head>:<close op_id>"` — the eid of the `removed` event the
    same-batch close implies, so the cleanup's `task_created` referent and its label are the same
    event.

### Cleanup-bead create for a prior-batch removal

- The journal already holds the `removed` event for the node (eid `E1`, from an earlier run whose
  cleanup errored); the batch carries the cleanup create but **no** close op for that node —
  its close landed last run.
- Assert the cleanup op's `idempotency.label` is `spex:E1` — read from the fold, not derived from
  any op in this batch — so a re-run at a moved HEAD still carries the label of the removal it
  answers, and label and `task_created` referent stay one fact across runs.
  - `deps: [{"ref":"bead","bead_id":"spexmachina-old","type":"blocks"}]` — lineage from the
    closed task being cleaned up.
  - `priority: 3` (`plan.FallbackPriority`).

### Op id numbering across a batch mixing every kind

- One batch carrying all of it together: two conventional creates, one cleanup create for a
  removed node, one retarget, and two closes — one of them the removal close the cleanup answers.
- Assert op ids run from `op-1` in creates → retargets → closes order, with no gap and no reuse,
  zero-padded to the digit width of the total op count across all three kinds.
- Assert the cleanup create's `idempotency.label` is `spex:<git_head>:<the removal close's op_id>`,
  reading that close's actual `op_id` out of the emitted document rather than recomputing the
  arithmetic the builder used. The cleanup's label is derived before the close ops are numbered,
  so it rests on a prediction of where the closes will start — and the retarget block sitting
  between the creates and the closes is exactly what that prediction has to account for. A batch
  of creates and closes alone satisfies both a correct rule and one blind to retargets.

### Cross-component scenario: Resolver + Sorter + Labeler + Builder produce byte-identical output across runs

- **Setup**: identical action batch (multi-create with at least one in-batch dep, one
  existing-task dep, and one retarget), identical journal, identical spec graph, identical
  `--git-head`. Run the builder twice in two separate processes.
- **Components exercised together**: `Resolver` classifies each dep into ref:op or ref:bead;
  `TopologicalSorter` orders the create ops — proposal epic first, then in-batch dep
  predecessors, with lex-tiebreak among independent ops; `IdempotencyLabeler` stamps each create's
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
  (`ref:op`) and existing-task (`ref:bead`) — plus one dep constructed to be unresolvable.
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

### Cross-component scenario: spec_node_kind per declared type comes from the profile

- **Setup**: an action batch carrying one create per plan-relevant node type, built once under
  the default profile and once under a profile declaring an additional plan-relevant `endpoint`
  type.
- **Components exercised together**: ChangesetBuilder fills each op's `spec_node_kind` from the
  action's node type; the profile declares only which types are plan-relevant — no bead types.
- **Assertions**:
  - Under the default profile, every op's `spec_node_kind` matches today's vocabulary exactly —
    the output is byte-identical to the pre-profile builder's over the same batch.
  - Under the extended profile, the endpoint create's op carries the declared `spec_node_kind`
    (`"endpoint"`) unchanged, composed through the same canonical field order with no new op
    shape; the kind-to-bead-type mapping is the adapter's, whose default arm files the unknown
    kind as `feature`.

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The tests in
`plan/builder_test.go` compose a builder environment (a fake journal-fold double plus a fake spec
graph), sample actions, per-scenario fold states — mixes of open, in_progress, closed, and absent
task pairings seeded directly on the fake fold — and per-scenario registrations, seeded
independently of the fold so the two epic verdicts can be told apart. Canonical-output and
determinism scenarios assert against JSON marshalled in-test rather than a golden file.

## Edge cases

- Empty action batch → changeset with only the proposal epic op (if any) or an empty op list.
- A batch with only closes (no creates) → no parent/dep resolution needed.
- DepSpecNodeIDs with a spec_node_id that is both a closed task in the fold AND a fresh create in
  the same batch → the in-batch op wins (ref:op) over the closed pairing (dropped).
