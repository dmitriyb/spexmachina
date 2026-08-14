# Change Proposal: One `plan` module, tasks keyed to changes

## Context

`spex impact` and `spex emit` are two pipeline stages with one consumer between them: the `ImpactReport` JSON that `impact` writes is read by `emit` and by nothing else — internal plumbing wearing the costume of a stable contract. Post-decouple and post-task-journal the two modules are one computation artificially halved: *(diff + tracker view + spec graph + git_head + proposal) → changeset*. The cost is concrete: a six-stage pipeline where five suffice, an intermediate schema maintained for an audience of one, duplicated journal plumbing on both sides, and a CLI seam (`spex impact --json | spex emit`) whose flags only ever run together. This half executes idea B — the last live remnant — of the retired draft `spec/proposals/obsolete/post-decouple-architectural-cleanup.md`, carried forward from the superseded `2026-08-02-merge-impact-emit` draft.

The second half fixes what the task-journal migration exposed but did not finish: **a task is work on a change (state A→A*), yet the classifier still keys tasks to components.** Today any modification of a node with a live pairing obsoletes the old bead and mints a successor — correct before the journal existed (there was nowhere to accumulate intermediate changes), pure churn now. An epic whose spec is corrected mid-flight recreates open beads whose content did not materially change, spending implementer/reviewer/merger resources to discover the task "can be closed automatically". The journal already models the truth — receipts pair tasks with *events*, not with node state — so the classifier can stop recreating and start retargeting. A second churn source is the cosmetic correction: a prose-only spec fix whose work already shipped still mints an obsolete+create pair. `ingest --mode refresh` absorbs such drift only as an all-or-nothing mode; there is no per-node marking inside a normal run.

Both halves touch the same leaves (ActionClassifier, Resolver, ChangesetBuilder, both flows, the builder tests), which is why they travel as one proposal: sequential epics would have the second immediately modify leaves the first just created — under today's rules, recreating their fresh beads, the exact waste being eliminated.

## Proposed change

### 1. The `plan` module replaces `impact` and `emit`

`spec/project.json`'s module list drops `impact` (`725a083f66a1`) and `emit` (`742ff18345ed`) and gains `plan` (path `plan/`): one deterministic pass from diff to changeset. Go packages `impact/` and `emit/` merge into `plan/`; `cmd/spex/impact.go` and `cmd/spex/emit.go` are replaced by `cmd/spex/plan.go`.

**Components (8), all declared in `spec/plan/module.json` with new ids:** `BeadReader`, `NodeMatcher`, `ActionClassifier`, `Resolver`, `TopologicalSorter`, `IdempotencyLabeler`, `ChangesetBuilder`, `PlanCommand`. Three have no successor: `ReportGenerator` `60d4747021ec` (its report is machine JSON with a single consumer; a `--explain` preview is later scope), and the two command components `ImpactCommand` `5d6813710879` / `EmitCommand` `cbe835d38c3e`, superseded by `PlanCommand`. `PlanCommand`'s flag surface is the union minus the seam: `--diff` (default stdin), `--beads`, `--git-head`, `--proposal`, `--absorb` (new, below), `--out` (default stdout); impact's `--json` dies with the report and the deprecated no-op `--bead-cli` is retired. The plan leaves are authored against the post-drift-fix spec (main `615f2d2`): the registration read is a distinct narrow input beside the fold, the fold is asked first for parent resolution, and the cleanup referent has both the prior-batch fold-read branch and the same-batch branch.

**Module wiring** (`spec/project.json` `modules[]`): `plan.requires_module` is the union minus each other — merkle `085966b6bfa1` and map `fb20a21b62f1`; ingest's entry re-points from `742ff18345ed` to plan's new id. This wiring is load-bearing: the classifier derives `DepSpecNodeIDs` from the transitive `requires_module` closure.

**Apis:** `spex impact` `62b47fdb7f2d` and `spex emit` `7cccc4a96101` removed; `spex plan` added, `provided_by` `PlanCommand`. Hard cutover, no wrappers. The pipeline reads `validate → diff → plan → adapter → ingest` everywhere it is written down.

**Requirements:** the nineteen module requirements re-derive under `plan` from the same unchanged project requirements — eighteen one-to-one with re-worded descriptions, each keeping its `preq_id`. One has no successor: `cc987600950e` *Output impact report* (the artifact is gone); its parent `0a0f49f9be9b` stays covered by five surviving successors. No project requirement is edited. Two module requirements are added under `plan` (§2, §3).

**Data flows and tests:** `Impact analysis flow` `855e53793fdf` and `Emit flow` `dbe8d1e707d5` merge into one `Plan flow`; the five test sections re-home under `plan`, scenarios joined where they tested the seam itself.

### 2. Tasks key to changes: retarget instead of recreate

New `plan` module requirement **Retarget open tasks instead of recreating** (`preq_id` `0a0f49f9be9b`), implemented by `ActionClassifier` and `Resolver`. The state transition table changes in one place — the `modified`/`added`-with-differing-hashes cells split on the live pairing's tracker status (already available from `BeadReader`'s `--beads` input):

- **Pairing's task is `open` (unclaimed):** the action is **retarget** — a third action word beside `create`/`obsolete`. No bead is closed, none created; the task's target moves from A1 to A2. The action carries the node, its new content hash, and freshly recomputed `DepSpecNodeIDs`.
- **Pairing's task is `in_progress`:** `spex plan` refuses, exit 2, naming every claimed task whose node changed — a claimed task's target never moves under it. The operator waits for the task to settle or splits the change.
- **Pairing's task is `closed`:** unchanged — obsolete + create successor (lineage as today).
- The "already tracked" cell (equal hashes → no action) is unchanged and checked first.

**Changeset v3** carries the new op: `type: "retarget"`, `target` `{ref:bead}`, `spec_node_id`, `labels: ["spex:<eid>"]` of this run's `modified` event (derived from `(git_head, op_id)` like every node-bearing create), and `deps` — the recomputed refs, applied add-only. The adapter (contract + `scripts/apply-br.sh`) translates retarget to `br update`: add the new event label, add missing deps. Update operations are naturally idempotent, so retarget needs no probe. The version bump is honest vocabulary change: v2 consumers must not silently drop ops they do not know.

**The journal records the shift.** New receipt event `task_retargeted` (`for`: the new `modified` event's eid, `task_id`: the existing task) joins the `taskReceipt` enum in `schema/bead-map.schema.json`. Ingest module requirement (new, `preq_id` `9120788210c9`): `Reconciler` constructs the pair — the `modified` change event plus its `task_retargeted` receipt — from a retarget op's receipt. Map module requirement (new, `preq_id` `9120788210c9`): `MappingStore`'s fold treats `task_retargeted` as task-bearing (latest-wins moves the pairing's sourcing event forward), and `ContextResolver` widens the context bracket for a retargeted task — `before_head` from the sourcing event of the task's original `task_created`, `after_head` from its latest retargeted event, so the implementer sees the full A→A2 diff, not the last increment. Because open tasks are no longer recreated, the `blocks` lineage edge between generations disappears for this path — history lives in the journal.

### 3. Cosmetic absorption inside a normal run

New `plan` module requirement **Absorb marked cosmetic modifications** (`preq_id` `81f8102ae1b5`), implemented by `PlanCommand` and `ChangesetBuilder`. Whether a prose edit is cosmetic is not deterministically decidable, so the decision is an authored input, never an inference: `--absorb <file>` names a git-committed JSON list of `{node, reason}` entries. Rules, deterministic in `spex`:

- A marked node must appear in the diff as `modified` — marking an `added`/`removed` node, or one absent from the diff, is exit 2.
- A marked node produces no op. Its change event still reaches the journal: the changeset gains a top-level `absorbed` array (node, before/after hashes, reason); the adapter ignores it; ingest appends the `modified` events — eids derived from `(node, before, after)` exactly as refresh-born events are — closed by the existing `refresh` receipt naming them. `ingest --mode refresh` remains the all-or-nothing peer; this is its per-node form riding a normal run.
- A marked node that also holds an open pairing is absorbed without a retarget — cosmetic means the task owes nothing.

The judgment discipline lives in the authoring loop, not in `spex`: the skills that move the baseline (`/spec`, `/drift-fix`) must declare per node what changed and why it is cosmetic, with contract-bearing sections (Interface, Responsibilities, tables, module.json) categorically ineligible; the presumption is non-cosmetic, silence never skips; the PR reviewer validates the declaration itself, as with blocking drift reports. The absorb file in the PR diff and the reasons in the journal are what make the bypass auditable.

### What the merge deletes outright

The `ImpactReport` JSON schema and its version checks; the report renderer; the seam plumbing; one of the two journal folds. Receipts and the snapshot contract are untouched. The changeset moves to v3 as specified — on inputs with no retarget-eligible modifications and no absorb file, `plan`'s output is byte-identical to what the v2 event-keyed pipeline emits, modulo the version field: same ops, same order, same labels.

### Sweep obligations, stated in advance

Removing the two apis and three unsucceeded components triggers the removal-time name sweep; re-homed component names are tolerated by `suppressed_by_live_name`. Grep at `615f2d2` shows 28 files under `spec/` (outside `spec/proposals/`) carrying `spex impact` or `spex emit` across merkle, map, ingest, cli, proposal and the two dissolved directories — the exact tokenized hit list is re-derived at /spec time; drift-fix #268 edited several of the previously inventoried leaves, so counts from the superseded draft are stale. Silently-stale surfaces the sweep cannot flag, to edit by hand: `spec/cli/arch_root_command.md` (constructor and api counts), `spec/cli/test_root_command.md`, `spec/cli/arch_hash_id_command.md` + `test_hash_id_command.md` (worked examples keyed on the `impact` module), `spec/map/arch_map_command.md` (worked example pointing at `spec/impact/` paths), `spec/merkle/arch_diff_engine.md` (pipeline arrow chain), plus README, CLAUDE.md, `docs/`, `skills/spec/SKILL.md`, and code comments (`merkle/doc.go`, `ingest/doc.go`, `cmd/spex/diff.go`, `cmd/spex/ingest.go` — which imports the emit package for `Changeset`/`ChangesetVersion`, re-homed into `plan`).

## Impact expectation

**A structural epic with a behavioral rider.** Removed bead-producing nodes: 10 components + 2 data flows + 3 of the 5 test sections — 15 obsolete actions, each with a cleanup bead (mostly trivially closable: the code moves rather than dies). Added: ~8 components, 1 data flow, and the multi-`describes` test sections under `plan` — ~11 creates plus the proposal epic. The retarget/absorb rider adds: one new requirement + component-leaf and test-leaf modifications in **ingest** (`Reconciler`, `IngestCommand`, both test sections), **map** (`MappingStore`, `ContextResolver`, their test sections and `flow_bead_mapping.md`), **adapters** (contract leaf + `BrReferenceAdapter`), and **schema** (journal-line schema leaf) — modify pairs on the order of 8–12 beads. Sweep collateral on merkle/cli/proposal leaves: another 15–20 modify pairs. All told **75–95 operations**, with `/spec`'s enumeration authoritative. Completeness rules are the driver: the new module requirements oblige changed leaves on every implementing component, and the `module.json` meta moves in plan, ingest, map, adapters and schema are suppressed by requirement changes in each.

**Module meta:** two module nodes removed, one added — changes refresh cannot absorb, so this epic runs the full gate end to end. **The journal records the dissolution**: every removed impact/emit node gets a `removed` event closing its lineage, every `plan` node an `added` event, cleanup labels carry their removal events' eids.

**Prerequisites:** `2026-08-11-event-keyed-linkage` fully landed and its epic closed — in particular IdempotencyLabeler on eid-keyed labels (hdkq.18), without which any structural mint mis-probes: the adapter's status-unfiltered exact-match probe is only safe once labels are unique per event. This proposal must not start while that epic's beads are open. Out of scope: the merkle hashing work; CLI compatibility shims; retarget dep *removal* (add-only; stale deps are closed by their own lifecycle); a proposal-lifecycle status surface (`registered → specified → in work → closed` — a later proposal); any `--explain` human-readable preview.

## Retired vocabulary

- `spex impact`
- `spex emit`
- `ImpactReport`
- `impact report`
- `ImpactCommand`
- `EmitCommand`
- `ReportGenerator`
- `--bead-cli`
- `changeset.json v2`
