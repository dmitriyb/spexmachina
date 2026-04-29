# Change Proposal: Close decouple-from-br contract gaps

## Context

The `2026-04-18-decouple-spex-from-br` proposal (landed in main as commits
`009233d` + `ab7653b` + `559a2ad`) split bead creation and closure out of
the `spex` binary into the `emit → adapter → ingest` pipeline. Pre-decouple,
`apply/bead_creator.go` and `apply/bead_closer.go` (last seen at `4743102^`)
encapsulated four contract pieces that did not survive the move:

1. **Reconciler proposal-epic skip.** Pre-decouple, the proposal epic was
   inserted directly into the mapping store by `createProposalEpic` with no
   spec-graph lookup. Post-decouple, `Reconciler.applyCreate` calls
   `SpecGraph.NodeMetadata(op.SpecNodeID)` for every create op. proposal_epic
   ops carry `spec_node_id = <proposal stem>` (not a 12-char identity hash),
   so the lookup fails and ingest aborts before any `/converge` of any fresh
   proposal can succeed.

2. **Adapter close-op status check.** Pre-decouple, `CloseBeads` called
   `cli.Status(beadID)` before `cli.Close` and skipped the close transition
   when the bead was already closed. Post-decouple, `scripts/apply-br.sh`
   runs `br close` unconditionally; `br close` exits 3 on already-closed
   targets, the adapter records the op as `error`, and top-level receipts
   become `partial`. The `spex:obsolete` and `commit:<HEAD>` labels DO get
   applied (via the existing `br update --add-label` loop), but the partial
   status blocks ingest from saving the snapshot. This breaks any pipeline
   run that closes a bead which a previous lifecycle already closed.

3. **Labeler modify-pair record-id reuse.** Pre-decouple, `CreateBeads`
   branched on `store.GetBySpecNode(SpecNodeID)`: when an existing record
   was found, the new bead got labelled `spex:<existing-rec.ID>` so the
   record's identity persisted across the modify pair. Post-decouple,
   `Labeler.Reserve(N)` returns N sequential labels for N creates regardless
   of action class. Modify-pair creates land at fresh record-ids,
   `Reconciler.applyCreate` never finds an existing record at the new
   label, the create lands as a parallel insert, and the original record
   stays pointing at the closed bead — invariant 3 fails (`modified bead X
   record still points to old bead_id`).

4. **Cleanup-bead production contract.** Pre-decouple, `CreateBeads`
   detected cleanup actions via `isCleanup(a)` (Reason starts with
   `"Code cleanup:"`) and routed to `createCleanupBead`, which produced a
   bead with title `"Code cleanup: <Node>"`, type `task`, parent = epic,
   deps `["blocks:<OldBeadID>"]`, label `spex:cleanup`, and **no mapping
   record**. Post-decouple, `emit/builder.go` doesn't detect cleanup actions
   at all; cleanup creates flow through the ordinary path producing wrong
   title (`<module>: <node>`), wrong type (`feature` from
   `spec_node_kind="component"`), no `spex:cleanup` label, and a mapping
   record that contradicts CLAUDE.md's "no map record for cleanup beads"
   rule.

The four gaps are independent of each other in terms of which layer owns
the fix, but interlinked in that any `/converge` run on `main` of any
fresh proposal will hit gaps 1–3 immediately, and any proposal that
removes a node whose bead is closed will additionally hit gap 4. The
pipeline-cleanup-and-refresh-mode proposal in `proposal/pipeline-cleanup-
and-refresh-mode` is one such case (HashCommand removal); it triggered
all four during a `/converge` attempt.

This is a bootstrap-meta proposal: the work it describes IS what makes
`/converge` produce correctly-shaped beads. The implementation must land
in the same PR as the proposal — there is no upstream pipeline to
materialise these beads through, because the pipeline is what we are
fixing. Beads that `/converge` produces during the fix PR's own pipeline
run are closed-on-creation with implementation-pointer reasons; the work
is the PR itself.

## Proposed change

Four amendments, each scoped to one or two arch leaves.

### A. Reconciler proposal-epic skip (gap 1)

Affected nodes: `ingest/Reconciler` (component `2b5158af774b`,
`spec/ingest/arch_reconciler.md`).

Add a "Proposal-Epic Ops" section after "The Modified-Node Pair":

- Create ops with `spec_node_kind == "proposal_epic"` carry
  `spec_node_id = <proposal stem>` — not an identity hash. The Reconciler
  MUST skip `SpecGraph.NodeMetadata` for these ops.
- Materialise the new record with `bead_type = "epic"` (via `beadTypeFor`),
  `node_type = "proposal"` (matching the on-disk vocabulary used by the
  pre-existing proposal-epic record `spexmachina-0lk`), `spec_node_id =
  <proposal stem>`, `component = <proposal stem>`, `module = ""`,
  `content_file = ""`, `spec_hash = ""`.
- Invariant 4 (orphan check) MUST be exempt for records with `node_type
  == "proposal"`. The check short-circuits on these records because their
  spec_node_id will never resolve through `SpecGraph.HasNode`.

This rule applies to both the fresh-create branch and the modified-pair
update branch (modified proposal_epic ops are rare but the no-spec-graph-
lookup rule still applies).

### B. Adapter close-op status check (gap 2)

Affected nodes: `adapters/BrReferenceAdapter` (component, in
`spec/adapters/arch_br_reference_adapter.md`); `Adapter idempotency tests`
test_section (`spec/adapters/test_idempotency.md`) gains a new scenario.

Amend the "Idempotency" section of `arch_br_reference_adapter.md` to
describe the close-op pre-flight as three branches read from the existing
`br show <bead> --format json` JSON the adapter already calls:

| Pre-state                                    | Action                                                       | Receipt |
|----------------------------------------------|--------------------------------------------------------------|---------|
| `labels` contain `spex:obsolete`             | Skip — already obsoleted in a prior run.                     | `status=skipped`, reason `"already obsoleted"` |
| `status == "closed"` (no `spex:obsolete`)    | Apply labels via `br update --add-label …` only; do NOT call `br close`. | `status=ok` |
| `status == "open"` (no `spex:obsolete`)      | Apply labels via `br update --add-label …`, then `br close --force --reason …`. | `status=ok` |

Pre-decouple's two-phase `LabelObsoletes`/`CloseBeads` split is an
implementation detail that the adapter MAY use internally but is NOT a
contract requirement. emit and ingest see one close op per obsoleted bead
regardless of branch the adapter takes.

Add a new test scenario to `spec/adapters/test_idempotency.md`:

```
### Close: bead is already closed without spex:obsolete
- Changeset close op targeting bead br-abc. br-abc is status=closed (closed
  by an earlier bead-lifecycle run) but its labels do NOT contain spex:obsolete.
- Expected: br update --add-label spex:obsolete --add-label commit:HEAD invoked
  (the labels apply); br close NOT invoked; receipt status=ok.
```

### C. Labeler modify-pair record-id reuse (gap 3)

Affected nodes: `emit/IdempotencyLabeler` (component `6f4b6dd8928f`,
`spec/emit/arch_idempotency_labeler.md`); `emit/ChangesetBuilder`
(component `7f06f7d80e94`, `spec/emit/arch_changeset_builder.md`) — its
internal call site shifts from flat `Reserve(N)` to per-action `LabelFor`.

Amend `arch_idempotency_labeler.md`:

- Replace the flat `Reserve(n)` API description with `LabelFor(action) →
  string`. The Labeler's responsibility is per-action label assignment,
  not flat range reservation.
- Per-action rules:
  - `action.OldBeadID != ""` AND the action is NOT a cleanup (Gap D
    discriminator) → look up `MappingStore.GetByBead(action.OldBeadID)`,
    return `spex:<existing-rec.ID>`. The cursor does NOT advance —
    modify-pair creates do not consume a fresh record-id.
  - cleanup actions → see Gap D for the special-case label.
  - All other creates → return `spex:<cursor>` and advance the cursor.
- Why this matters: `Reconciler.applyCreate` keys its modify-pair detection
  on `wc.byID[recID]`. Reusing the existing record's id is what lets
  Reconciler hit the modify-pair-update branch and rebind the bead_id;
  fresh allocation makes Reconciler insert a parallel record at a new id,
  leaving the original orphaned and breaking invariant 3.

Mention the rule briefly in `arch_changeset_builder.md` in the
Responsibilities section ("Builder calls `Labeler.LabelFor(action)` once
per ordered create instead of consuming a flat slice from `Reserve`.").

Existing test_section `Changeset builder tests` (id `6951b8d84604`) gains
an assertion in the "Obsolete + create lineage" scenario: the create's
`idempotency.label` MUST equal the existing record's id, not a freshly-
reserved sequential value. Seed a record with `bead_id = OldBeadID` in
the fixture so the lookup is exercised.

### D. Cleanup-bead production contract (gap 4)

Affected nodes:

- `emit/ChangesetBuilder` (`7f06f7d80e94`, `arch_changeset_builder.md`) —
  cleanup-action detection and op-shape declaration.
- `emit/IdempotencyLabeler` (`6f4b6dd8928f`, `arch_idempotency_labeler.md`)
  — cleanup label format.
- `adapters/BrReferenceAdapter` (`arch_br_reference_adapter.md`) —
  label-on-create support and `cleanup → task` type mapping.
- `ingest/Reconciler` (`2b5158af774b`, `arch_reconciler.md`) — skip
  mapping-record materialisation for cleanup creates.
- Test sections covering each amended component pick up scenarios for
  cleanup behaviour.

The discriminator stays consistent with the pre-decouple `isCleanup(a)`:
**`action.Reason` starts with `"Code cleanup:"`**. emit detects this and
declares the cleanup explicitly via the op's `spec_node_kind` field.

**emit op shape for cleanup creates:**

| Field             | Value                                                              |
|-------------------|--------------------------------------------------------------------|
| `type`            | `"create"`                                                         |
| `spec_node_kind`  | `"cleanup"` (new value alongside `proposal_epic`, `component`, `data_flow`, `test_section`) |
| `spec_node_id`    | `action.SpecNodeID` (the identity hash of the removed node — for traceability) |
| `idempotency.label` | `"spex:cleanup-<spec_node_id>"` — unique by removed-node identity hash; does NOT consume the Labeler cursor; gives idempotency on re-runs (which pre-decouple lacked) |
| `parent`          | proposal-epic ref (same as other creates)                          |
| `deps`            | `[{ref:bead, bead_id:<action.OldBeadID>, type:"blocks"}]`           |
| `priority`        | `3` (`emit.FallbackPriority`) — pre-decouple set `-1` as a "don't pass `--priority`" sentinel, which doesn't translate to the changeset's plain-int schema; using FallbackPriority keeps cleanup beads consistent with other unresolved-chain creates |
| `title`           | `action.Reason` verbatim (i.e. `"Code cleanup: <module>/<node>"`)   |
| `labels`          | `["spex:cleanup"]` — the discriminator label (`Op.Labels` field already exists, currently used only on close ops) |
| `body`            | empty                                                              |

**emit Labeler:** `LabelFor(action)` returns `spex:cleanup-<action.SpecNodeID>`
when the action's Reason starts `"Code cleanup:"`. No cursor advance.

**Adapter:**

- `spec_kind_to_bead_type` (`scripts/apply-br.sh`) maps `"cleanup" → "task"`.
- `process_create` MUST pass each entry of the op's `Labels` array as
  `--add-label` to `br create`. Currently the adapter passes labels only
  on `br close` and `br update`; it ignores any labels on create ops.
- Idempotency check before create looks for the cleanup-specific label.
  Existing flow already queries `br list --json --label
  <idempotency.label>` — for cleanup ops this resolves to
  `spex:cleanup-<spec_node_id>` and finds existing cleanup beads on
  re-runs.

**Reconciler:**

- `applyCreate` MUST branch on `op.SpecNodeKind == "cleanup"` BEFORE the
  recID parse and spec-graph lookup. Skip the entire mapping-record
  materialisation: no `wc.put`, no `wc.advanceCounter`, no
  `lookupMetadata`. Count the op (`sum.OkCreates++`) and return nil.
- Invariant 1 ("every ok create has a record") MUST be amended: cleanup
  creates are exempt by design (no record by construction).
- Per-op transition table in `arch_reconciler.md` gets a new row:
  `create / spec_node_kind=cleanup / ok / — → No mapping change (cleanup
  beads have no map record per pre-decouple convention; CLAUDE.md "no map
  record" rule)`.

**Test sections:**

- `Changeset builder tests` (id `6951b8d84604`) — new scenario: cleanup
  Action with Reason `"Code cleanup: m/X"` produces an op with
  `spec_node_kind=cleanup`, title=Reason verbatim, labels=`["spex:cleanup"]`,
  idempotency.label=`spex:cleanup-<spec_node_id>`, deps=`[{ref:bead,
  bead_id:<OldBeadID>, type:"blocks"}]`.
- `Reconciliation tests` (id `425f743382a7`) — new scenario: ingest of a
  cleanup create produces no mapping record; counter does not advance;
  invariant 1 does not fire.
- `Adapter idempotency tests` (id in `spec/adapters/module.json`) — new
  scenario: changeset cleanup-create with labels `["spex:cleanup"]`
  produces a `br create --add-label spex:cleanup --add-label
  spex:cleanup-<n> --type task` invocation; receipt `ok`; bead in tracker
  has `spex:cleanup` label.

## Impact expectation

This proposal generates beads through the standard pipeline. Listing by
spec module:

**ingest module:**

- `Reconciler` arch leaf modified (`spec/ingest/arch_reconciler.md`) —
  proposal_epic skip + cleanup skip rules added. Existing bead
  `spexmachina-0lk.18` (closed) → obsolete (label-only via gap 2 fix) +
  new replacement bead. Reconciler component bead is the modify-pair
  carrier for both gap 1 and gap 4 work.
- `Reconciliation tests` test_section content modified
  (`test_reconciliation.md` adds a cleanup-skip scenario). describes==1
  (single-component bead-bundled) so no bead lifecycle on this leaf.

**emit module:**

- `IdempotencyLabeler` arch leaf modified
  (`spec/emit/arch_idempotency_labeler.md`) — LabelFor rule for
  modify-pair AND cleanup. Existing bead `spexmachina-0lk.10` (closed) →
  obsolete + new replacement bead.
- `ChangesetBuilder` arch leaf modified
  (`spec/emit/arch_changeset_builder.md`) — cleanup-action detection,
  per-action LabelFor call. Existing bead `spexmachina-0lk.7` (closed) →
  obsolete + new replacement bead.
- `Changeset builder tests` test_section content modified — gains the
  modify-pair label-reuse assertion plus the cleanup scenario. describes
  is currently 1 (Builder only); this proposal does NOT change describes
  count, so still bundled with Builder's bead. No separate bead lifecycle.

**adapters module:**

- `BrReferenceAdapter` arch leaf modified
  (`spec/adapters/arch_br_reference_adapter.md`) — close-op status check,
  label-on-create support, `cleanup → task` mapping. Existing bead
  `spexmachina-0lk.2` (closed) → obsolete + new replacement bead.
- `Adapter idempotency tests` test_section content modified
  (`test_idempotency.md` adds the closed-without-obsolete scenario plus
  the cleanup-create scenario). describes-state TBD when /spec runs;
  if it stays bead-non-producing, bundled.

**Total estimated bead delta:** 1 proposal_epic + 4 modify-pair
(component) creates + 4 obsolete-with-labels closes on closed targets =
9 ops in the changeset. The adapter's gap-2 fix makes all 4 closes
record `ok`; the Labeler's gap-3 fix makes all 4 modify-pair creates
reuse their existing record-ids; ingest reconciles cleanly; snapshot
saves. The proposal_epic create exercises the gap-1 fix.

After this proposal lands to main, `/converge` works correctly for any
fresh proposal. The `proposal/pipeline-cleanup-and-refresh-mode` branch
can rebase on new main and `/converge` cleanly — its HashCommand cleanup
will produce a correctly-shaped cleanup bead (title `Code cleanup:
merkle/HashCommand`, type `task`, label `spex:cleanup`, no map record).

Beads produced by this proposal's own `/converge` run are closed
immediately with reason `"Implemented inline in bootstrap PR. Pipeline-
fix meta-PR; the work IS the PR. See PR#NNN."` This is a documented
departure from CLAUDE.md's `/review`-closes-beads rule, justified by the
bootstrap nature: there is no working pipeline that could otherwise
materialise these beads for separate `/implement` PRs.
