# Changeset builder tests

Module integration tests covering the four-component composition that
produces `changeset.json`: `ChangesetBuilder` (id `7f06f7d80e94`), `Resolver`
(id `f7775ac5f1f3`), `TopologicalSorter` (id `7249fd093b8a`), and
`IdempotencyLabeler` (id `6f4b6dd8928f`).

These four components have no public surface independent of `Builder.Build()` —
the module's `uses` graph shows ChangesetBuilder is the only consumer of the
other three. Cross-component coverage therefore lives here, exercised through
`Builder.Build()`'s public API rather than against each subordinate component
in isolation. Per-method unit tests for the individual components live in
`emit/{resolver,sorter,labeler}_test.go` and are bundled with each
component's implementation bead.

## Setup

- Use `testdata/` fixtures: impact_report.json, bead_map.json, project.json,
  and a minimal spec tree.
- Set `--git-head` to a fixed SHA string for byte-identical output assertions.

## Scenarios

### Canonical schema and field order

- Build a changeset from a single-create impact report. Assert the output has
  `"version": 1` at the top, `git_head` set to the fixed SHA, and canonical
  field order on every op (`op_id`, `type`, `target`, `spec_node_id`,
  `idempotency`, `parent`, `deps`, `priority`, `title`, `body`).
- Same inputs produce byte-identical output across two runs.

### Proposal epic parents every non-epic create

- Impact report with one proposal, one component create, one data_flow create.
  Assert: first op is `type: create` with `spec_node_kind: proposal_epic`;
  following two creates' `parent` fields are `{"ref":"op","op_id":"<epic op_id>"}`.

### In-batch dep chain resolves to ref:op

- Three new components A, B, C where B uses A and C uses B. Impact emits
  DepSpecNodeIDs on each. Assert:
  - A's create has empty `deps`.
  - B's create has `deps: [{"ref":"op","op_id":"<A op>"}]`.
  - C's create has `deps: [{"ref":"op","op_id":"<B op>"}]`.

### Existing-bead dep resolves to ref:bead

- New component X uses existing-and-open component Y (Y has a mapping record
  with `status: "open"`). Assert X's create has `deps: [{"ref":"bead","bead_id":"<Y's bead>"}]`.

### Closed-bead dep is dropped

- New component X uses Y; Y's mapping record has `status: "closed"`. Assert
  X's `deps` is empty (closed dependency is satisfied — no edge).

### Spec-node fallback when mapping has no record

- New component X uses spec_node Z; Z has no mapping record and no in-batch
  op. Assert X's deps include `{"ref":"spec_node","spec_node_id":"<Z>"}` so
  the adapter can resolve at exec time.

### Priority propagation

- Component implements two module requirements with preq priorities `2` and
  `1`. Assert the create op's `priority` is `1` (lowest wins).
- Component implements a module requirement with no priority chain (no
  preq_id found upstream). Assert priority defaults per the documented
  fallback (e.g., 3 or omitted — verify against impl).

### Obsolete + create lineage

- Modified component Q: old bead `spexmachina-abc` closed, new create op. Assert:
  - Close op carries `target: {"ref":"bead","bead_id":"spexmachina-abc"}` and `labels: ["spex:obsolete","commit:<git-head>"]`.
  - Create op for the replacement includes `deps: [{"ref":"bead","bead_id":"spexmachina-abc","type":"blocks"}]` for lineage.
  - Create op's `idempotency.label` MUST equal the existing record's id, NOT a freshly-reserved sequential value. Seed a record with `bead_id="spexmachina-abc"` and `id=42` in the fixture; assert the create op carries `idempotency: {"label":"spex:42"}`. This is the modify-pair record-id-reuse rule from `arch_idempotency_labeler.md`: `LabelFor` looks up the existing record via `MappingStore.GetByBead(action.OldBeadID)` and returns `spex:<existing-rec.ID>` so Reconciler hits the modify-pair-update branch and rebinds the bead_id rather than inserting a parallel record.

### Cleanup-bead create

- Action with `Reason: "Code cleanup: m/X"`, `OldBeadID: "spexmachina-old"`, `SpecNodeID: "abc123def456"`. Assert the resulting create op carries:
  - `spec_node_kind: "cleanup"` (the new vocabulary value distinct from `component`/`data_flow`/`test_section`).
  - `title: "Code cleanup: m/X"` (the Reason verbatim, NOT the conventional `"<module>: <node>"` form).
  - `labels: ["spex:cleanup"]` on the create op (the discriminator label; emit populates `Op.Labels` on creates, not just on closes).
  - `idempotency.label: "spex:cleanup-abc123def456"` — unique by removed-node identity hash; does NOT consume the Labeler's cursor.
  - `deps: [{"ref":"bead","bead_id":"spexmachina-old","type":"blocks"}]` — lineage from the closed bead being cleaned up.
  - `priority: 3` (`emit.FallbackPriority`).
- Cursor non-advancement: build a changeset containing one cleanup create AND one fresh component create. Assert the fresh create's `spex:<n>` label uses the cursor value the Labeler would have returned WITHOUT the cleanup op present (the cleanup did not bump the cursor).

### Cross-component scenario: Resolver + Sorter + Labeler + Builder produce byte-identical output across runs

- **Setup**: identical impact report (multi-create batch with at least one in-batch
  dep, one existing-bead dep, and one spec_node fallback), identical
  `.bead-map.json`, identical spec graph, identical `--git-head`. Run the
  builder twice in two separate processes (so the labeler is not sharing
  in-memory state).
- **Components exercised together (named in setup and assertion text so the
  test binds to all four)**:
  - `Resolver` classifies each dep into one of the three ref shapes.
  - `TopologicalSorter` orders the create ops — proposal epic first, then
    in-batch dep predecessors, with lex-tiebreak among independent ops.
  - `IdempotencyLabeler` reserves the next N record-ids from the mapping
    store counter and pairs them with the sorted op order.
  - `ChangesetBuilder` composes the final changeset, applying canonical
    field order.
- **Assertions**:
  - First and second `changeset.json` outputs are byte-identical (SHA-256
    equal).
  - Each op's `idempotency.label` matches the expected `spex:<id>`.
  - Each op's `deps` carries the expected ref shape.
  - Each op appears in topologically valid order with lex-tiebreak respected
    among independent peers.
- **Rationale**: Determinism is the headline contract for the four-component
  composition. A single byte difference between the two runs would indicate
  Resolver, Sorter, Labeler, or Builder leaked nondeterminism (map iteration,
  unordered set, time-based field).

### Cross-component scenario: dep classification round-trip through Builder

- **Setup**: a multi-create impact report with three deps that must each
  resolve through a different code path:
  - In-batch (`ref:op`): another create op in the same batch targets the
    spec_node.
  - Existing-bead (`ref:bead`): mapping store has an open bead for the
    spec_node.
  - Spec-node fallback (`ref:spec_node`): no in-batch op, no mapping record.
- **Components exercised together**: Resolver classifies, Sorter orders so
  the in-batch ref:op dep predecessor is sequenced before its dependent,
  Builder composes the final ops with all three ref shapes intact.
- **Assertions**:
  - The dependent create's `deps` array contains exactly the three ref
    shapes named above, each with the correct fields populated.
  - The in-batch predecessor op appears earlier in the changeset than the
    dependent op (Sorter respected the in-batch dep).
  - No ref shape is "flattened" or coerced — the Resolver/Sorter/Builder
    chain preserves the three-way distinction end-to-end.

### Cross-component scenario: idempotency label reservation paired with sort order

- **Setup**: mapping store next-record-id counter is 100. Impact report has
  three creates A, B, C with in-batch dep edges A → B → C (so the
  topological order is A, B, C).
- **Components exercised together**: Sorter assigns op_ids in topological
  order, Labeler reserves three monotonic record-ids starting from 100, and
  Builder pairs each create's op_id with its label.
- **Assertions**:
  - Op order in the final changeset is A, B, C.
  - Labels are `spex:100`, `spex:101`, `spex:102` paired one-to-one with
    that op order.
  - The pairing is deterministic across two runs of the same input (the
    second run, against a counter advanced to 103, produces `spex:103`,
    `spex:104`, `spex:105` paired with the same A, B, C order).
- **Rationale**: Sort order and label assignment must compose deterministically.
  A subtle bug — assigning labels before sorting, or using map iteration to
  pair them — would surface as a label/op mismatch on re-run.

### Cross-component scenario: cycle detection surfaces through Builder.Build error

- **Setup**: a constructed impact report with an in-batch dep cycle (A's
  DepSpecNodeIDs includes B; B's DepSpecNodeIDs includes A). This is an
  invalid spec, fabricated for the test.
- **Components exercised together**: Resolver classifies both deps as
  `ref:op`, Sorter detects the cycle when running Kahn's algorithm, Builder
  receives the structured error and propagates it.
- **Assertions**:
  - `Builder.Build()` returns a non-nil error.
  - The error message names both spec_node_ids in the cycle.
  - No partial `changeset.json` is written (Builder either writes the full
    file or no file at all).
- **Rationale**: The Sorter's cycle detection must surface through the
  Builder's public API as a usable error, not as a silently dropped op or a
  panic. This is the failure mode that protects the pipeline from a
  spec-graph bug entering the changeset.

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The
tests in `emit/builder_test.go` compose `builderEnv` (a `fakeStore`
mapping-store double plus a `fakeSpecGraph`), `sampleComponentCreate`
for impact actions, and per-scenario seeded records — mixes of open,
closed, and missing mapping records are seeded directly on
`fakeStore.bySpecNode`. Canonical-output and determinism scenarios
assert against JSON marshalled in-test rather than a golden file.

## Edge cases

- Empty impact report → changeset with only the proposal epic op (if any) or
  an empty op list.
- Impact report with only closes (no creates) → no parent/dep resolution
  needed.
- DepSpecNodeIDs with a spec_node_id that is itself a closed bead AND a
  fresh create in the same batch → the open in-batch op wins (ref:op) over
  the closed bead (skipped).
