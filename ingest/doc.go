// Package ingest is the pipeline tail: it moves the task journal and
// the merkle snapshot together, in one of two modes dispatched by
// IngestCommand's --mode flag (see spec/ingest/flow_ingest.md). Both
// modes terminate with the same atomicity guarantee: snapshot and
// spec/.history.jsonl represent the same point-in-time spec state, or
// neither moves.
//
// # Mode: normal (default)
//
// Reconciles the task journal from a paired changeset.json (typed by
// emit) + receipts.json (typed by adapters). Pure function over local
// files — no subprocesses, no tracker calls, no git calls:
//
//  1. Pre-flight: parse both files, changeset version == 2 / receipts
//     version == 1 check, op_id set equality between changeset and
//     receipts.
//  2. Reconciler.Apply: construct the batch's journal lines in memory —
//     change events and task receipts, with event ids derived from
//     (git_head, op_id) — dropping any line whose eid (or, for cleanup
//     and proposal-epic creates, whose fold entry) the journal already
//     carries. Only once the batch is complete does it assert
//     invariants 1, 2 and 5 over journal-plus-batch (invariant 3 —
//     re-running appends nothing — falls out of the dedup construction
//     itself; invariant 4 — snapshot saved iff complete — is
//     SnapshotSaver's gate), then commits the append atomically.
//  3. SnapshotSaver.Save: gate on status — if complete, build the
//     merkle tree and atomically write spec/.snapshot.json; if partial,
//     leave the snapshot untouched so the next emit recomputes against
//     the unchanged baseline.
//  4. Emit a JSON Summary to stdout.
//
// # Mode: refresh
//
// Absorbs content-only drift on non-bead-producing leaves without bead
// lifecycle. The changeset and receipts must be empty; the pre-refresh
// snapshot is the diff baseline and must exist:
//
//  1. Pre-flight: both artifacts empty, snapshot present.
//  2. Compute the diff: rebuild the current tree, load the snapshot,
//     run the merkle diff.
//  3. Refusal gates: any added or removed entry, or any orphan
//     bead-map record, refuses the run (typed RefreshRefusal) with
//     both files byte-identical to their pre-call state.
//  4. Update stale record spec_hash fields in memory — no other field,
//     no counter advance.
//  5. Atomic paired commit: .bead-map.json and spec/.snapshot.json
//     move together; a snapshot-write failure rolls the bead-map back.
//  6. Emit a JSON RefreshSummary to stdout.
//
// # Partial-run recovery (mode: normal)
//
// A partial top-level status means some ops succeeded and some did not.
// The journal reflects the partial state (events and receipts for ok
// ops, nothing for error ops). The snapshot is intentionally NOT
// written so the next `spex emit` diffs the spec against the original
// baseline and resurfaces the unfinished ops through the idempotency
// path. This is the "unfinished operations resurface" mechanism.
//
// # Atomic writes
//
// spec/.history.jsonl (normal mode) and .bead-map.json plus
// spec/.snapshot.json (refresh mode) are written via temp-file + rename.
// A crash mid-write leaves the pre-write file intact. Refresh mode is
// stricter than normal mode: its two writes form one commit boundary,
// because the refreshed snapshot IS the next run's diff baseline.
//
// # Exit codes
//
// 0 success; 1 input error (bad flags, malformed JSON, op_id mismatch,
// snapshot write failure, missing pre-refresh snapshot, non-empty
// refresh artifacts); 2 invariant failure or refresh refusal (mapping
// file unchanged on disk in both cases).
//
// # Contract surface
//
// This package is intentionally minimal: the wire-format Summary and
// RefreshSummary types (the JSON written to stdout), the close-reason
// prefix vocabulary that discriminates remove vs modify at the
// reconciler boundary, the RefreshRefusal error contract, and the CLI
// exit-code constants. Component-level behavior — Reconciler,
// SnapshotSaver, RefreshHandler, IngestCommand — is owned by the
// per-component beads.
package ingest
