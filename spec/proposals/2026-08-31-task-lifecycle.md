# Change Proposal: Task lifecycle completes the event system

## Context

Three proposals moved task↔spec linkage onto the journal: `2026-08-01-task-journal` (pairing by events), `2026-08-11-event-keyed-linkage` (eids as the one linkage key, labels optional insurance), `2026-08-13-plan-module` (open pairings retarget instead of being recreated — "a task is work on a change"). The migration stopped half-way, and the 2026-08-31 post-epic mint of the h4gv drift triage demonstrated every leftover at once:

1. **The closed path still runs the pre-journal lifecycle.** `ActionClassifier` (`8aa1ab5ac102`): a matched modified node whose pairing is not known-open takes obsolete+create — a close op against an already-closed task (a no-op the adapter must converge) plus a successor create carrying a `blocks` dependency on its closed predecessor. The observed mint emitted 6 creates, 6 no-op closes and 6 lineage edges onto closed tasks. `2026-08-13-plan-module` already stated the principle that kills this — "no blocks lineage edge exists between generations… the accumulation lives in the journal" — but only for the open path.
2. **The status input is a foreign, unbounded format.** `--beads` (requirement `d027a902f414`) accepts the raw tracker listing — measured in that run: 288 KB, 380 entries, 19 fields each — of which `BeadReader` (`9f1578d7af6d`) reads two fields. The format is br's, so every br output change is inherited breakage; the file grows with tracker history forever. Changeset (v3) and receipts (v1) are self-owned versioned formats; this is the one input that is not.
3. **Terminology is split down the middle.** The journal speaks tasks (`task_created`, `task_retargeted`, `task_closed`, `task_id`); plan and its neighbours still speak beads: `--beads`, `BeadReader`, `BeadStatus`, `{"ref":"bead","bead_id":…}` in changesets, `bead_id` in receipts, `BeadMapSchema` (`d125b5e775b4`) carrying `schema/bead-map.schema.json`, requirement titles ("Read bead metadata", "Match changed nodes to beads"), and roughly 500 further mentions across the plan, adapters, ingest, map and proposal leaves. No proposal ever declared `bead` retired, so `scripts/lens-lexicon.sh` never swept it.

## Proposed change

### 1. Lifecycle: two decisions on one bounded input

`ActionClassifier` (`8aa1ab5ac102`) and `Resolver` (`e9a3b1b85953`), plan module — re-described:

- A matched added/modified node whose task appears in the task-state artifact as `open` → **retarget** (unchanged). As `in_progress` → **refuse the run** (unchanged).
- A matched added/modified node whose task is **absent from the artifact** → one plain **create**. The obsolete step dies: no close op against the predecessor, no `OldBeadID`, no `blocks` lineage dependency — generation history is the journal's event chain, surfaced by `ContextResolver` (`6b79188dff4c`) brackets. "Absent" *means* "no live task": completion is derived from the journal pairing plus absence, never carried as a status.
- A **removed** node whose task appears in the artifact as `open` → one **close** op (cancelling live work is a real action, not lineage). As `in_progress` → **refuse the run**, the same protection the modified path has — a deliberate tightening over today's silent close. Absent → **cleanup create** (`8987ef169e48`'s gate, now derived from journal-pairing-plus-absence instead of an explicit `closed` status).
- The `closed` status disappears from plan's inputs entirely. The unknown-status branch ("not known-open") dies with it.

**Changeset moves to v4** (`7f275787df34`, `77f5182ac5f7` re-described): op vocabulary is `create`, `close`, `retarget`; refs are `{"ref":"task","task_id":…}` and `{"ref":"op",…}`; `obsolete` leaves the vocabulary. **Receipts move to v2**: `task_id` replaces `bead_id`. `ChangesetBuilder` (`4c1146bb7287`) and ingest's `Reconciler` (`2b5158af774b`) follow; `IdempotencyLabeler` (`6efd7f8ebdb2`) is untouched (eid labels carry no bead wording).

### 2. Task-state artifact: an internal format the adapter produces

A new self-owned versioned document replaces the raw tracker listing:

```json
{"version": 1, "tasks": [{"task_id": "…", "status": "open"}]}
```

- `status` ∈ {`open`, `in_progress`}. **Only in-flight tasks belong in it** — the file is bounded by work in progress: a handful mid-epic, empty after one. An empty list is a legal, explicit "nothing in flight". The artifact is a required input to `spex plan`.
- The format contract ships as `schema/task-state.schema.json` beside the other schema documents; `TaskReader` validates shape, version and status enum on read and starts no process, exactly as `BeadReader` does today.
- **Producing the artifact is the adapter's job** — the component that owns the tracker-format boundary under `2026-04-18-decouple-spex-from-br`'s "adapter script bridges those artifacts to the tracker". `BrReferenceAdapter` (`7f2e76cecab3`, adapters module) is re-described with an export half: derive the artifact from the tracker (for br: from its listing, filtered to open/in_progress, projected to two fields). A br output change then touches one script, never spex.
- `PlanCommand` (`92ae9dab6d6d`): flag surface changes — `--tasks <file>` replaces `--beads`.

### 3. Terminology: tasks everywhere inside the spec corpus

- **Component renames (delete+create, names are identity):** `BeadReader` → `TaskReader` (plan); `BeadMapSchema` → `JournalLineSchema` (schema module), its file `schema/bead-map.schema.json` → `schema/journal-line.schema.json`, with `SchemaLoader` (`ee88263d6555`) and `MappingStore` (`205e67ca4aad`) prose following.
- **Node renames elsewhere:** the map module's bead-named flow leaf (`flow_bead_mapping.md` and its declaring data_flow node) takes the task vocabulary; the plan test_section behind `test_bead_matching.md` likewise. `/spec`'s enumeration over the slim render is authoritative for the full rename list.
- **Requirement retitles** (module-scoped ids re-derive; remove+add pairs): `022dcab6a70d` "Read bead metadata" → "Read task state", `acc550bb0e73` "Match changed nodes to beads" → "Match changed nodes to tasks", `d027a902f414` "Accept bead state via --beads file input" → "Accept task state via --tasks file input", `3ec0a433e476` "Data flow contract bead gating" → "Data flow contract task gating". The project requirement `9120788210c9` "Map spec nodes to beads" is retitled to "Map spec nodes to tasks" **keeping its id byte-for-byte** (project-level ids are carried, never recomputed).
- **Boundary rule, stated in the adapters leaves:** inside the spec corpus the word is task(s), everywhere; the br reference adapter names br's surface only through its command strings (`br create`, `br close`, `br list`) and calls what they manage tasks. The tracker's own name for its objects does not enter the corpus.
- The corpus-wide sweep rides the Retired vocabulary section below; `scripts/lens-lexicon.sh` holds the line afterwards.

### 4. Skills correction — outside this epic, its own task

`.claude/skills/mint`, `drift-fix`, `spec`, `spec-review` and the repo `CLAUDE.md` still teach the retired model: the obsolete+create path, `--beads <br list --json>`, bead wording throughout. These files are not spec nodes and no bead derives from them. **This work must not belong to this proposal's epic: a separate standalone tracker task is created for it**, and it is executed after the epic lands so the skills describe the shipped state, not an intermediate one. After that task closes, no skill carries the old lifecycle, the old flag, or the bead vocabulary.

## Impact expectation

Work is born throughout — no `mode: refresh`, no absorb candidates are predicted here; `/mint`'s assessment decides against the committed diff.

- **plan**: re-describes of `ActionClassifier`, `Resolver`, `ChangesetBuilder`, `PlanCommand`, `NodeMatcher` (`972faea162a6`, prose only); delete+create of `BeadReader`→`TaskReader`; four requirement retitles (remove+add); `flow_plan.md`, `test_classification.md`, `test_changeset_builder.md`, `test_plan_command.md`, `test_bead_matching.md` (renamed) follow. The state-transition table in `arch_action_classifier.md` shrinks to the two-decision form.
- **adapters**: `BrReferenceAdapter` re-described (export half, receipts v2, close-only convergence); `test_idempotency.md` and the reference script contract follow.
- **ingest**: `Reconciler` re-described (receipts v2, `task_id`); `test_reconciliation.md` follows. Journal event shapes are untouched — they already speak tasks.
- **schema**: `BeadMapSchema`→`JournalLineSchema` delete+create; new `task-state.schema.json` contract described in the renamed component's leaf; `SchemaLoader` carry-list prose.
- **map**: prose alignment in `MappingStore`/`ContextResolver` leaves; the bead-named flow node renamed.
- **project.json**: one retitle keeping its id.
- Renames are delete-plus-create: each renamed bead-producing node costs an old-node close and a new create in the mint, every inbound `[[…]]` link repointed, and the removal-time name sweep must clear the old names — that cost is accepted as the price of "no tails". Requirement retitles oblige their implementing components' leaves, which change anyway. Estimated scope: two component delete+create pairs, one data_flow and one test_section rename, four module-requirement retitles, re-describes across roughly ten arch leaves and six test leaves in five modules. This epic itself is minted by the shipped (old-path) plan code — the last run of that path.
- **Out of the epic:** the skills/CLAUDE.md correction (Proposed change §4) — one standalone tracker task, created manually, not derived from any spec node.

## Retired vocabulary

- `--beads`
- `BeadReader`
- `BeadMapSchema`
- `bead-map.schema.json`
- `bead_id`
- `BeadStatus`
- `obsolete`
- `bead`
- `beads`
