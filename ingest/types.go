package ingest

// Close-reason prefix vocabulary. The reconciler discriminates close-op
// handling on these prefixes: a "Spec node removed" close closes the
// node's lineage with a removed event, which the fold reads as a
// tombstone; a "Spec node modified" close builds its own modified event
// and task_closed from the journal's live fold entry — see "Fold-Back
// Closes" in arch_event_builder.md. ActionClassifier emits these reasons
// in plan/action_classifier.go and ChangesetBuilder propagates them
// verbatim onto the close op's Reason field.
const (
	ReasonRemovedPrefix  = "Spec node removed"
	ReasonModifiedPrefix = "Spec node modified"
)

// Exit-code vocabulary for `spex ingest`. The CLI maps RunE errors to
// these codes: a sentinel invariant error produces ExitInvariant, any
// other error produces ExitInputError, success produces ExitOK.
//
//	ExitOK          success (complete OR partial with no reconciler errors)
//	ExitInputError  bad flags, malformed JSON, op_id mismatch, IO failure
//	ExitInvariant   AssertInvariants rejected the post-apply state;
//	                .bead-map.json on disk is unchanged
const (
	ExitOK         = 0
	ExitInputError = 1
	ExitInvariant  = 2
)

// Summary is the v1 stdout contract of `spex ingest` (mode: normal), per
// flow_ingest.md's "Summary output" shape. Field order on this struct IS
// the canonical JSON field order — do not reorder. SnapshotSaved and
// Status always serialize (no omitempty); the count fields are integers
// and naturally serialize as 0 when absent.
//
// Ok aggregates ok creates and ok closes — the per-op type breakdown
// stays inside the reconciler's internal summary and does not surface
// on the wire. EventsAppended and ReceiptsAppended are Reconciler's two
// append counts, zero on an idempotent re-run even when Ok is not. The
// receipts top-level status (complete | partial) echoes back into Status
// so callers can drive downstream behavior without re-reading
// receipts.json.
type Summary struct {
	Ok               int    `json:"ok"`
	Skipped          int    `json:"skipped"`
	Errors           int    `json:"errors"`
	EventsAppended   int    `json:"events_appended"`
	ReceiptsAppended int    `json:"receipts_appended"`
	SnapshotSaved    bool   `json:"snapshot_saved"`
	Status           string `json:"status"`
}
