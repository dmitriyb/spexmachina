# Changeset builder tests

Module integration tests covering the four-component composition that produces `changeset.json`:
`ChangesetBuilder` (id `7f06f7d80e94`), `Resolver` (id `f7775ac5f1f3`), `TopologicalSorter`
(id `7249fd093b8a`), and `IdempotencyLabeler` (id `6f4b6dd8928f`).

These four components have no public surface independent of `Builder.Build()` — the module's
`uses` graph shows ChangesetBuilder is the only consumer of the other three. Cross-component
coverage therefore lives here, exercised through `Builder.Build()`'s public API rather than
against each subordinate component in isolation. Per-method unit tests for the individual
components live in `emit/{resolver,sorter,labeler}_test.go` and are bundled with each component's
implementation bead.

## Setup

- Use the in-code fixtures described under Fixtures below: a builder environment wrapping a fake
  journal fold and a fake spec graph.
- Set `--git-head` to a fixed SHA string for byte-identical output assertions.

## Scenarios

### Canonical schema and field order

- Build a changeset from a single-create impact report. Assert the output has `"version": 2` at
  the top, `git_head` set to the fixed SHA, and canonical field order on every op (`op_id`,
  `type`, `spec_node_kind`, `spec_node_id`, `idempotency`, `parent`, `deps`, `priority`, `title`,
  `body`, `target`, `labels`, `reason`).
- Same inputs produce byte-identical output across two runs.

### Proposal epic parents every non-epic create

- Impact report with one proposal, one component create, one data_flow create. Assert: first op is
  `type: create` with `spec_node_kind: proposal_epic` and `idempotency.label: "spex:<slug>"`;
  following two creates' `parent` fields are `{"ref":"op","op_id":"<epic op_id>"}`.
- With the journal fold already listing an epic task for the slug, no epic create is emitted and
  every create's parent is `{"ref":"bead","bead_id":"<epic task>"}`.

### In-batch dep chain resolves to ref:op

- Three new components A, B, C where B uses A and C uses B. Impact emits DepSpecNodeIDs on each.
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

### Unresolvable dep is an emit error

- New component X uses spec_node Z; Z has no in-batch op and no fold entry. Assert
  `Builder.Build()` returns an error naming Z. There is no `ref:spec_node` fallback in v2 — the
  adapter reads no spex-owned file, so a dep emit cannot resolve is a hard error at emit time,
  where the operator can still fix the input.

### Priority propagation

- Component implements two module requirements with preq priorities `2` and `1`. Assert the
  create op's `priority` is `1` (lowest wins).
- Component implements a module requirement with no priority chain (no preq_id found upstream).
  Assert priority defaults per the documented fallback (`emit.FallbackPriority`).

### Obsolete + create lineage

- Modified component Q: old task `spexmachina-abc` closed, new create op. Assert:
  - Close op carries `target: {"ref":"bead","bead_id":"spexmachina-abc"}` and
    `labels: ["spex:obsolete","commit:<git-head>"]`.
  - Create op for the replacement includes `deps: [{"ref":"bead","bead_id":"spexmachina-abc","type":"blocks"}]`
    for lineage.
  - Create op's `idempotency.label` is `spex:<Q's spec_node_id>` — the same label the original
    create carried, with no lookup required, because the node's identity hash is unchanged across
    the modify pair. Nothing in the fixture needs seeding to make this true; assert it holds with
    an empty fold as well as a populated one.

### Cleanup-bead create

- Action with `Reason: "Code cleanup: m/X"`, `OldBeadID: "spexmachina-old"`,
  `SpecNodeID: "abc123def456"`. Assert the resulting create op carries:
  - `spec_node_kind: "cleanup"`.
  - `title: "Code cleanup: m/X"` (the Reason verbatim, NOT the conventional `"<module>: <node>"`
    form).
  - `labels: ["spex:cleanup"]` on the create op (the discriminator label; emit populates
    `Op.Labels` on creates, not just on closes).
  - `idempotency.label: "spex:cleanup-abc123def456"` — unique by removed-node identity hash.
  - `deps: [{"ref":"bead","bead_id":"spexmachina-old","type":"blocks"}]` — lineage from the
    closed task being cleaned up.
  - `priority: 3` (`emit.FallbackPriority`).

### Cross-component scenario: Resolver + Sorter + Labeler + Builder produce byte-identical output across runs

- **Setup**: identical impact report (multi-create batch with at least one in-batch dep and one
  existing-task dep), identical journal, identical spec graph, identical `--git-head`. Run the
  builder twice in two separate processes.
- **Components exercised together**: `Resolver` classifies each dep into ref:op or ref:bead;
  `TopologicalSorter` orders the create ops — proposal epic first, then in-batch dep
  predecessors, with lex-tiebreak among independent ops; `IdempotencyLabeler` stamps each op's
  label from its own spec_node_id; `ChangesetBuilder` composes the final changeset with canonical
  field order.
- **Assertions**:
  - First and second `changeset.json` outputs are byte-identical (SHA-256 equal).
  - Each op's `idempotency.label` matches `spex:<its own spec_node_id>`.
  - Each op's `deps` carries the expected ref shape.
  - Each op appears in topologically valid order with lex-tiebreak respected.
- **Rationale**: determinism is the headline contract, and under label-from-the-op it no longer
  depends on any store state — the same input is byte-identical whatever ran before. A single
  byte difference would indicate leaked nondeterminism (map iteration, unordered set, time-based
  field).

### Cross-component scenario: dep classification round-trip through Builder

- **Setup**: a multi-create impact report with deps exercising both live code paths — in-batch
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

- **Setup**: a constructed impact report with an in-batch dep cycle (A's DepSpecNodeIDs includes
  B; B's includes A). This is an invalid spec, fabricated for the test.
- **Components exercised together**: Resolver classifies both deps as `ref:op`, Sorter detects
  the cycle when running Kahn's algorithm, Builder receives the structured error and propagates
  it.
- **Assertions**:
  - `Builder.Build()` returns a non-nil error naming both spec_node_ids in the cycle.
  - No partial `changeset.json` is written (Builder either writes the full file or no file).

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The tests in
`emit/builder_test.go` compose a builder environment (a fake journal-fold double plus a fake spec
graph), sample impact actions, and per-scenario fold states — mixes of open, closed, and absent
task pairings seeded directly on the fake fold. Canonical-output and determinism scenarios assert
against JSON marshalled in-test rather than a golden file.

## Edge cases

- Empty impact report → changeset with only the proposal epic op (if any) or an empty op list.
- Impact report with only closes (no creates) → no parent/dep resolution needed.
- DepSpecNodeIDs with a spec_node_id that is both a closed task in the fold AND a fresh create in
  the same batch → the in-batch op wins (ref:op) over the closed pairing (dropped).
