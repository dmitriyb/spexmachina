# Consistency invariants

Tests for the seven consistency invariants the ingest module enforces after applying receipts. The per-invariant scenarios construct a state that SHOULD violate an invariant and assert that reconciliation rejects it with a specific error; the invariant-6 scenarios assert the snapshot gate behaves on both partial and complete runs; and the Happy Path scenario asserts the positive integrated property that a clean complete run leaves `.bead-map.json` AND the snapshot both updated and schema-valid.

## The Seven Invariants

1. Every ok create op has a mapping record with its bead_id.
2. Every ok close on a removed node has NO mapping record.
3. Every modified node's close+create pair has a record pointing to the new bead_id.
4. No orphan records (record whose spec_node_id doesn't exist in the post-apply spec tree).
5. No duplicate records (same spec_node_id twice).
6. Snapshot saved iff receipts top-level status is complete.
7. `.bead-map.json` passes the bead-map schema.

## Scenarios

### Invariant 1: ok create missing record (negative test via injected bug)

- Simulate a reconciler bug by bypassing the create path and running AssertInvariants with an ok create op whose spec_node_id has no record.
- Expected: error `"ingest: invariant 1: ok create op <op_id> has no mapping record for spec_node <id>"`.

### Invariant 2: close-on-removed leaves a record

- Apply reconciler with a close op whose reason is "Spec node removed" and status ok. Then manually inject a leftover record for the removed spec_node_id. AssertInvariants.
- Expected: error `"ingest: invariant 2: removed spec_node <id> still has mapping record"`.

### Invariant 3: modified node points to old bead_id

- Apply close+create for a modified node. Manually set the record's bead_id to the old (pre-close) value. AssertInvariants.
- Expected: error `"ingest: invariant 3: modified node <id> record points to old bead_id <old> not new <new>"`.

### Invariant 4: orphan record

- Initial map has a record for spec_node `orphan1` that doesn't exist in the spec tree after the reconcile. (Simulates a bug where a node was deleted without generating a close op.)
- Expected: error `"ingest: invariant 4: orphan record for spec_node <id>"` OR a warning if the contract is to merely flag orphans. The current implementation promotes to error to match CLAUDE.md's "Do not leave orphans" guidance.

### Invariant 5: duplicate spec_node_id

- Inject two records with the same spec_node_id.
- Expected: error `"ingest: invariant 5: duplicate records for spec_node <id>"`.

### Invariant 6: partial → snapshot not saved

- Receipts top-level status is partial. Reconciler runs successfully. AssertInvariants.
- Expected: spec/.snapshot.json is unchanged on disk (assert via mtime or content equality against the pre-run baseline).

### Invariant 6: complete → snapshot saved

- Receipts status complete. Expected: snapshot rewritten with current merkle tree.

### One snapshot format across both writers

**Given** a fixture spec tree, a fixed timestamp, and two destinations.
**When** the saver's atomic write produces one file and the `merkle` module's own in-place `Save` produces the other.
**Then** the two files are byte-identical.

**Rationale**: two writers of one format is the shape this repository already
got wrong once — the saver carried its own tree walk described as "mirroring"
merkle's, nothing compared the two, and either could have drifted while both
kept passing their own tests. The formats are one implementation now; this
scenario is what makes a second one fail loudly instead of silently. The
timestamp is fixed because `created_at` is the only field that would otherwise
differ between two writes.

### Invariant 7: schema violation

- After reconciliation, manually corrupt .bead-map.json (missing required field on a record). AssertInvariants.
- Expected: schema validator error surfaced through AssertInvariants.

## Happy Path

- Full complete run with 5 ok creates, 3 ok closes (2 modified, 1 removed), 1 modified pair. AssertInvariants returns nil. .bead-map.json and snapshot both updated and schema-valid.

## Fixtures

In-memory Go harness, no on-disk testdata. The shipped tests reuse the
package's shared helpers — `newFakeSpecGraph` and `newTestStore` from
`reconciler_test.go`, `setupSpecDir` from `snapshot_saver_test.go`, and
the `idem` label helper — plus the local `runWithSnapshot` in
`ingest/consistency_invariants_test.go`, which drives `Reconciler.Apply`
then `Saver.Save` in the order IngestCommand wires them. Per-invariant
violation states are built inline from those helpers.
