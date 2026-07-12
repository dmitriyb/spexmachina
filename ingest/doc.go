// Package ingest is the pipeline tail: it moves the mapping store and
// the merkle snapshot together, in one of two modes dispatched by
// IngestCommand's --mode flag (see spec/ingest/flow_ingest.md). Both
// modes terminate with the same atomicity guarantee: snapshot and
// .bead-map.json represent the same point-in-time spec state, or
// neither moves.
//
// # Mode: normal (default)
//
// Reconciles the mapping store from a paired changeset.json (typed by
// emit) + receipts.json (typed by adapters). Pure function over local
// files — no subprocesses, no tracker calls, no git calls:
//
//  1. Pre-flight: parse both files, version == 1 check, op_id set
//     equality between changeset and receipts.
//  2. Reconciler.Apply: clone the mapping store in memory, apply per-op
//     transitions (insert on ok create, delete on ok close-on-removed,
//     update on the close+create modified-node pair), assert the
//     consistency invariants (1–5 and 7; invariant 6 — snapshot saved
//     iff complete — is SnapshotSaver's gate), atomically commit
//     .bead-map.json.
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
// The mapping store reflects the partial state (records for ok creates,
// no records for error creates). The snapshot is intentionally NOT
// written so the next `spex emit` diffs the spec against the original
// baseline and resurfaces the unfinished ops through the idempotency
// path. This is the "unfinished operations resurface" mechanism.
//
// # Atomic writes
//
// Both .bead-map.json and spec/.snapshot.json are written via temp-file
// + rename. A crash mid-write leaves the pre-write file intact. Refresh
// mode is stricter than normal mode: the two writes form one commit
// boundary, because the refreshed snapshot IS the next run's diff
// baseline.
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
