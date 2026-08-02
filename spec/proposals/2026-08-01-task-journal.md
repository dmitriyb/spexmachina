# Change Proposal: The task journal — tasks pair with events, not nodes

## Context

`.bead-map.json` links each plan-relevant spec node to the task it produced: `spec_node_id → bead_id` plus eight fields of denormalized metadata, keyed by an integer record id that exists to form the `spex:<int>` idempotency label. A task, however, is born from a *change*, not from a node — `spex impact` maps diff entries to actions and every changeset op is event-shaped — and `ingest`'s Reconciler then flattens the op into the node-keyed record, discarding the event identity. Three standing problems are symptoms of that flattening, each measured:

- **Cleanup labels are unresolvable.** A `spex:cleanup-<hash>` task references a node that left the spec; by design it has no map record, so nothing in the repository can say what the hash named.
- **Modify lineage is ambiguous.** 33 of the 81 distinct `spex:<int>` label values are carried by more than one bead — several events sharing one node key. *(Method: `^spex:[0-9]+$` labels in `.beads/issues.jsonl`, ints on ≥2 bead ids; independently matches the reduced-task-map proposal's count.)*
- **Removed-node recovery leans on a tracker artifact.** 32 of 168 beads carry a label whose record is gone *(method: label ints absent from `.bead-map.json` record ids at the contracts tip)*, and `validator/removed_name_checker.go:340-372` reads `Module`/`Component` from map records as its retired-node name source — so a spex user with no tracker gets weaker validation, and the map fields cannot be dropped while the validator reads them.

A fourth motivation is authoring precision: an implementing agent today receives the *current* leaf and must infer the change against the code. If a task knew its own before/after refs, its briefing becomes *the exact spec delta + the current leaf + the code* and the inference step disappears. Node-pairing cannot express "this task's before/after"; event-pairing provides it structurally.

This proposal supersedes `2026-07-29-reduced-task-map.md` (unlanded): its renames and field-elimination survive here, its node-keyed two-field record does not. It also settles the `dce73d6` draft's idea A — the map is eliminated, but into a spex-owned journal rather than into tracker labels, which is what makes elimination safe where the draft's version was not (31 tasks would have lost their linkage irrecoverably; here nothing is lost — see backfill). It assumes `2026-07-25-declarative-spec-contracts` has landed.

## Proposed change

### The journal

One append-only file, `spec/.history.jsonl`, one JSON object per line, written only by `ingest` — both modes — with the snapshot's existing atomic write-and-rename. Change events record what a baselining absorbed: `{"event":"added|removed|modified","eid":<id>,"node":<identity-hash>,"name":…,"node_type":…,"module":…,"before":<leaf-hash|null>,"after":<leaf-hash|null>,"git_head":…,"proposal":…}`. Receipt events record what the tracker did: `{"event":"task_created|task_closed","for":<eid>,"task_id":…}`. Refresh runs append their own receipt naming the events they absorbed. `git_head` is free on the normal path — `spex emit` already requires `--git-head` and the changeset carries it; refresh accepts an optional `--git-head` and otherwise records its absence.

Three constraints are part of the contract, not implementation detail: the journal stores **pointers, never content** (spec byte-history stays in git; "show the change" is `git diff <before-head> <after-head> -- <leaf>`, derived); **no chaining or proofs** (plain JSONL, single writer, the trust model of `.beads/issues.jsonl`); and it is **never load-bearing for gating** — `validate`, `diff` and the completeness checker read spec and snapshot only, and a corrupt journal degrades name resolution, never the pipeline. Event ids are journal-internal, minted once at ingest; idempotency does not depend on them (see labels).

### `.bead-map.json` is deleted; readers fold the journal

The map file, its schema, and the integer record id all go. Current-state linkage — "the latest task-bearing event per node" — is a fold over the journal, computed in-process by each reader, exactly how `br` rebuilds its view from `issues.jsonl`:

- **`emit`**: `IdempotencyLabeler` `6f4b6dd8928f` loses its cursor and store dependency (requirement `0d468e176aaf`); labels become `spex:<spec_node_id>`, matching the `spex:cleanup-<hash>` shape already in use, so idempotency lives entirely in the label and existing `spex:<int>` labels become inert history needing no migration. Parent resolution (requirement `79c821e01654`) folds the journal, and `Resolver` resolves `ref:spec_node` in-process — the resolved `bead_id` is written into the changeset, so **`ref:spec_node` leaves the adapter-facing op vocabulary** (requirement `240f59bf64f6`, changeset schema v2; `ref:op` and `ref:bead` remain). `scripts/apply-br.sh` drops its `SPEX_MAPPING_FILE` lookup — the reference adapter stops reading any spex-owned file.
- **`impact`**: `BeadReader` `bec96486c6b2` parses `spex:<spec_node_id>` instead of integers (requirements `5179e5a95653`, `d165e2fe215e`).
- **`ingest`**: `Reconciler` `2b5158af774b` stops maintaining nine record fields and instead appends change and receipt events (requirements `539030e8c5a4`, `ee28b5d190ae` — the orphan-record invariant is re-stated over the journal; `fd6f08ef34fa` re-run idempotency now rests on labels). `RefreshHandler` `f9033352c13f` appends refresh receipts (requirement `e68653819f38`).
- **`map`**: the module keeps all four api nodes — names are invocation strings and none change. `MappingStore` `205e67ca4aad` is re-described as the journal store (append, scan, fold). `ContextResolver` `6b79188dff4c` resolves live nodes from the spec (as the reduced-task-map proposal specified — measured there to reproduce the store's output exactly except five records where the store was stale) and removed nodes from the journal: name, type, module, the removing proposal, and the before/after `git_head` refs an agent needs for a diff-based briefing. `MapCommand` `08909d62930b` re-keys `get` on identity hash or task id (the integer id no longer exists) — `spex map get|list|context` keep their names.
- **`validator`**: `removed_name_checker.go` reads retired names from the journal instead of map records, giving tracker-less users full removal checking.
- **`schema`**: `BeadMapSchema` `d125b5e775b4` becomes the journal-line schema; requirement `f7ef8bef0ba1` is re-described.

### Backfill

A one-shot script replays the 20 committed versions of `.bead-map.json` plus the `issues.jsonl` labels into an initial journal. Measured: the 82 record ids that ever existed cover all 81 label ints — **0 unrecoverable** *(method: union of `.records[].id` over `git log -- .bead-map.json`, compared against label ints)* — so every one of the 32 currently-dark cleanup/stale references becomes resolvable. The script's output is reviewed and committed like any migration; it runs once, and the map file is deleted in the same change.

### Corrected in passing

Three drifts the reduced-task-map proposal already identified in `spec/map/arch_mapping_store.md`: the file-location claim (`spec/.bead-map.json` vs repository root — now moot, the section describes the journal), "maintained by `spex apply`" (a command that no longer exists), and § *Why auto-incrementing record IDs?*, replaced by the rationale for label-borne idempotency and journal-internal event ids.

## Impact expectation

**No node is added, removed or renamed.** Every affected node is modified in place — `MappingStore` keeps its name re-described over the journal, and all api identities survive because their invocation strings do. Project requirement `9120788210c9` *Map spec nodes to beads* is deliberately not edited, avoiding the project-level completeness cascade.

**Modified bead-producing nodes, by module:** map — `MappingStore` `205e67ca4aad`, `MapCommand` `08909d62930b`, `ContextResolver` `6b79188dff4c`, data flow `38b6d99f08bf`; ingest — `Reconciler` `2b5158af774b`, `RefreshHandler` `f9033352c13f`; emit — `IdempotencyLabeler` `6f4b6dd8928f`; impact — `BeadReader` `bec96486c6b2`; schema — `BeadMapSchema` `d125b5e775b4`; adapters — the reference-adapter contract leaf for the `ref:spec_node` removal. On the order of ten obsolete-plus-create pairs, roughly twenty operations. Requirement descriptions edited in map (`934d627f0e90`, `27c046dde129`, `4aee62bd3c15`), ingest (`539030e8c5a4`, `ee28b5d190ae`, `fd6f08ef34fa`, `e68653819f38`), emit (`79c821e01654`, `0d468e176aaf`, `240f59bf64f6`), impact (`5179e5a95653`, `d165e2fe215e`), schema (`f7ef8bef0ba1`); each obliges its implementors' leaves, which the substantive rewrites above already satisfy.

**Data migration:** the one-shot backfill, plus deletion of `.bead-map.json` and its schema file. Reversible while the old file remains in git history; touches no external system — existing tracker labels are not rewritten.

**Changeset schema:** v2 drops `ref:spec_node` from the op vocabulary. The reference adapter simplifies; any future adapter needs only create, close, label and an identifier — the boundary the decouple proposal originally wanted.

**Out of scope:** the `/spec`-skill briefing rewrite that consumes before/after refs (blocked on an A/B: three closed beads, one agent briefed each way, compare correctness and tokens); the merkle work (per-node structure hashes for meta attribution, diff labels, the neighbor attention set) which remains its own proposal; and the impact+emit merge (`dce73d6` idea B), which this change simplifies but does not perform. Both superseded documents — `2026-07-29-reduced-task-map.md` and the post-decouple draft — are merged into this branch for history and relocated to `spec/proposals/obsolete/`; their branches are then deleted. Idea A of the draft is settled here, idea B awaits its own future proposal, idea C is dropped.
