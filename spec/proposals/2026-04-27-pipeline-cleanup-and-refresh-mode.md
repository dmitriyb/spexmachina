# Change Proposal: Pipeline cleanup and refresh-mode pathway

## Context

Three coupled problems surfaced during execution of `2026-04-18-decouple-spex-from-br.md` and remain unresolved on `main`. They share the same underlying gap: the spec does not describe a pathway for refreshing snapshot and bead-map state without triggering bead lifecycle, and parts of the existing pipeline have either drifted from working behavior or describe behavior that doesn't actually work.

**1. `spex hash` is documented as the bootstrap mechanism but produces a dead state.** The merkle module spec (`spec/merkle/arch_hash_command.md`, `spec/merkle/flow_hash_computation.md`) and `README.md` describe the bootstrap flow as `validate → hash → diff → impact → emit → adapter → ingest`. Following that flow on a fresh project: `spex hash` writes `spec/.snapshot.json` matching the current spec exactly, then `spex diff` finds zero changes (snapshot equals current state), so impact produces nothing, emit produces nothing, no beads are ever created, and the project is stuck. The implementation accidentally works around this because `cmd/spex/diff.go:40-48` treats a missing snapshot as a nil tree (so the first `diff` reports "everything added") — but this only works if the user does NOT run `spex hash` first. The spec's prescribed bootstrap is broken, the working bootstrap is undocumented, and `spex hash` standalone in steady-state breaks the snapshot+bead-map atomicity invariant (it writes the snapshot without touching bead-map records' `spec_hash`, leaving them stale relative to current content).

**2. The emit `test_resolver_and_sorter` test_section (id `280a5495eef8`, bead spexmachina-0lk.22) was authored as per-method unit tests despite a structural shape claiming integration coverage.** The test_section's `describes` array contains three components — Resolver (`f7775ac5f1f3`), TopologicalSorter (`7249fd093b8a`), IdempotencyLabeler (`6f4b6dd8928f`) — which per the impact module's gating rule (`spec/impact/impl_action_classification.md` lines 13–20: `describes >= 2` produces a task bead) declares this is a cross-component integration scenario. The actual scenario content in `spec/emit/test_resolver_and_sorter.md` is a list of per-method unit tests (one component, one method, one assertion per scenario) that were satisfied during the three component implementation beads' PRs (`emit/{resolver,sorter,labeler}_test.go`). The bead spexmachina-0lk.22 now represents work that does not need to be done. Structurally, these three components have no public surface independent of `ChangesetBuilder` (`spec/emit/module.json` shows the builder `uses` all three; nothing else does), so any genuine cross-component integration coverage IS Builder coverage — there is no separate integration surface for these three to test.

**3. The ingest module has no documented pathway for absorbing drift without bead churn.** When a spec content leaf is edited outside the normal pipeline (e.g., `/review` finds drift between code and spec and corrects the spec to match shipped code), the snapshot drifts and any affected bead-map record's `spec_hash` field becomes stale. The current pipeline always treats `modified` content as `obsolete + create` per the state transition table (`spec/impact/impl_action_classification.md` lines 26–37). For drift fixes where the work scope hasn't actually changed (only the prose was corrected), this produces a new bead for already-completed work which must then be closed immediately — bureaucratic churn with no audit-trail benefit beyond what a proposal document already provides. Ingest's `spec/ingest/test_consistency_invariants.md` invariant 6 says "snapshot saved iff complete," and the implementation only updates `spec_hash` for records receiving receipts (`cmd/spex/ingest.go:252`), so there is no in-pipeline way to refresh hashes without a bead receipt — and no out-of-pipeline way that respects the invariants.

## Proposed change

Three coordinated module changes. The proposal is `mode: normal` (touches bead-producing leaves and adds new components; refresh mode does not yet exist to consume).

### A. merkle — drop `spex hash`, document the actual bootstrap

Remove the `HashCommand` component (id `ef841f48d313`, `arch_hash_command.md`) from `spec/merkle/module.json`. Remove its associated impl_section `Hash command implementation` (id `15226d6b45d0`, `impl_hash_command.md`). The HashCommand entry in the `Merkle command tests` test_section (id `49a61e0d5737`, `test_merkle_commands.md`) is updated to drop HashCommand from `describes` (leaving DiffCommand id `c8b958ec310d` as the sole described component); the test_section content `test_merkle_commands.md` has its `spex hash` scenarios (S1, S2, S3, S4, E1, E3, E5, S5 in `test_snapshots.md`'s context) removed or rewritten as `spex diff` scenarios where they exercise the same underlying TreeBuilder/SnapshotStore flows.

Keep `Hasher` (id `325f48728e04`), `TreeBuilder` (id `dfe1467b7a4b`), `SnapshotStore` (id `b2fcd9457a28`), `DiffEngine` (id `cb262b280963`), `ImpactClassifier` (id `f1a672216ce9`), `DiffCommand` (id `c8b958ec310d`), `CompletenessChecker` (id `de3309dfbd3c`). These are still used by `spex diff` and by ingest's snapshot save path.

Rewrite `spec/merkle/flow_hash_computation.md` (data_flow id `35f39388bc76`) to describe the actual bootstrap flow: on a fresh project with no existing snapshot, `spex diff` treats the missing snapshot as the empty tree, so the first `validate → diff → impact → emit → adapter → ingest` cycle produces "everything added" bead actions, and `spex ingest` writes the first snapshot atomically with the first bead-map records. No separate hash step is required, and running `spex hash` standalone is unnecessary in both bootstrap and steady-state operation.

The data_flow's `uses` field stays the same (`[Hasher, TreeBuilder, SnapshotStore]`) because those components are still part of the hash-computation flow — they just compose differently (under `spex diff`'s call site, not under a standalone `spex hash` command).

Also update `README.md` pipeline diagram (line 16) from `spec change → spex validate → spex hash → spex diff → spex impact → spex apply` to `spec change → spex validate → spex diff → spex impact → spex emit → adapter → spex ingest` (matches `spec/test_end_to_end_pipeline.md` and the actual modern pipeline).

### B. emit — clean up `test_resolver_and_sorter`, expand `test_changeset_builder`

Remove the `test_resolver_and_sorter` test_section entry (id `280a5495eef8`) from `spec/emit/module.json`. Delete `spec/emit/test_resolver_and_sorter.md`.

Fold component ids `f7775ac5f1f3` (Resolver), `7249fd093b8a` (TopologicalSorter), `6f4b6dd8928f` (IdempotencyLabeler) into the existing `test_changeset_builder` test_section's `describes` array (id `6951b8d84604`); the new `describes` is `[ChangesetBuilder, Resolver, TopologicalSorter, IdempotencyLabeler]`.

Expand `spec/emit/test_changeset_builder.md` content with explicit cross-component integration scenarios that exercise the four components together through `Builder.Build()`'s public API. New scenarios at minimum:

- **Resolver+Sorter+Labeler+Builder produce byte-identical changeset across runs**: same impact report + spec graph + git head input across two consecutive `Build` invocations produces byte-identical `changeset.json` output, naming all four components in the scenario's setup and assertion text.
- **Dep classification round-trip through Builder**: a multi-create batch where Resolver classifies one dep as `ref:op` (in-batch), one as `ref:bead` (existing), one as `ref:spec_node` (fallback); Sorter respects the in-batch dependency by ordering correctly; Builder composes the final changeset with the three-shape refs intact.
- **Idempotency label reservation paired with sort order**: Labeler reserves N labels starting from the mapping store counter; Sorter assigns op_ids in topological order; Builder pairs the two so each create op carries `idempotency.label = spex:<reserved-id>` and the assignment is deterministic across runs.
- **Cycle detection surfaces through Builder.Build error**: a constructed in-batch cycle (Sorter detects it) propagates as an error from `Build()` with no partial changeset written.

Existing builder-only scenarios in `test_changeset_builder.md` remain. The structural rule "test_section with `describes>=2` describes cross-component integration" now matches the content.

### C. ingest — add refresh-mode pathway

Add a new component to `spec/ingest/module.json`: **`RefreshHandler`** (recommended) — separates refresh-mode logic from the existing `Reconciler` for testability and clear bead boundaries. (Alternative: extend `Reconciler` with a refresh mode and a flag; the proposal author should choose the recommended option unless implementer judgment during /spec finds a strong reason otherwise.)

The new component's responsibilities:

- Accept an empty changeset (no create/close/label/tag ops) plus an empty receipts file as input.
- Walk the current spec graph and re-hash every content leaf via `merkle.HashFile`.
- For each existing bead-map record, compare the recorded `spec_hash` to the current content hash. If different, update `spec_hash` to the current value. Do NOT modify any other field.
- Refuse if the diff (computed against the snapshot before refresh) contains any `added` or `removed` entries — refresh mode is for `modified` content only; structural changes require normal pipeline.
- Refuse if any bead-map record has no corresponding spec node in the current spec — that indicates structural drift the user must resolve via normal pipeline.
- After successful refresh, save the snapshot atomically (same path as `SnapshotSaver.Save` for `complete` status).

Add a new requirement to `spec/ingest/module.json`:

- **Title**: "Refresh mode for impl_only drift"
- **preq_id**: `6faeb216618a` (Deterministic — same spec state + snapshot = same refresh result)
- **Description**: Captures the behavior above. Refresh mode is opt-in via proposal frontmatter `mode: refresh`; pipeline routing reads the frontmatter and routes the run through ingest's RefreshHandler instead of the normal Reconciler path. The same atomicity invariant (snapshot+bead-map move together) applies.

Add a new arch leaf `arch_refresh.md` describing the RefreshHandler component contract. Add a new impl leaf `impl_refresh.md` describing the implementation (walk spec graph, compare hashes, update records, save snapshot atomically, refuse on adds/removes).

Add a new test_section `Refresh mode tests` (`describes: [RefreshHandler]`, single component, bundled to RefreshHandler's feature bead per the impact gating rule). Content `test_refresh.md` covers: refresh-only diff is accepted and updates `spec_hash` for changed records; diff with `added` entry is refused with structured error; diff with `removed` entry is refused with structured error; diff with no changes is a no-op (returns success without writing); idempotency on re-run (second invocation of refresh on the same state is a no-op); interaction with `mode: normal` runs (normal runs continue to update `spec_hash` only for records receiving receipts, refresh runs update all stale records).

Update `spec/ingest/flow_ingest.md` (data_flow id `6fd1f0cbb76c`) to mention the alternate path: `mode: refresh` proposals route through RefreshHandler; `mode: normal` (default) routes through the existing Reconciler+SnapshotSaver path. Update the data_flow's `uses` array to include `RefreshHandler`.

Update the IngestCommand component (id `db90eb607bcb`) to wire RefreshHandler when the run is in refresh mode. Add `RefreshHandler` to its `uses` array.

The /spec skill's recognition of `mode: refresh` frontmatter and the pipeline routing logic that consumes it are NOT in this proposal — they ship in a follow-up skills PR after this proposal's spec changes land.

## Impact expectation

This proposal generates several beads through the standard pipeline. Listing by spec module:

**merkle module**:

- HashCommand component removed → existing component bead is obsoleted; cleanup-create generated (real cleanup work: delete `cmd/spex/hash.go`, `cmd/spex/hash_test.go`, the Cobra registration in `cmd/spex/main.go`, related references in `README.md` if any beyond the pipeline diagram).
- `Hash command implementation` impl_section removed → no bead lifecycle (impl_sections don't produce beads per the gating rule).
- `Merkle command tests` test_section modified (drops HashCommand from `describes`, content rewrites hash scenarios as diff scenarios). Currently `describes >= 2` (HashCommand + DiffCommand); after change `describes == 1` (DiffCommand only). This shifts the test_section from bead-producing to bundled-with-DiffCommand-bead. Existing test_section bead obsoleted; no new bead generated (coupled to DiffCommand's bead going forward).
- `flow_hash_computation` data_flow content modified → existing data_flow bead obsoleted + create new replacement bead (real work: rewrite the markdown to describe the bootstrap-via-diff flow).
- README.md pipeline diagram update is implementation work for the cleanup-create bead generated by HashCommand removal (not a separate spec node).

**emit module**:

- `test_resolver_and_sorter` test_section removed (currently describes 3 components, bead-producing). Existing bead spexmachina-0lk.22 (currently in_progress per the conversation history; depending on actual current status when /spec runs, the state transition is either obsolete-only for open/in_progress or obsolete + cleanup-create for closed). The cleanup bead, if generated, has no real cleanup work — its closure reason will be "no code to delete; tests already exist as `emit/{resolver,sorter,labeler}_test.go` per the component implementation beads' PRs; this test_section was a structural misuse (per-method unit tests in describes-3 shape)."
- `test_changeset_builder` test_section modified (expanded `describes`, expanded content). Existing bead obsoleted + create new replacement bead carrying the broader scope. The replacement bead's implementation work is to ensure the new cross-component scenarios in the expanded markdown have corresponding Go test functions in `emit/builder_test.go` (or to verify existing tests already cover them and no new code is needed; either outcome is acceptable as long as the bead's closure documents which scenarios map to which tests).

**ingest module**:

- New `RefreshHandler` component → fresh create bead (real implementation work: write `ingest/refresh.go`, refactor common helpers from Reconciler/SnapshotSaver if needed).
- New `Refresh mode for impl_only drift` requirement → no bead (requirements don't produce beads).
- New `Refresh mode tests` test_section (single-describes, bundled to RefreshHandler's feature bead) → no separate bead.
- `flow_ingest` data_flow content modified (mentions alternate path) → existing data_flow bead obsoleted + create new replacement.
- IngestCommand component modified (wires RefreshHandler) → existing component bead obsoleted + create new replacement bead.

**Total estimated bead delta**: ~6–8 new beads (mix of cleanup, modified-replacement, and fresh creates). All cleanup beads close with explanatory reason; modified-replacement beads are real work proportional to the scope of the change; fresh creates for RefreshHandler are the bulk of the implementation effort.

After this proposal's pipeline run completes, snapshot, bead-map, and tracker are consistent. The drift on `main` for `impact/test_classification_reporting.md`, `proposal/impl_proposal_commands.md`, and `proposal/test_proposal_commands.md` (impl_only changes from earlier PR follow-ups) will ALSO be absorbed by this run because `SnapshotSaver` rebuilds the snapshot from the current spec state on every complete ingest, regardless of which leaves received receipts. Those impl_only leaves are non-bead-producing, so their drift was always a snapshot-only issue; this proposal's pipeline run clears them as a side effect.

Once this proposal's beads are implemented and shipped, the refresh-mode capability is available for future drift corrections, allowing `mode: refresh` proposals to skip bead lifecycle. A follow-up skills PR will add /spec recognition of the frontmatter and update /spec-review and /spec-drift to declare `mode: refresh` when their findings are bead-non-producing-only.
