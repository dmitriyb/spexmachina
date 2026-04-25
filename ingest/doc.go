// Package ingest reconciles the mapping store and saves the merkle
// snapshot from a paired changeset.json + receipts.json.
//
// ingest is a pure function over local files: it reads changeset.json
// (typed by emit) and receipts.json (typed by adapters), applies per-op
// state transitions to .bead-map.json, asserts seven consistency
// invariants, and writes spec/.snapshot.json iff the receipts top-level
// status is complete. No subprocesses, no tracker calls, no git calls.
//
// # Flow
//
// The pipeline within `spex ingest` is:
//
//  1. Pre-flight: parse both files, version == 1 check, op_id set
//     equality between changeset and receipts.
//  2. Reconciler.Apply: clone the mapping store in memory, apply per-op
//     transitions (insert on ok create, delete on ok close-on-removed,
//     update on the close+create modified-node pair), assert
//     invariants, atomically commit .bead-map.json.
//  3. SnapshotSaver.Save: gate on status — if complete, build the
//     merkle tree and atomically write spec/.snapshot.json; if partial,
//     leave the snapshot untouched so the next emit recomputes against
//     the unchanged baseline.
//  4. Emit a JSON Summary to stdout.
//
// # Partial-run recovery
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
// + rename. A crash mid-write leaves the pre-write file intact.
//
// # Exit codes
//
// 0 success (complete OR partial without reconciler errors), 1 input
// error (bad flags, malformed JSON, op_id mismatch, snapshot write
// failure), 2 invariant failure (mapping file unchanged on disk).
//
// # Contract surface
//
// This package is intentionally minimal: the wire-format Summary type
// (the JSON written to stdout), the close-reason prefix vocabulary that
// discriminates remove vs modify at the reconciler boundary, and the
// CLI exit-code constants. Component-level behavior — Reconciler,
// SnapshotSaver, IngestCommand — lives in per-component beads under
// spexmachina-0lk.
package ingest
