# Consistency invariants

Tests for the five journal invariants the ingest module enforces at baselining. The per-invariant
scenarios construct a state that SHOULD violate an invariant and assert that reconciliation rejects
it with a specific error; the snapshot-gate scenarios assert the gate behaves on both partial and
complete runs; and the Happy Path scenario asserts the positive integrated property that a clean
complete run leaves `spec/.history.jsonl` AND the snapshot both updated and schema-valid.

## The Five Invariants

1. Every ok create pairs exactly one `task_created` receipt with exactly one referent event — a
   change event, the `removed` event the cleanup answers (prior-batch or same-batch), or the
   proposal's `registered` event (epic creates). A `proposal`-keyed receipt with no `for` is a
   legacy line read as inert history, never constructed anew.
2. No receipt references an event id the journal does not contain.
3. Re-running the same changeset+receipts pair appends nothing — event ids derive from
   `(git_head, op_id)`.
4. The snapshot is saved iff receipts top-level status is complete, so journal and snapshot
   describe the same point-in-time state.
5. Every appended line validates against the journal-line schema before the write.

The retired store invariants — one record per node, modify rebinds the record, no leftover record
after a removal — are void by construction over an append-only log: lineage replaces them, and the
scenarios below include one that proves the replacement holds.

## Scenarios

### Invariant 1: ok create with no referent (negative test via injected bug)

- Construct a receipts batch whose ok create op matches no change event, no prior removed event,
  and carries no proposal stem.
- Expected: structured error naming the op; nothing appended.

### Invariant 1: double pairing refused

- Construct a batch where two `task_created` receipts would pair with the same change event.
- Expected: error naming the event id; nothing appended.

### Invariant 2: dangling receipt reference

- Inject a receipt whose `for` names an eid absent from both the existing journal and the batch
  being appended.
- Expected: error `"ingest: receipt references unknown event <eid>"`; nothing appended.

### Invariant 3: re-run appends nothing

- Run a complete reconcile; capture the journal byte-for-byte. Run the identical
  changeset+receipts again.
- Expected: journal unchanged, run reports success. Assert via content equality, not just line
  count.

### Invariant 4: partial → snapshot not saved

- Receipts top-level status is partial. Reconciler appends events for the ok ops.
- Expected: spec/.snapshot.json is unchanged on disk (assert via content equality against the
  pre-run baseline); the journal carries the ok ops' events — the two artifacts may legitimately
  diverge only in this direction and only until the completing re-run.

### Invariant 4: complete → snapshot saved

- Receipts status complete. Expected: snapshot rewritten with the current merkle tree in the same
  baselining step as the journal append.

### One snapshot format across both writers

**Given** a fixture spec tree, a fixed timestamp, and two destinations.
**When** the saver's atomic write produces one file and the `merkle` module's own in-place `Save`
produces the other.
**Then** the two files are byte-identical.

**Rationale**: two writers of one format is the shape this repository already got wrong once — the
saver carried its own tree walk described as "mirroring" merkle's, nothing compared the two, and
either could have drifted while both kept passing their own tests. The formats are one
implementation now; this scenario is what makes a second one fail loudly instead of silently. The
timestamp is fixed because `created_at` is the only field that would otherwise differ between two
writes.

### Invariant 5: schema-invalid line refused

- Construct a batch that would append a change event missing its `node` field.
- Expected: the journal-line schema validation fails before the write; error names the violated
  constraint; the on-disk journal is untouched.

### Lineage replaces the rebind invariant

- Run a modified-node close+create pair to completion.
- Expected: the journal holds both pairings — old task closed, new task created — and the fold
  answers with the new task only. No assertion anywhere demands the old line be gone; asserting
  its presence IS the test.

## Happy Path

- Full complete run with 5 ok creates and 3 ok closes — 2 of the creates paired with 2 of the closes as modify pairs, the third close a removal.
  All invariants pass. `spec/.history.jsonl` gains exactly the expected events and receipts,
  the snapshot is rewritten, and every appended line validates against the journal-line schema.

## Fixtures

In-memory Go harness, no on-disk testdata. The shipped tests reuse the package's shared helpers —
the fake spec graph and journal seeder from `reconciler_test.go`, `setupSpecDir` from
`snapshot_saver_test.go` — plus a local runner that drives `Reconciler.Apply` then `Saver.Save` in
the order IngestCommand wires them. Per-invariant violation states are built inline from those
helpers.
