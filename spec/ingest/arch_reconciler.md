# Reconciler

Consumes `changeset.json` + `receipts.json` and applies per-op state transitions to the mapping store. Asserts the seven consistency invariants before returning.

## Responsibilities

- Pair each op in the changeset with its corresponding receipt by op_id.
- For ok ops, derive the mapping transition from the op type and the op's spec_node_id.
- For error/skipped ops, leave the mapping unchanged.
- After applying all transitions, run AssertInvariants.
- Atomically commit the new .bead-map.json.

## Interface

```go
type Reconciler struct {
    MappingStore mapping.Store
    SpecGraph    *spec.Graph
}

func (r *Reconciler) Apply(cs emit.Changeset, rc receipts.Receipts) (Summary, error)

type Summary struct {
    OkCreates    int
    OkCloses     int
    Skipped      int
    Errors       int
    RecordsAdded int
    RecordsUpdated int
    RecordsDeleted int
}
```

## Per-Op Transition Table

| Op type | Receipt status | was_existing | Transition |
|---------|----------------|--------------|------------|
| create  | ok             | false        | **Insert** record at `id = parse(idempotency.label)`, spec_node_id = op's, bead_id = receipt's |
| create  | ok             | true         | **No-op** if existing record already matches receipt's bead_id; else update fields that drift |
| create  | error          | —            | No-op |
| create  | skipped        | —            | No-op |
| close   | ok (reason="Spec node removed")  | — | **Delete** record by bead_id match |
| close   | ok (reason starts "Spec node modified") | — | **No immediate action** — the paired create op handles the update |
| close   | error          | —            | No-op |
| label   | any            | —            | No mapping change (labels don't affect mapping records) |
| tag     | any            | —            | No mapping change |

## The Modified-Node Pair

Emit generates two ops for a modified node: a close op for the old bead, plus a create op with the SAME record-id label as the old bead's record. Reconciler processes them as a pair:

1. See close op → do not delete the record (deferred decision).
2. See create op with label matching the closed bead's record → update the record's bead_id to the new bead_id.

Record-id preservation is what makes this "an update" rather than "delete + insert" at the mapping level. The record-id is the persistent identity; the bead_id is what changes.

Detection mechanism: the close op's reason field. "Spec node removed" → delete on close; "Spec node modified" → wait for create.

## Invariant Enforcement

Run AssertInvariants after all transitions:

```go
func (r *Reconciler) AssertInvariants(cs emit.Changeset, rc receipts.Receipts) error {
    // Invariant 1: every ok create has a record.
    for _, op := range cs.Ops { ... }

    // Invariant 2: every ok close-on-removed has no record.
    for _, op := range cs.Ops { ... }

    // Invariant 3: modified-pair records point to new bead_id.
    for _, op := range cs.Ops { ... }

    // Invariant 4: orphans. Scan all records; for each, confirm spec_node_id exists in SpecGraph.
    for _, rec := range r.MappingStore.All() {
        if !r.SpecGraph.HasNode(rec.SpecNodeID) {
            return fmt.Errorf("ingest: invariant 4: orphan record for spec_node %s", rec.SpecNodeID)
        }
    }

    // Invariant 5: duplicates. Scan records by spec_node_id; count > 1 → error.

    // Invariant 6: enforced by SnapshotSaver's gate.

    // Invariant 7: re-validate .bead-map.json against schema.
    return r.MappingStore.ValidateSchema()
}
```

## Transaction Semantics

- Reconciler applies changes to an in-memory copy of the mapping store.
- Only after AssertInvariants succeeds does it commit to disk via the store's atomic write.
- A failure at any step leaves the on-disk .bead-map.json untouched.

## Ordering

Within a run, ops are processed in the order they appear in changeset.json — same order the adapter executed them. This matches receipts order. Ordering matters for the modified-node pair (close must precede create; emit always emits them in that order).

## Error Surface

All errors are wrapped with `"ingest: reconcile: ..."` context. Invariant failures name the specific invariant and the offending spec_node_id or op_id.
