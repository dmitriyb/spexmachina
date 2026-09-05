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
// plan) + receipts.json (typed by adapters). Pure function over local
// files — no subprocesses, no tracker calls, no git calls:
//
//  1. Pre-flight: parse both files, changeset version ==
//     plan.ChangesetVersion / receipts version == adapters.ReceiptsVersion
//     check, op_id set equality between changeset and receipts.
//  2. Reconciler.Apply: assembles the per-run state and constructs one
//     EventBuilder, dispatching every op (and the changeset's absorbed
//     entries) to it — change events and task receipts, with event ids
//     derived from (git_head, op_id) — dropping any line whose eid the
//     journal or in-flight batch already carries. Only once the batch is
//     complete does Reconciler run InvariantChecker over
//     journal-plus-batch (invariants 1 and 2; invariant 3 — re-running
//     appends nothing — falls out of the dedup construction itself;
//     invariant 4 — snapshot saved iff complete — is SnapshotSaver's
//     gate) and JournalEncoder over each surviving line (invariant 5),
//     then commits the append atomically.
//  3. SnapshotSaver.Save: gate on status — if complete, build the
//     merkle tree and atomically write spec/.snapshot.json; if partial,
//     leave the snapshot untouched so the next spex plan recomputes
//     against the unchanged baseline.
//  4. Emit a JSON Summary to stdout.
//
// EventBuilder's construction paths derive added-vs-modified from the
// journal fold's latest change event per node, and a cleanup create
// mints its own removal event when the journal shows none — the
// pre-task-lifecycle "Modified-Node Pair" mechanism (a create's `blocks`
// dep naming the bead its paired close retired) is gone, and so is
// Reconciler's own batch-wide same-batch-removal pre-pass: no op's lines
// depend on another op in the batch, so there is nothing left to resolve
// up front. The changeset/receipts version check reads
// plan.ChangesetVersion and adapters.ReceiptsVersion symbolically rather
// than hardcoding either; ingest/reconciler_test.go covers the deeper v4
// behavior (lineage-free creates, unconditional fold-back closes)
// surviving Reconciler.Apply.
//
// # Mode: refresh
//
// Absorbs spec drift that owes no bead work — content edits to any
// leaf, plus additions and removals of the node types the resolved
// profile declares absorbable (requirements and apis in either
// direction, component removals, in the default profile) — without any
// bead lifecycle running. The changeset and receipts must be empty; the
// pre-refresh snapshot is the diff baseline and must exist:
//
//  1. Pre-flight: both artifacts empty, snapshot present.
//  2. Compute the diff: rebuild the current tree, load the snapshot,
//     run the merkle diff.
//  3. Refusal gates: any added or removed entry the absorbable set does
//     not cover, or any removed node whose journal fold still shows a
//     live (unclosed) task pairing, refuses the run (typed
//     RefreshRefusal) with both files byte-identical to their pre-call
//     state.
//  4. Construct one change event per absorbed drift entry — added,
//     modified or removed, before/after hashes taken straight off the
//     two trees — closed by one refresh receipt naming the batch's
//     event ids and stamped with --git-head when given. A refresh-born
//     event's eid derives from (node, before, after) rather than
//     (git_head, op_id), since no op stands behind it.
//  5. Atomic paired commit: spec/.history.jsonl and spec/.snapshot.json
//     move together; a snapshot-write failure rolls the journal append
//     back. A run that finds no diff entry at all writes neither file.
//  6. Emit a JSON RefreshSummary to stdout.
//
// # Partial-run recovery (mode: normal)
//
// A partial top-level status means some ops succeeded and some did not.
// The journal reflects the partial state (events and receipts for ok
// ops, nothing for error ops). The snapshot is intentionally NOT
// written so the next `spex plan` diffs the spec against the original
// baseline and resurfaces the unfinished ops through the idempotency
// path. This is the "unfinished operations resurface" mechanism.
//
// # Atomic writes
//
// spec/.history.jsonl and spec/.snapshot.json are both written via
// temp-file + rename, in both modes. A crash mid-write leaves the
// pre-write file intact. Refresh mode is stricter than normal mode: its
// two writes form one commit boundary, because the refreshed snapshot
// IS the next run's diff baseline.
//
// # Exit codes
//
// 0 success; 1 input error (bad flags, malformed JSON, op_id mismatch,
// snapshot write failure, missing pre-refresh snapshot, non-empty
// refresh artifacts); 2 invariant failure or refresh refusal (journal
// unchanged on disk in both cases).
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
//
// # Reconciler decomposition
//
// Normal-mode construction, invariant checking and line encoding —
// previously carried inline in Reconciler — are declared as three
// further components per spec/ingest/module.json: EventBuilder
// (construction, arch_event_builder.md), InvariantChecker (invariants
// 1 and 2, arch_invariant_checker.md) and JournalEncoder (encode plus
// schema validation — invariant 5, shared with RefreshHandler,
// arch_journal_encoder.md). EventBuilder's construction paths
// (event_builder.go) and InvariantChecker's invariants 1 and 2
// (invariant_checker.go) are extracted and independently tested, as is
// JournalEncoder's Encode/Validate (journal_encoder.go,
// spexmachina-ugrs.4) — the wire-shape types, checkInvariant5 and the
// compiled schema now live there, and checkInvariant5 (called by both
// Reconciler and RefreshHandler) delegates line-by-line to
// JournalEncoder.Validate rather than carrying its own copy, so both
// pathways already inherit the gate from the one component that owns
// it. Reconciler (reconciler.go, spexmachina-ugrs.5) assembles the
// per-run state, constructs one EventBuilder and dispatches every op to
// it, runs InvariantChecker over journal-plus-batch, then encodes and
// commits — it no longer carries its own copy of construction or
// invariant-1/2 logic.
package ingest
