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
| create  | other          | ok             | true         | **No-op** if existing record already matches receipt's bead_id; else update fields that drift |
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
