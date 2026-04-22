# Reconciliation rules

Implementation of `Reconciler.Apply` — per-op transition table, modified-pair handling, invariant enforcement.

## Sketch

```go
func (r *Reconciler) Apply(cs emit.Changeset, rc receipts.Receipts) (Summary, error) {
    // 1. Pair ops with receipts.
    receiptsByOp := map[string]receipts.OpReceipt{}
    for _, or := range rc.Ops {
        receiptsByOp[or.OpID] = or
    }
    for _, op := range cs.Ops {
        if _, ok := receiptsByOp[op.OpID]; !ok {
            return Summary{}, fmt.Errorf("ingest: no receipt for op %s", op.OpID)
        }
    }
    for opID := range receiptsByOp {
        found := false
        for _, op := range cs.Ops {
            if op.OpID == opID { found = true; break }
        }
        if !found {
            return Summary{}, fmt.Errorf("ingest: receipt op_id %s not in changeset", opID)
        }
    }

    // 2. In-memory working copy of mapping store.
    working := r.MappingStore.Clone()

    // 3. Apply per op.
    var sum Summary
    for _, op := range cs.Ops {
        rc := receiptsByOp[op.OpID]
        if err := applyOp(working, op, rc, &sum); err != nil {
            return Summary{}, err
        }
    }

    // 4. Assert invariants against working copy.
    if err := r.AssertInvariants(working, cs, rc); err != nil {
        return Summary{}, err
    }

    // 5. Atomically commit.
    if err := r.MappingStore.CommitFrom(working); err != nil {
        return Summary{}, err
    }
    return sum, nil
}
```

## Per-Op Handlers

```go
func applyOp(m mapping.Store, op emit.Op, rc receipts.OpReceipt, sum *Summary) error {
    switch rc.Status {
    case "error":
        sum.Errors++
        return nil
    case "skipped":
        sum.Skipped++
        return nil
    case "ok":
        // fallthrough below
    default:
        return fmt.Errorf("ingest: op %s: unknown receipt status %q", op.OpID, rc.Status)
    }

    switch op.Type {
    case "create":
        sum.OkCreates++
        return applyCreate(m, op, rc, sum)
    case "close":
        sum.OkCloses++
        return applyClose(m, op, rc, sum)
    case "label", "tag":
        return nil // no mapping impact
    default:
        return fmt.Errorf("ingest: op %s: unknown type %q", op.OpID, op.Type)
    }
}
```

### applyCreate

```go
func applyCreate(m mapping.Store, op emit.Op, rc receipts.OpReceipt, sum *Summary) error {
    recID, err := parseRecordID(op.Idempotency.Label) // parses "spex:42" → 42
    if err != nil { return err }

    if rc.WasExisting {
        // Idempotent re-match. Update drift-eligible fields but keep record-id.
        rec, ok := m.Get(recID)
        if !ok {
            // Shouldn't happen — was_existing=true means tracker already had a bead
            // with our label, but our store doesn't have the record. Inconsistency.
            return fmt.Errorf("ingest: op %s: was_existing=true but no record for %s", op.OpID, op.Idempotency.Label)
        }
        if rec.BeadID != rc.BeadID {
            // Re-match against a different bead than we previously recorded — drift.
            return fmt.Errorf("ingest: op %s: was_existing bead_id %s does not match stored %s", op.OpID, rc.BeadID, rec.BeadID)
        }
        return nil // no-op
    }

    // Fresh create — insert or update.
    existing, had := m.Get(recID)
    if had && existing.SpecNodeID == op.SpecNodeID && existing.BeadID != rc.BeadID {
        // Modified-node pair: update bead_id to the new one.
        existing.BeadID = rc.BeadID
        existing.SpecHash = specHashOf(op.SpecNodeID)
        m.Put(existing)
        sum.RecordsUpdated++
        return nil
    }

    if had {
        return fmt.Errorf("ingest: op %s: record id %d collision (existing spec_node %s, op spec_node %s)", op.OpID, recID, existing.SpecNodeID, op.SpecNodeID)
    }

    m.Insert(mapping.Record{
        ID: recID,
        SpecNodeID: op.SpecNodeID,
        BeadID: rc.BeadID,
        Module: lookupModule(op.SpecNodeID),
        Component: lookupComponentName(op.SpecNodeID),
        ContentFile: lookupContent(op.SpecNodeID),
        SpecHash: specHashOf(op.SpecNodeID),
        BeadType: beadTypeFor(op.SpecNodeKind),
    })
    sum.RecordsAdded++
    m.AdvanceCounter(recID + 1)
    return nil
}
```

### applyClose

```go
func applyClose(m mapping.Store, op emit.Op, rc receipts.OpReceipt, sum *Summary) error {
    if strings.HasPrefix(op.Reason, "Spec node removed") {
        // Delete by bead_id lookup.
        if op.Target.Kind != "bead" {
            return fmt.Errorf("ingest: op %s: close target must be ref:bead", op.OpID)
        }
        m.DeleteByBeadID(op.Target.BeadID)
        sum.RecordsDeleted++
        return nil
    }
    // Modified: the paired create op handles the update; close is a no-op at the mapping level.
    return nil
}
```

## Counter Advance

Each successful create calls `m.AdvanceCounter(recID + 1)` which sets `next_record_id = max(current, recID + 1)`. After all ops, the counter reflects the max record-id committed in this run.

## Working-Copy Pattern

`m.Clone()` returns an in-memory duplicate of the store. `m.CommitFrom(working)` does an atomic write of the working copy's state back to disk. The `mapping.Store` already supports this pattern in existing code.

## Error Wrapping

All errors: `"ingest: reconcile: ..."` context.
