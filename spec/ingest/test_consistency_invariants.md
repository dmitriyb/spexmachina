# Consistency invariants

Tests for the five journal invariants the ingest module enforces at baselining. Enforcement is
`InvariantChecker`'s: the per-invariant scenarios construct a state that SHOULD violate an
invariant and assert that the check rejects it with a specific error naming the invariant, whether
reached through `Reconciler.Apply` or against the checker directly. Line validity is
`JournalEncoder`'s: invariant 5's scenarios exercise the encoder that owns the schema gate. The
snapshot-gate scenarios assert the gate behaves on both partial and complete runs; and the Happy
Path scenario asserts the positive integrated property that a clean complete run leaves the
journal AND the snapshot both updated and schema-valid — both at the locations the lifecycle
resolver answered for the fixture project.

## The Five Invariants

1. Every ok create pairs exactly one `task_created` receipt with exactly one referent event — a
   change event, the `removed` event the cleanup answers (prior-batch, or minted by the cleanup
   op itself), or the proposal's `registered` event (epic creates) — every ok retarget pairs
   exactly one `task_retargeted` receipt with its own `modified` event, and a batch's absorbed
   events are closed by exactly one `refresh` receipt naming them. A `proposal`-keyed receipt
   with no `for` is a legacy line read as inert history, never constructed anew.
2. No receipt references an event id that neither the journal nor the batch contains — `for`
   fields and the entries of a `refresh` receipt's `absorbed` list alike.
3. Re-running the same changeset+receipts pair appends nothing — op-born event ids derive from
   `(git_head, op_id)` and absorb-born ones from `(node, before, after)`.
4. The snapshot is saved iff receipts top-level status is complete, so journal and snapshot
   describe the same point-in-time state.
5. Every appended line validates against the journal-line schema before the write.

The retired store invariants — one record per node, modify rebinds the record, no leftover record
after a removal — are void by construction over an append-only log: lineage replaces them, and the
scenarios below include one that proves the replacement holds. No invariant demands a
`task_closed` between one generation of a node's task and the next: the journal records what the
pipeline did, and a task's completion is not something the pipeline does.

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
- Expected: the snapshot is unchanged on disk (assert via content equality against the
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

### Invariant 5: the encoder refuses at its own boundary

- Hand `JournalEncoder` a deliberately schema-invalid event directly — no changeset, no
  reconciliation run around it.
- Expected: the encoder refuses the line naming the violated constraint, before any write path is
  reached. This exercises invariant 5 against the component that owns it rather than only through
  the integrated run above, so a future caller of the encoder inherits the gate rather than
  re-implementing it.

### Lineage replaces the rebind invariant

- Run a create for a node whose earlier task is finished — a plain create, no close beside it —
  to completion.
- Expected: the journal holds both pairings — the earlier `task_created`, with no `task_closed`
  after it, and the new one — and the fold answers with the new task only. No assertion anywhere
  demands the old line be gone or closed; asserting its presence, unclosed, IS the test.

### Invariant 1: retarget pairing

- Run an ok retarget to completion; then construct a batch where a `task_retargeted` receipt's
  `for` names an eid absent from journal and batch alike.
- Expected: the clean run appends exactly the `modified` event plus its `task_retargeted` — no
  `task_closed`, no `task_created` — and the dangling variant is refused naming the eid, nothing
  appended. Invariants 3 and 5 hold unchanged over the pair: a re-run appends nothing, and both
  lines validate against the journal-line schema before the write.

### Invariant 1: absorbed batch closes under one refresh receipt

- Run a batch with two absorbed entries to completion; then construct a batch whose `refresh`
  receipt names an eid no absorbed event carries.
- Expected: the clean run appends two `modified` events and exactly one `refresh` receipt naming
  both eids and nothing else; the dangling variant is refused before the write. Invariant 2's
  no-unknown-referent rule covers the `absorbed` list exactly as it covers `for`.

### Invariant 1: a cleanup's self-minted removal is one referent

- Run a cleanup create for a node with no prior `removed` event to completion.
- Expected: exactly one `removed` event, minted from the cleanup op's own `(git_head, op_id)`,
  and exactly one `task_created` naming it. Then re-run the pair: nothing appended, because the
  removal's eid derives from the same op.

## Happy Path

- Full complete run with 5 ok creates and 2 ok closes — 2 of the creates for nodes whose earlier
  tasks are finished, 1 a cleanup, the closes a removal and a fold-back.
  All invariants pass. The journal gains exactly the expected events and receipts,
  the snapshot is rewritten, and every appended line validates against the journal-line schema.

## Fixtures

In-memory Go harness, no on-disk testdata. The shipped tests reuse the package's shared helpers —
the fake spec graph and journal seeder from `reconciler_test.go`, `setupSpecDir` from
`snapshot_saver_test.go` — plus a local runner that drives `Reconciler.Apply` then `Saver.Save` in
the order IngestCommand wires them. Per-invariant violation states are built inline from those
helpers.
