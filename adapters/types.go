package adapters

// ReceiptsVersion is the wire-format version of receipts.json. Bumped when
// the schema changes in a non-backwards-compatible way; ingest rejects
// any version it does not recognize. Version 2 renamed the per-op
// tracker id field from bead_id to task_id.
const ReceiptsVersion = 2

// Top-level status values on Receipts. The adapter sets Status to
// StatusComplete when every op produced ok or intentional-skipped, and
// StatusPartial when any op errored or the adapter exited mid-run.
// SnapshotSaver in ingest gates on this field: only StatusComplete
// triggers a snapshot write.
const (
	StatusComplete = "complete"
	StatusPartial  = "partial"
)

// Per-op status vocabulary. ok = the op executed and produced its intended
// effect, or was a no-op due to an idempotent create match (was_existing
// match on create — ingest still needs the journal line) or an idempotent
// close against an already-closed target; skipped = an op the adapter
// deliberately did nothing for — the reference adapter no longer produces
// this status, since its already-closed close now converges on ok; error =
// the op failed and no state was changed.
const (
	OpStatusOk      = "ok"
	OpStatusSkipped = "skipped"
	OpStatusError   = "error"
)

// Adapter label vocabulary. This literal is part of the adapter
// contract — plan and the reference adapter both rely on it, and ingest
// recognizes it when reconciling state. Close ops carry no labels: the
// reference adapter keys close idempotency on the tracker's own status.
//
//	IdempotencyLabelPrefix + <eid> on every create (plan)
const (
	IdempotencyLabelPrefix = "spex:"
)

// TaskStateVersion is the wire-format version of tasks.json, the
// task-state artifact plan reads through its required --tasks flag. plan
// refuses any version it does not recognize (spec/schema/arch_task_state_schema.md,
// "Versioned and refused, not tolerated").
const TaskStateVersion = 1

// Task status vocabulary on a TaskStateEntry. Exactly these two — there is
// no closed, no done, and no third status: a task the artifact does not
// list has no live work, and what that means for its node is plan's
// decision, never a status the artifact carries
// (spec/schema/arch_task_state_schema.md, "Why no closed status").
const (
	TaskStatusOpen       = "open"
	TaskStatusInProgress = "in_progress"
)

// TaskStateEntry is one in-flight task in tasks.json: the tracker's own id
// and its live status, the same task_id a receipt and a journal receipt
// carry, so the join onto a journal pairing is a string comparison.
type TaskStateEntry struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// TaskState is the v1 task-state artifact an adapter's export half derives
// from the tracker and plan's TaskReader reads via --tasks: in-flight
// tasks only, nothing more (spec/map/flow_task_mapping.md, "Data Shapes";
// spec/schema/arch_task_state_schema.md). An empty Tasks slice is a legal,
// explicit statement that nothing is in flight, not a degenerate case.
//
// This is the wire shape; plan.ReadTasks/ReadTasksBytes (TaskReader)
// parses and validates a --tasks document into plan.Task, mirroring
// TaskStateEntry's two fields. cmd/spex/plan.go now requires --tasks and
// no longer wires --beads at all (spexmachina-swvx.21); the pre-task-
// lifecycle Bead/ReadBeads/ReadBeadsBytes trio was removed in
// spexmachina-swvx.7 (BeadReader cleanup).
type TaskState struct {
	Version int              `json:"version"`
	Tasks   []TaskStateEntry `json:"tasks"`
}

// OpReceipt is the per-op record an adapter writes after attempting one
// op. Field order on this struct IS the canonical JSON field order — do
// not reorder. TaskID and WasExisting always serialize (no omitempty)
// because the v2 contract requires them on every entry; Reason and Error
// are optional (omitempty) and mutually exclusive in practice (Reason on
// skipped, Error on error).
type OpReceipt struct {
	OpID        string `json:"op_id"`
	Status      string `json:"status"`
	TaskID      string `json:"task_id"`
	WasExisting bool   `json:"was_existing"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Receipts is the v2 output schema an adapter writes after processing a
// changeset. Op ordering in Ops mirrors the input changeset's op ordering —
// ingest relies on this 1:1 alignment to pair each receipt with the
// originating op.
type Receipts struct {
	Version int         `json:"version"`
	Status  string      `json:"status"`
	Ops     []OpReceipt `json:"ops"`
}
