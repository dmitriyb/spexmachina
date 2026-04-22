# Idempotency labeling

Implementation notes for `IdempotencyLabeler`.

## Reserve-Then-Commit Pattern

```go
type Labeler struct {
    MappingStore mapping.Store
}

// Reserve returns n sequential labels starting from the store's current counter.
// Does NOT advance the persisted counter — that happens in ingest on a complete run.
func (l *Labeler) Reserve(n int) []string {
    start := l.MappingStore.NextRecordID()
    out := make([]string, n)
    for i := 0; i < n; i++ {
        out[i] = fmt.Sprintf("spex:%d", start+i)
    }
    return out
}
```

`MappingStore.NextRecordID()` reads the counter from `.bead-map.json` metadata (existing behavior: max existing record ID + 1, or 1 if empty).

## Why Emit Reserves But Does Not Commit

- **Emit is pure**: no file writes to `.bead-map.json`. Emit produces changeset.json; the mutation happens in ingest.
- **Re-runnable**: a failed emit run (e.g., cycle error before writing changeset) leaves the store untouched. The next attempt reserves the same label range.
- **Deterministic**: for a fixed `(spec state, mapping store state)` input, the reserved labels are identical across repeated invocations.

Ingest advances the counter as part of its upsert — one label consumed per successful create receipt. Partial runs advance the counter only by the number of successful creates.

## Label Format

- `spex:<n>` where `<n>` is a positive decimal integer.
- No zero padding.
- No reuse: once a record-id has been assigned to a bead (successful or obsoleted), that integer is never assigned to another record.

## Re-Emit After Partial Ingest

Sequence:

1. Run 1: emit reserves `spex:42`..`spex:44` for three new creates. Adapter runs; creates succeed for op1 and op2, fails on op3. Ingest commits records 42 and 43, counter advances to 44. Snapshot NOT saved (partial).
2. Run 2: spec graph unchanged. Emit re-runs. Impact report now shows only op3 as new (op1, op2 are mapped). Labeler reserves `spex:44` for the single create. Adapter runs; create succeeds with `spex:44` as the idempotency label — matches nothing in the tracker (since op3 never created a bead in run 1), so a fresh bead is created.
3. Ingest commits record 44. Counter advances to 45. Snapshot saved.

The label stayed stable for each spec node across attempts.

## Interaction with Ingest

Ingest's reconciler reads receipts.json, pairs each ok-create receipt with its label, and upserts the mapping store:

- `was_existing: false` + receipt's bead_id → create a new mapping record with `id = <label's integer>`, `bead_id = <receipt>`, `spec_node_id = <op's>`, etc.
- `was_existing: true` + receipt's bead_id → the label's integer already maps to this bead_id. Verify; update any drift-eligible fields (spec_hash).

The "label's integer" is parsed back from the `spex:<n>` string. Ingest does NOT invent new integers — it just reuses the ones emit placed in the changeset.
