# Idempotency labeling

Implementation notes for `IdempotencyLabeler`.

## Per-Action LabelFor Pattern

```go
type Labeler struct {
    MappingStore mapping.Store
    // cursor lazily initializes from MappingStore.NextRecordID() on the
    // first LabelFor call and advances in memory on each fresh create.
    cursor      int
    initialized bool
}

// LabelFor returns the idempotency label for one create action,
// branching on action class. Cleanup is checked before modify-pair
// because cleanup actions also carry OldBeadID (lineage) but get a
// different label format.
func (l *Labeler) LabelFor(action CreateAction) (string, error)
```

The three branches:

1. **Cleanup** (`Reason` starts with `"Code cleanup:"`): return
   `spex:cleanup-<action.SpecNodeID>`. The cursor does NOT advance —
   cleanup beads have no mapping record, so no record-id is consumed.
2. **Modify-pair** (`OldBeadID != ""` and not a cleanup): look up
   `MappingStore.GetByBead(OldBeadID)` and return
   `spex:<existing-rec.ID>`. The cursor does NOT advance — the
   replacement bead reuses the obsoleted record's id, so ingest updates
   the same record instead of inserting a new one.
3. **Fresh create**: return `spex:<cursor>` and advance the in-memory
   cursor.

`MappingStore.NextRecordID()` reads the counter from `.bead-map.json`
metadata (existing behavior: max existing record ID + 1, or 1 if empty);
the Labeler reads it once, lazily, on the first `LabelFor` call. A
`NextLabel()` accessor exposes the would-be-next id without consuming it.

## Why Emit Labels But Does Not Commit

- **Emit is pure**: no file writes to `.bead-map.json`. Emit produces changeset.json; the mutation happens in ingest.
- **Re-runnable**: a failed emit run (e.g., cycle error before writing changeset) leaves the store untouched. The next attempt starts from the same persisted counter, so each action re-derives the same label.
- **Deterministic**: for a fixed `(spec state, mapping store state)` input, `LabelFor` returns identical labels for the same ordered action sequence across repeated invocations — fresh creates consume cursor values in sorted-op order; cleanup and modify-pair labels are functions of the action alone.

Ingest advances the persisted counter as part of its upsert — one fresh-create label consumed per successful create receipt. Partial runs advance the counter only by the number of successful creates.

## Label Format

- Fresh and modify-pair: `spex:<n>` where `<n>` is a positive decimal integer. No zero padding.
- Cleanup: `spex:cleanup-<spec_node_id>` — unique by removed-node identity hash.
- No reuse across records: once a record-id has been assigned to a bead (successful or obsoleted), that integer is never assigned to a *different* record. Modify-pair reuse of the same record's id is the mechanism, not a violation.

## Re-Emit After Partial Ingest

Sequence:

1. Run 1: emit labels three fresh creates `spex:42`, `spex:43`, `spex:44` (cursor 42..44 in sorted-op order). Adapter runs; creates succeed for op1 and op2, fails on op3. Ingest commits records 42 and 43, counter advances to 44. Snapshot NOT saved (partial).
2. Run 2: spec graph unchanged. Emit re-runs. Impact report now shows only op3 as new (op1, op2 are mapped). `LabelFor` on the single fresh create returns `spex:44` (cursor initialized from the advanced counter). Adapter runs; the create succeeds with `spex:44` as the idempotency label — matches nothing in the tracker (since op3 never created a bead in run 1), so a fresh bead is created.
3. Ingest commits record 44. Counter advances to 45. Snapshot saved.

The label stayed stable for each spec node across attempts.

## Interaction with Ingest

Ingest's reconciler reads receipts.json, pairs each ok-create receipt with its label, and upserts the mapping store:

- `was_existing: false` + receipt's bead_id → create a new mapping record with `id = <label's integer>`, `bead_id = <receipt>`, `spec_node_id = <op's>`, etc.
- `was_existing: true` + receipt's bead_id → the label's integer already maps to this bead_id. Verify; update any drift-eligible fields (spec_hash).

The "label's integer" is parsed back from the `spex:<n>` string. Ingest does NOT invent new integers — it just reuses the ones emit placed in the changeset. Cleanup labels (`spex:cleanup-*`) carry no integer and materialise no record.
