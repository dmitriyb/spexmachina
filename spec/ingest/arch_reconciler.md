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

| Op type | spec_node_kind | Receipt status | was_existing | Transition |
|---------|----------------|----------------|--------------|------------|
| create  | `proposal_epic` | ok            | false        | **Insert** proposal record (see Proposal-Epic Ops below — no spec-graph lookup, special metadata defaults) |
| create  | `cleanup`       | ok            | any          | **No mapping change** — cleanup beads have no map record by design (see Cleanup-Create Ops below). Counter does NOT advance. |
| create  | other          | ok             | false        | **Insert** record at `id = parse(idempotency.label)`, spec_node_id = op's, bead_id = receipt's |
| create  | other          | ok             | true         | Record present with the receipt's bead_id → strict **no-op**, nothing reconciled. Record present with a *different* bead_id → **error**. No record at that id → fall through and **insert** (adapter-side recovery, below) |
| create  | any            | error          | —            | No-op |
| create  | any            | skipped        | —            | No-op |
| close   | —              | ok (reason="Spec node removed")  | — | **Delete** record by bead_id match |
| close   | —              | ok (reason starts "Spec node modified") | — | **No immediate action** — the paired create op handles the update |
| close   | —              | error          | —            | No-op |
| label   | —              | any            | —            | No mapping change (labels don't affect mapping records) |
| tag     | —              | any            | —            | No mapping change |

## The Modified-Node Pair

Emit generates two ops for a modified node: a close op for the old bead, plus a create op with the SAME record-id label as the old bead's record. Reconciler processes them as a pair:

1. See close op → do not delete the record (deferred decision).
2. See create op with label matching the closed bead's record → update the record's bead_id to the new bead_id.

Record-id preservation is what makes this "an update" rather than "delete + insert" at the mapping level. The record-id is the persistent identity; the bead_id is what changes.

Detection mechanism: the close op's reason field. "Spec node removed" → delete on close; "Spec node modified" → wait for create.

## Adapter-Side Recovery

`was_existing = true` means the adapter found an open bead already carrying the op's record-id label. Usually the local store agrees and the transition is a strict no-op — the store is already in the target state, so no field is refreshed and no counter moves. If the store instead holds a record at that id bound to a *different* bead, the two sources of truth have diverged and Reconciler errors out rather than picking a winner.

The remaining case is the interesting one: `was_existing = true` with **no** record at that id. That is the signature of a previous run where the adapter created the bead and then died before its receipt reached disk. Reconciler falls through to the ordinary insert path and materialises the record now, so the next emit sees the id as taken and does not re-create the bead. See `test_partial_run_recovery.md`, "Partial with Adapter-Side Duplicates".

## Proposal-Epic Ops

Create ops with `spec_node_kind == "proposal_epic"` represent a proposal as a
tracker bead. Their `spec_node_id` is the **proposal stem** (e.g.
`2026-04-29-decouple-contract-gaps`), NOT a 12-character identity hash, because
proposals live as markdown files under `spec/proposals/` and have no
spec-graph node.

The Reconciler MUST treat proposal-epic creates as a distinct case:

- **Skip** the `SpecGraph.NodeMetadata` lookup. The spec_node_id is not an
  identity hash; the lookup would always fail with `"spec graph: no node
  <stem>"` and the run would abort with exit 2.
- **Materialise** the new record with these defaults (no spec-graph
  metadata to populate):

  | Field          | Value                                              |
  |----------------|----------------------------------------------------|
  | `bead_type`    | `"epic"` (via `beadTypeFor("proposal_epic")`)      |
  | `node_type`    | `"proposal"` (matches the on-disk vocabulary used by the pre-existing `spexmachina-0lk` epic record; NOT `"proposal_epic"`) |
  | `spec_node_id` | `op.SpecNodeID` (the proposal stem, carried verbatim) |
  | `component`    | `op.SpecNodeID` (so the record's component field carries a stable identifier; `op.Title` like `"Proposal: <stem>"` is NOT used) |
  | `module`       | `""`                                               |
  | `content_file` | `""`                                               |
  | `spec_hash`    | `""`                                               |

This rule applies to BOTH the fresh-create branch and the modified-pair
update branch. Modified proposal_epic ops are rare (proposals don't usually
modify after their first create), but the no-spec-graph-lookup rule still
applies if one ever appears.

Invariant 4 (orphan check) MUST short-circuit on records with `node_type ==
"proposal"` because the proposal stem will never resolve through
`SpecGraph.HasNode`. Treating proposal records as orphan candidates would
produce false positives on every run.

## Cleanup-Create Ops

Create ops with `spec_node_kind == "cleanup"` represent a code-cleanup bead
for a spec node that was removed in a prior or concurrent batch. Their
`spec_node_id` carries the identity hash of the now-removed spec node (for
traceability — the spec graph no longer has a node at that hash, by design).
The cleanup bead exists in the tracker (label `spex:cleanup`, type `task`,
parented under the proposal epic) but has **no mapping record** — that's the
pre-decouple `apply/bead_creator.go::createCleanupBead` contract carried
forward into the post-decouple architecture.

The Reconciler MUST treat cleanup creates as a distinct case:

- **Branch on `op.SpecNodeKind == "cleanup"` BEFORE the recID parse and
  spec-graph lookup.** Skip the entire mapping-record materialisation: no
  `wc.put`, no `wc.advanceCounter`, no `lookupMetadata`. Count the op
  (`sum.OkCreates++`) and return nil.
- The op's `idempotency.label` (e.g. `spex:cleanup-<spec_node_id>`) is NOT
  parsed or used as a record id. The Reconciler does not allocate or refer
  to a record at that "id" because no record exists.
- Counter does NOT advance for cleanup creates — they don't consume the
  monotonic counter.

Invariant 1 ("every ok create has a record") MUST be amended: cleanup
creates are exempt by design — no record by construction. The check
short-circuits when the iteration encounters a cleanup op.

Invariant 4 (orphan check) is trivially satisfied for cleanup beads because
no record was materialised. No exemption logic is needed in invariant 4 for
cleanup itself; the absence of a record is the proof.

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
    // Records with node_type == "proposal" are exempt — their spec_node_id
    // is the proposal stem (not in the identity-hash keyspace).
    for _, rec := range r.MappingStore.All() {
        if rec.NodeType == "proposal" {
            continue
        }
        if !r.SpecGraph.HasNode(rec.SpecNodeID) {
            return fmt.Errorf("ingest: invariant 4: orphan record for spec_node %s", rec.SpecNodeID)
        }
    }

    // Invariant 5: duplicates. Scan records by spec_node_id; count > 1 → error.

    // Invariant 6: enforced by SnapshotSaver's gate.

    // Invariant 7: not checked here. Store.Replace validates the whole
    // candidate file against the bead-map schema before the atomic
    // rename, so the commit itself enforces it.
    return nil
}
```

## Where a record's `node_type` comes from

The `node_type` written onto a record is the kind of the spec node the op targets: the spec graph's metadata for `op.spec_node_id`, falling back to the op's own `spec_node_kind` when the graph has nothing to say (the `proposal_epic` case above, which maps to `proposal`). Reconciler does not choose the vocabulary; it copies it.

Two independent gates keep that copied value inside the closed enum the bead-map schema declares. Emit refuses to order a create op for a kind it cannot tier, so an op for an unmappable node kind never exists to be reconciled; and `Store.Replace` re-validates the entire candidate file against the bead-map schema before the atomic rename, which is invariant 7. The pair fails closed — a kind outside the enum is rejected at the write boundary, leaving `.bead-map.json` untouched, rather than persisting a record that no later run could resolve.

This is why the set of mappable node kinds belongs in the schema rather than being implied by the transition table above. The table discriminates the cases Reconciler handles specially; the enum states which kinds may reach a record at all. Retiring a kind that was never in the enum changes neither: no op was ever built for one, so no transition and no record refers to it.

## Transaction Semantics

- Reconciler applies changes to an in-memory copy of the mapping store.
- Only after AssertInvariants succeeds does it commit to disk via the store's atomic write.
- A failure at any step leaves the on-disk .bead-map.json untouched.

## Sole writer, no tracker

Reconciler is the only component that writes `.bead-map.json` on the normal path. Emit reads the store — record lookup for modify-pairs, the `next_id` counter for fresh labels — and never writes it; the counter advances here, on a reconcile, not at emit time. That asymmetry is deliberate: a failed emit that never reaches ingest must leave the store byte-identical, so the next emit re-derives the same labels and the run is retryable.

Reconciler also never contacts a tracker. It learns what happened solely from the `(changeset, receipts)` pair — which is why a receipt's `bead_id` and `was_existing` are load-bearing rather than advisory, and why an op with an `error` receipt is a no-op rather than a trigger for a status query. `spex` closes the loop on file evidence alone; the adapter is the only participant that ever ran a tracker command.

The practical consequence is that ingest is replayable. Given the same changeset and receipts, reconciling twice produces the same store, because every transition is keyed off the record id carried in `idempotency.label` rather than off tracker state that may have moved underneath.

## Ordering

Within a run, ops are processed in the order they appear in changeset.json — same order the adapter executed them. This matches receipts order. Ordering matters for the modified-node pair (close must precede create; emit always emits them in that order).

## Error Surface

All errors are wrapped with `"ingest: reconcile: ..."` context. Invariant failures name the specific invariant and the offending spec_node_id or op_id.
