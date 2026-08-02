# Draft: Post-decouple architectural cleanup

**Status:** conceptual draft, not yet a finalized proposal. Input for `/propose`.

This is a holding document for an architectural-simplification idea that emerged during work on `proposal/fix-decouple-contract-gaps` (the PR that closes contract pieces lost in the decouple-from-br migration). The contract-gaps PR fixes the existing pipeline's correctness; this draft is about whether the post-decouple architecture itself can be simplified once the gaps are closed.

## Motivation

The `2026-04-18-decouple-spex-from-br` proposal split bead creation/closure out of the `spex` binary into `emit → adapter → ingest`. That decoupling solved a real problem (no in-binary subprocess calls to `br`/`bd`/etc., adapter-pluggable) but left behind some architectural cruft:

1. `.bead-map.json` is a denormalized cache. Every field except `bead_id` is derivable from spec + snapshot. The `bead_id` itself is on the bead in the tracker as the `spex:<int>` label.
2. The `mapping/` module exists to maintain `.bead-map.json` and provides a CLI surface (`spex map context`) that skills consume to bridge bead → spec.
3. `impact` and `emit` are two stages with one consumer between them: `emit` is the only thing that reads `impact`'s `ImpactReport`. The intermediate JSON is internal plumbing, not a stable cross-tool contract.
4. `ingest`'s Reconciler exists almost entirely to maintain `.bead-map.json`. If `.bead-map.json` goes away, Reconciler shrinks to "save snapshot iff complete" — a few lines.

These four observations chain together. Eliminating the bead-map cascades into shrinking ingest and arguably justifies merging impact+emit (their separation only existed to produce a record-id-aware impact report). The result is a leaner pipeline with fewer moving parts.

## Three coupled ideas

### A. Eliminate `.bead-map.json` (drop the `mapping/` module)

Move the `bead_id ↔ spec_node_id` linkage from `.bead-map.json` to the bead itself: every bead carries `spex:<spec_node_id>` as a label. Skills that today do "bead → bead-map record → spec_node_id → spec context" instead do "bead → strip prefix from spex label → spec_node_id → spec context."

What gets simpler:
- No counter (`next_id` disappears). No `Labeler.Reserve` cursor.
- Modify-pair "label reuse" becomes implicit: the label IS the spec_node_id, which doesn't change across a modify pair. Re-emits naturally produce the same label without lookups.
- Atomicity invariant collapses to snapshot-only.
- Orphan check (current invariant 4) collapses — there are no records to orphan.

What costs effort:
- Migration: every existing bead in the tracker has `spex:<int>` labels; need a one-shot script to add `spex:<spec_node_id>` labels (and optionally drop the int form).
- All consumers of bead-map need rewriting (emit's modify-pair lookup, impact's classifier, ingest's reconciler, `spex map context` CLI).
- Skills (`/implement`, `/review`, `/cleanup`) lose `spex map context <int>`; gain `spex map context <bead_id>` or read the label directly.

### B. Merge `impact` and `emit` into one module (`plan`)

The intermediate `ImpactReport` JSON has exactly one consumer: `emit`. It's not a stable inter-tool contract. Merging:

- One pass over (diff + tracker view + spec graph + git_head + proposal) → changeset.json.
- Internal pieces (Classifier, Sorter, Resolver, Labeler, Builder) survive but live in one module.
- One CLI command instead of two: `spex plan` (or extend `spex emit`).

Independent of (A), but (A) makes (B) more attractive — both modules currently read bead-map, so removing it is a coordinated change to both.

### C. Collapse `ingest` to "save snapshot if complete"

Without `.bead-map.json`, ingest's per-op transition table goes away. The remaining job is "if receipts.status == complete, write a fresh snapshot from the current spec." That's ~50 lines of code in one function.

Whether this remains its own command (`spex save-snapshot`) or folds into the adapter wrap-up step is a small naming question. Probably keep it separate — the `complete`-status gate is a clean discriminator that doesn't belong inside the adapter.

## Pipeline shape after the cleanup

Before:
```
spex validate → spex diff → spex impact → spex emit → adapter → spex ingest
                                                                    ↓
                                                         spec/.snapshot.json
                                                          .bead-map.json
```

After:
```
spex validate → spex diff → spex plan → adapter → spex save-snapshot
                                                          ↓
                                                spec/.snapshot.json
```

Six stages → five. Two Go modules dissolved (`mapping/`, most of `ingest/`). One module merged (`impact + emit → plan`). One persistent file removed (`.bead-map.json`).

## Open questions

- **proposal_epic label format.** Today the proposal-epic record's `spec_node_id` is the proposal stem (e.g. `2026-04-29-decouple-contract-gaps`), not an identity hash. New scheme: bead carries label `spex:<stem>`. Stems are unique by virtue of dated slug naming. Consistent with the rule "label after `spex:` is the spec_node_id."

- **cleanup-bead label.** Cleanup beads have no spec node by construction (the node was removed). Two options:
  - `spex:cleanup-<spec_node_id>` keyed on the now-removed node's identity hash. Traceable via git history (find the proposal that removed it). Used in the contract-gaps PR.
  - `spex:cleanup-of:<old-bead-id>` keyed on the obsoleted bead. Simpler conceptually but less traceable.
  - Going with the first form is consistent with "label encodes spec_node_id."

- **CLI surface backward compatibility.** Should `spex impact` and `spex emit` survive as deprecated wrappers, or is a hard cutover acceptable? Argues for hard cutover: the bead-map elimination already breaks `spex map ...`, so we're already in a major-version-bump story.

- **Migration order.** Three sub-PRs probably:
  1. Add `spex:<spec_node_id>` labels alongside existing `spex:<int>` labels (dual-label transition). All existing beads get the new label; new beads get both forms.
  2. Rewrite consumers (emit, impact, Reconciler, skills) to prefer new labels. Old labels still work as fallback.
  3. Migrate fully, drop bead-map and the `mapping/` module, drop old `spex:<int>` labels.
  - Or land it all in one PR if test coverage is solid. Risk depends on how many in-flight branches exist.

- **`/converge` skill changes.** Currently summarizes changeset by `spec_node_kind`. New scheme uses the same vocabulary but with one less field of bead-map state to inspect. Probably minimal skill changes, maybe just simpler.

- **Test fixtures.** Lots of test data under `scripts/testdata/idempotency/`, `emit/testdata/`, `ingest/testdata/` — fixtures contain `spex:<int>` labels and bead-map snippets. Many would need updating. Probably easier to regenerate from a fresh end-to-end run than to patch by hand.

## What this proposal does NOT change

- The diff/snapshot mechanism. Merkle hashing, snapshot atomicity, the snapshot-as-baseline contract — all intact.
- The adapter contract. The adapter still consumes `changeset.json`, writes `receipts.json`. Op vocabulary stays the same. Tool-agnostic property preserved.
- The proposal lifecycle. Proposals → /spec → /converge stays the user-facing flow.
- The spec graph itself. Identity hashes, edges, content leaves — unchanged.
- The bead lifecycle conceptually. Beads still go through fresh-create → implementation → close. Modify-pair semantics still exist; just the bookkeeping moves to bead labels.

## Where the discussion lives

- `proposal/fix-decouple-contract-gaps` PR (TBD #) — the precursor work that closes the gaps lost in decoupling. This proposal builds on top of that PR being merged.
- `proposal/pipeline-cleanup-and-refresh-mode` branch — earlier work on /converge skill itself; orthogonal to this architectural question.

## Suggested next step

Run `/propose` with this draft as input. The skill will research the implications more thoroughly (component-level impact, test-fixture audit, skill-side effect mapping), nail down the migration plan, and produce a finalized proposal under `spec/proposals/`.
