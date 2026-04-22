# IdempotencyLabeler

Assigns each create op an `idempotency.label` of the form `spex:<record-id>` where `record-id` is the next integer from the mapping store's monotonic counter.

## Responsibilities

- Reserve the next record-id range up front, before emitting any create op. The reserved range sits in memory for the emit run; the mapping store counter is advanced only when ingest commits the run.
- Assign labels in emission order: first create in the sorted batch → `spex:<next>`, second → `spex:<next+1>`, and so on.
- Surface the assigned labels to ChangesetBuilder so each op's JSON carries its label.

## Why label-at-emit-time, not adapter-time

The label is the adapter's idempotency key — it checks the tracker for a bead carrying this label before creating. If two emit runs were to reassign labels for the same spec node differently, re-runs could duplicate beads. Reserving at emit time and persisting through ingest keeps the label stable.

The mapping store's counter is advanced by ingest (on complete receipts), not by emit. Emit only **reserves** — a failed emit that never reaches ingest leaves the counter untouched, and the next emit reuses the same starting value. Because emit is deterministic over its inputs, the reserved range is stable across re-attempts with identical inputs.

## Interface

```go
type Labeler struct {
    MappingStore mapping.Store
}

// Reserve returns labels[i] for op index i. Advances an in-memory cursor; the store's
// persisted counter is bumped by ingest on a successful full receipt.
func (l *Labeler) Reserve(n int) []string
```

## Conflict Avoidance

- A label is only assigned to a *new* create op. Close ops, label ops, and tag ops do not get `spex:<id>` assigned (they reference existing beads by bead_id or target spec_node_id).
- The mapping store's counter starts at `max(existing record ids) + 1` when the store is loaded. There is no re-use of closed-record IDs.
- Re-emit: if a prior emit already ran and ingest partially committed, the persisted counter reflects committed records. A subsequent emit begins from that counter. Ops that were already committed by the previous run's ingest carry mapping records and will not re-appear in the impact report, so their labels are not re-reserved.
