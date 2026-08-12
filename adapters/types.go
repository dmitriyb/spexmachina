package adapters

// ReceiptsVersion is the wire-format version of receipts.json. Bumped when
// the schema changes in a non-backwards-compatible way; ingest rejects
// any version it does not recognize.
const ReceiptsVersion = 1

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
// match on create — ingest still needs the journal line); skipped = the op
// was a no-op because the tracker state was already-obsolete on close;
// error = the op failed and no state was changed.
const (
	OpStatusOk      = "ok"
	OpStatusSkipped = "skipped"
	OpStatusError   = "error"
)

// Adapter label vocabulary. These literals are part of the adapter
// contract — emit and the reference adapter both rely on them, and ingest
// recognizes them when reconciling state.
//
//	IdempotencyLabelPrefix + <record-id> on every create (emit)
//	ObsoleteLabel                       on every close   (adapter)
//	CommitLabelPrefix      + <git HEAD>  on every close   (adapter)
const (
	IdempotencyLabelPrefix = "spex:"
	ObsoleteLabel          = "spex:obsolete"
	CommitLabelPrefix      = "commit:"
)

// OpReceipt is the per-op record an adapter writes after attempting one
// op. Field order on this struct IS the canonical JSON field order — do
// not reorder. BeadID and WasExisting always serialize (no omitempty)
// because the v1 contract requires them on every entry; Reason and Error
// are optional (omitempty) and mutually exclusive in practice (Reason on
// skipped, Error on error).
type OpReceipt struct {
	OpID        string `json:"op_id"`
	Status      string `json:"status"`
	BeadID      string `json:"bead_id"`
	WasExisting bool   `json:"was_existing"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Receipts is the v1 output schema an adapter writes after processing a
// changeset. Op ordering in Ops mirrors the input changeset's op ordering —
// ingest relies on this 1:1 alignment to pair each receipt with the
// originating op.
type Receipts struct {
	Version int         `json:"version"`
	Status  string      `json:"status"`
	Ops     []OpReceipt `json:"ops"`
}
