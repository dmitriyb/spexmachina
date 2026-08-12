// Package adapters defines the receipts.json v1 contract that any spex
// adapter implementation must produce.
//
// Adapters live outside the spex binary. They consume changeset.json
// (typed by the emit package), execute the listed operations against a
// bead tracker (br, bd, GitHub Issues, Jira, …), and write receipts.json.
// ingest then reads receipts.json to reconcile the mapping store and gate
// the merkle snapshot write.
//
// # Flow
//
// The reference adapter (scripts/apply-br.sh) implements this pipeline:
//
//  1. Pre-flight: parse changeset v1, br --version check, init the
//     op_id → bead_id substitution table.
//  2. For each op in order: resolve refs, idempotency-check the tracker,
//     run the appropriate subcommand, append a per-op receipt, update
//     the substitution table on successful creates.
//  3. Emit receipts.json: derive top-level status (complete vs partial),
//     assemble the v1 wrapper, atomic write.
//
// # Determinism and idempotency
//
// Re-running the same changeset against an unchanged tracker state must
// produce the same final tracker state. Already-created beads are
// detected via the IdempotencyLabelPrefix label and recorded as
// was_existing=true status=ok receipts, so ingest still constructs a
// journal line for them; already-closed beads are detected via the
// ObsoleteLabel and recorded as status=skipped with reason
// "already obsoleted".
//
// # Contract surface
//
// This package is intentionally minimal: the wire-format types
// (Receipts, OpReceipt) and the label vocabulary (IdempotencyLabelPrefix,
// ObsoleteLabel, CommitLabelPrefix) plus version and status constants.
// Component-level behavior — substitution table, ref resolution, br
// subprocess invocation — lives in the bash reference adapter and in
// per-component beads under spexmachina-0lk.1.
package adapters
