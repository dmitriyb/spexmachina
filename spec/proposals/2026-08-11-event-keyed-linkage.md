# Change Proposal: Event-keyed linkage — labels, pairings, and briefings key on eids

## Context

The task journal moved task↔change pairing onto events: `task_created` references an event id, and every event carries `before`/`after` content hashes plus `git_head`. But three node-shaped legacies survived that migration, each a special case the journal makes unnecessary:

1. **Idempotency labels are node-keyed.** `spex:<spec_node_id>` collides across a node's lifetime by construction, so the contract compensates twice: the adapter matches labels against *open* beads only (`b8d894dff9b5` — closed beads with the same label must not count), and cleanup creates need the `spex:cleanup-` prefix to dodge the closed feature bead of the same node. A third shape (`spex:<proposal-slug>`) exists because registration produces no event at all. Three branches in `IdempotencyLabeler` `6f4b6dd8928f` where one rule should do.
2. **The epic pairs through a side channel.** An epic's `task_created` carries a `proposal` field instead of `for` — a special branch in the fold (`mapping/mapping_store.go`) and a disjunction in ingest's orphan invariant ("references an existing event **or proposal**", `ee28b5d190ae`).
3. **Briefings serve the snapshot, not the change.** The journal's fourth motivation — "its briefing becomes the exact spec delta + the current leaf" — landed only for removed nodes: `ContextResolver` `6b79188dff4c` exposes `before_head`/`after_head` solely on the removed path. A live task's `spex map context` answer has no event refs, so an implementer still reconstructs the delta by hand — the inference step the journal was built to delete.

The enabling fact is already in the contract: event ids are deterministic, derived from `(git_head, op_id)` (`ingest/reconciler.go`), so emit can know every eid at changeset-build time. The journal is the sole source of linkage truth; this proposal makes the three remaining surfaces say so.

## Proposed change

### Registration writes the journal

`Registrar` `24180f55c0b4` appends a `registered` event on successful registration — `{"event":"registered","eid":"<git_head>:<slug>","proposal":"<slug>","git_head":"<head>"}` — through `MappingStore` `205e67ca4aad`, the journal's one writer-owner. Requirement `2b62ad5e8ef2` *Register proposal* is re-described; the proposal module's `requires_module` gains map `fb20a21b62f1` — a deliberate wiring change: the module that starts a lifecycle records its start. The journal-line schema (`BeadMapSchema` `d125b5e775b4`, requirement `f7ef8bef0ba1`) gains the `registered` event type.

### One label rule

`IdempotencyLabeler` `6f4b6dd8928f` (requirement `0d468e176aaf`): every create op's `idempotency.label` is `spex:<eid>` of the journal event its `task_created` will reference. Node-bearing creates key the change event ingest will mint from `(git_head, op_id)`; cleanup creates key the removal event their same-batch close implies; the epic keys the `registered` event read from the fold. The `cleanup-` prefix, the slug shape, and the collision rationale die — a label is unique per *change*, which is what a task is. `Resolver` `f7775ac5f1f3` (requirement `79c821e01654`) resolves the epic parent via the registered event instead of the fold's proposal-keyed entry.

### Adapter: exact match, any status

Requirement `b8d894dff9b5` re-describes: the pre-create check matches the op's label exactly, in any bead status — the open-only filter existed solely to dodge node-key collisions and dies with them. Receipts remain the recovery mechanism; `fd6f08ef34fa` *Re-run idempotency* re-describes as deterministic eids journal-side, receipts (`op_id → bead_id`) adapter-side. `scripts/apply-br.sh` follows. Close idempotency `7bad082a34b6` (`spex:obsolete` + `commit:<HEAD>` markers) is state, not linkage — untouched. Changeset stays v2: the label *value* changes, the schema does not.

### BeadReader joins by task id

`BeadReader` `bec96486c6b2` (requirement `5179e5a95653`) stops parsing labels: pairings arrive from the fold carrying task ids, and live status joins onto them by task id. The label becomes what it always should have been — an adapter-facing key spex reads nothing from.

### The fold and ingest drop the special case

Orphan invariant `ee28b5d190ae` simplifies: every `task_created` references an existing event — the "or proposal" branch dies, epics now reference their registered event. `Reconciler` `2b5158af774b` dedups the epic's receipt by eid like every other line. Legacy journal lines — `proposal`-field `task_created`s, `spex:<int>`/`spex:<spec_node_id>`/`spex:cleanup-<hash>` labels on existing beads — stay inert history behind a read-only fold branch; no migration, no relabeling. In-flight proposals registered before this change get their `registered` event from a one-shot backfill line committed with the migration (`2026-08-02-merge-impact-emit` is the known candidate), the same precedent the journal's own backfill set.

### Briefings get the bracket

`ContextResolver` `6b79188dff4c` (requirement `40a3d3155131`) serves `eid`, `event`, `before_head`, `after_head` for **any** node, live or removed: a live node's bracket is its latest task-bearing event's `git_head` against the preceding event's (`before_head` null for `added`); the removed path already answers this. `MapCommand` `08909d62930b` extends `context` output with the four fields, keyed by identity hash or task id as today. Downstream — out of scope here — faber's `gather-context` turns the bracket into a `git diff <before_head> <after_head> -- <leaves>` section, so an implementer sees the change, not just the state.

## Impact expectation

**A modify-only correction: no spec nodes added or removed.** Component re-describes: `Registrar`, `IdempotencyLabeler`, `Resolver`, `BeadReader`, `Reconciler`, `ContextResolver`, `MapCommand` — seven modify pairs — plus the reference adapter leaf, the journal-line schema leaf, nine requirement re-descriptions across six modules (titles keep their identity-bearing wording, the precedent `a2645b77b8bc` set), and the affected flow/test leaves (emit labeling scenarios, adapter idempotency tests, ingest reconciliation and partial-run tests, map context tests). `/spec`'s enumeration is authoritative.

**Module meta changes** (`proposal.requires_module` gains map) — outside the refresh-absorbable set, so the correction runs the full gate.

**Acceptance bar:** same ops, same order, same op ids; label values change by design — `spex:<eid>` everywhere a create op carries idempotency. Adapter behavior is equivalent or stricter (exact match beats filtered match). `spex map context` output is a superset of today's for every existing key.

**Out of scope:** the faber/dot `gather-context` hook change (downstream consumer, different repo); any changeset schema bump; any relabeling of existing beads; the impact/emit module merge — `2026-08-02-merge-impact-emit` is sequenced strictly after this proposal's epic closes and is authored against the post-eid spec.

**Prerequisite:** `2026-08-01-task-journal` fully landed (it is — epic `spexmachina-y0wc` closed).
