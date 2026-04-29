# IdempotencyLabeler

Assigns each create op an `idempotency.label`. The label format depends on
the action class — three branches: modify-pair, cleanup, fresh.

## Responsibilities

- For each create action, return the appropriate label per the per-action
  rules below. Labeler is **per-action**, not per-batch: the label depends
  on what the action looks like, not on the action's position in the
  ordered batch.
- Maintain an in-memory cursor for fresh creates only. Cursor reads from
  the mapping store's persisted counter on first use; the persisted counter
  itself is advanced by ingest on a complete reconcile, not by emit.
- Surface assigned labels to ChangesetBuilder so each op's JSON carries
  its label.

## Per-action rules

| Action class | Discriminator | Label format | Cursor effect |
|--------------|---------------|--------------|---------------|
| Modify-pair  | `action.OldBeadID != ""` AND `action.Reason` does NOT start with `"Code cleanup:"` | `spex:<existing-rec.ID>` (looked up via `MappingStore.GetByBead(action.OldBeadID)`) | Cursor does NOT advance |
| Cleanup      | `action.Reason` starts with `"Code cleanup:"` (the same `isCleanup` discriminator pre-decouple's `apply/bead_creator.go` used) | `spex:cleanup-<action.SpecNodeID>` | Cursor does NOT advance |
| Fresh        | All other creates (no `OldBeadID`, no cleanup Reason) | `spex:<cursor>` and advance the cursor | Cursor advances by 1 |

### Why modify-pair reuses the existing record-id

`Reconciler.applyCreate` keys its modify-pair detection on
`wc.byID[recID]` where `recID = parseRecordID(op.idempotency.label)`. If
emit hands a modify-pair create a fresh cursor value, Reconciler doesn't
find the existing record at that id — it inserts a parallel record at the
new id, and the original record (still pointing at the closed bead)
becomes orphaned. Invariant 3 fails (`modified bead X record still points
to old bead_id`). Reusing the existing record-id is what lets Reconciler
hit the modify-pair-update branch and rebind `bead_id` to the new value.

### Why cleanup uses a per-spec-node-id label

Pre-decouple `createCleanupBead` had NO idempotency check: re-runs would
create duplicate cleanup beads. Post-decouple, the adapter's idempotency
check before `br create` uses the op's `idempotency.label`. A label of
the form `spex:cleanup-<spec_node_id>` is unique per removed-node identity
hash and gives clean re-run idempotency. It does NOT consume the cursor
because cleanup beads have no mapping record and therefore no record-id
to allocate.

## Why label-at-emit-time, not adapter-time

The label is the adapter's idempotency key — it checks the tracker for a
bead carrying this label before creating. If two emit runs were to assign
labels for the same spec node differently, re-runs could duplicate beads.
Computing labels deterministically at emit time keeps them stable.

The mapping store's counter is advanced by ingest (on complete receipts),
not by emit. Emit only **reads** the counter for fresh creates — a failed
emit that never reaches ingest leaves the counter untouched, and the next
emit re-derives the same labels (modify-pair labels look up the same
existing record; cleanup labels are pure functions of `spec_node_id`;
fresh labels start from the same counter value). Emit is deterministic
over its inputs.

## Interface

```go
type Labeler struct {
    MappingStore mapping.Store
}

// LabelFor returns the idempotency label for one create action,
// applying the per-action rules. The internal cursor advances only
// for fresh creates.
//
// For modify-pair actions (OldBeadID != "" AND not a cleanup):
//   look up MappingStore.GetByBead(OldBeadID) and return spex:<existing-id>.
// For cleanup actions (Reason starts "Code cleanup:"):
//   return spex:cleanup-<action.SpecNodeID>. No cursor advance.
// For fresh creates:
//   return spex:<cursor> and advance the cursor.
func (l *Labeler) LabelFor(action CreateAction) (string, error)
```

The earlier `Reserve(n)` flat-slice API is replaced because per-action
labelling cannot be expressed as a fixed N-element range — modify-pair and
cleanup labels depend on the specific action, not on a sequence position.

## Conflict Avoidance

- A label is only assigned to a *new* create op. Close ops, label ops, and
  tag ops do not get `spex:<...>` assigned (they reference existing beads
  by `bead_id` or target spec_node_id).
- Fresh-create cursor starts at `max(existing record ids) + 1` when the
  store is loaded. There is no re-use of closed-record IDs.
- Modify-pair labels reuse the existing record's id by design — the
  Reconciler's modify-pair detection requires this match. The new-bead's
  ingestion *replaces* the bead_id in that record; the record-id stays
  the same.
- Cleanup labels are pure functions of `action.SpecNodeID`; identical
  inputs across runs produce identical labels. The adapter's `br list
  --json --label spex:cleanup-<spec_node_id>` lookup gives idempotent
  re-runs.
