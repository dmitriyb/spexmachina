// Package adapters defines the receipts.json v2 contract that any spex
// adapter implementation must produce.
//
// Adapters live outside the spex binary. Each has two halves: an export
// half that derives tasks.json (the task-state artifact plan reads) from
// the tracker, and an apply half that consumes changeset.json (typed by
// the plan package), executes the listed operations against a task
// tracker (br, bd, GitHub Issues, Jira, …), and writes receipts.json.
// ingest then reads changeset.json and receipts.json to reconcile the
// task journal and gate the merkle snapshot write.
//
// # Flow
//
// scripts/apply-br.sh is the reference implementation of the apply half.
// There is no reference export half yet — TODO(bead:spexmachina-swvx.5):
// scripts/export-br.sh. apply-br.sh today:
//
//  1. Pre-flight: parse changeset.json, confirm the tracker CLI answers,
//     start with an empty op_id → task_id substitution table. It enforces
//     changeset version 3, matching plan.ChangesetVersion — the spec's v4
//     ref-shape rename (bead_id → task_id on plan.Ref) has not landed on
//     either side yet: TODO(bead:spexmachina-swvx.5) tracks the adapter's
//     half, TODO(bead:spexmachina-swvx.6) tracks plan's. The changeset's
//     top-level absorbed array is ingest's input, not read past the parse.
//  2. Per op in order: resolve parent/deps/target refs — today a "bead"
//     ref resolves as-is, an "op" ref resolves via the substitution
//     table — idempotency-check the tracker where the op kind supports
//     one, run the tracker-specific subcommand, append a per-op receipt;
//     a create additionally records its resolved task id in the
//     substitution table.
//  3. Emit receipts.json: derive the top-level status (complete vs
//     partial), assemble the v2 wrapper, atomic write.
//
// # Determinism and idempotency
//
// Re-running the same changeset against an unchanged tracker state must
// produce the same final tracker state. The create-idempotency probe is
// an optional adapter capability — a tracker without label support may
// omit it — but the reference adapter implements it: a task already
// carrying a create op's idempotency.label (an opaque, exact-match
// string) is recorded as was_existing=true status=ok rather than
// duplicated, so ingest still constructs its journal line. Close
// idempotency keys on the tracker's own status instead — close ops
// carry no labels. A retarget's two effects (label add, dep add) are
// naturally idempotent and need no probe at all.
//
// # Contract surface
//
// This package is intentionally minimal: the wire-format types
// (Receipts, OpReceipt) plus version and status constants. Component-
// level behavior — ref resolution, the substitution table, tracker
// subprocess invocation — lives in the bash reference adapter
// (spexmachina-swvx.5, BrReferenceAdapter) and in adapters written for
// other trackers, which consult this contract unchanged.
package adapters
