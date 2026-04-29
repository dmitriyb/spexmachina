package emit

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/mapping"
)

// Labeler reserves spex:<record-id> idempotency labels for new create ops.
//
// The record-id is the monotonic counter the mapping store would assign on
// its next Create. Labeler reads the counter once on the first Reserve and
// advances an in-memory cursor on each subsequent call within the same
// emit run. The persisted counter is NOT advanced — emit is pure; ingest
// commits the advance only after a successful create receipt is reconciled.
//
// Re-runnable: a failed emit run (e.g., cycle error before changeset is
// written) leaves the store untouched. The next attempt reserves the same
// label range.
type Labeler struct {
	MappingStore mapping.Store

	cursor      int
	initialized bool
}

// Reserve returns n sequential labels of the form spex:<record-id>,
// starting from the mapping store's persisted counter on the first call
// and from the in-memory cursor on subsequent calls. The persisted counter
// is not advanced.
//
// Reserve(0) returns an empty slice and initializes the cursor so callers
// can probe NextLabel after a no-op reservation. Negative n is rejected.
//
// Reserve is the bulk-allocation API. For per-action labelling that
// respects modify-pair record-id reuse and cleanup label format, use
// LabelFor — Reserve only allocates fresh sequential labels and does not
// know about action class.
func (l *Labeler) Reserve(n int) ([]string, error) {
	if n < 0 {
		return nil, fmt.Errorf("emit: idempotency labeler: negative count %d", n)
	}
	if err := l.ensureInit(); err != nil {
		return nil, err
	}

	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("spex:%d", l.cursor+i)
	}
	l.cursor += n
	return out, nil
}

// LabelFor returns the idempotency label for one create action,
// branching on action class per spec/emit/arch_idempotency_labeler.md:
//
//   - Cleanup actions (Reason starts with "Code cleanup:"):
//     return spex:cleanup-<action.SpecNodeID>. Cursor does NOT advance.
//   - Modify-pair actions (OldBeadID != "" and not a cleanup):
//     look up MappingStore.GetByBead(OldBeadID) and return
//     spex:<existing-rec.ID>. Cursor does NOT advance.
//   - Fresh creates: return spex:<cursor> and advance the cursor.
//
// Cleanup is checked before modify-pair because cleanup actions also
// carry OldBeadID (lineage) but get a different label format.
func (l *Labeler) LabelFor(action CreateAction) (string, error) {
	if err := l.ensureInit(); err != nil {
		return "", err
	}
	if action.IsCleanup() {
		return fmt.Sprintf("spex:cleanup-%s", action.SpecNodeID), nil
	}
	if action.OldBeadID != "" {
		rec, err := l.MappingStore.GetByBead(action.OldBeadID)
		if err != nil {
			return "", fmt.Errorf("emit: idempotency labeler: lookup record for %s: %w", action.OldBeadID, err)
		}
		return fmt.Sprintf("spex:%d", rec.ID), nil
	}
	label := fmt.Sprintf("spex:%d", l.cursor)
	l.cursor++
	return label, nil
}

// NextLabel returns the next record-id this Labeler would assign without
// consuming it. Initializes the cursor from the mapping store if it has
// not yet been read.
func (l *Labeler) NextLabel() (int, error) {
	if err := l.ensureInit(); err != nil {
		return 0, err
	}
	return l.cursor, nil
}

func (l *Labeler) ensureInit() error {
	if l.initialized {
		return nil
	}
	start, err := l.MappingStore.NextRecordID()
	if err != nil {
		return fmt.Errorf("emit: idempotency labeler: read counter: %w", err)
	}
	l.cursor = start
	l.initialized = true
	return nil
}
