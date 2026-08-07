package emit

// TODO(bead:spexmachina-y0wc.27): Labeler read the mapping-store counter
// (mapping.Store.NextRecordID/GetByBead), both retired by
// spexmachina-y0wc.19's migration of MappingStore onto the journal — the
// new MappingStore is read-only and has no counter (labels are
// spex:<spec_node_id>, not spex:<int>). Re-derive idempotency labelling
// per spec/emit/arch_idempotency_labeler.md and re-enable this file.
//
// Original implementation, preserved for reference:
//
// import (
// 	"fmt"
//
// 	"github.com/dmitriyb/spexmachina/mapping"
// )
//
// // Labeler assigns spex:<record-id> idempotency labels for new create ops.
// //
// // The record-id is the monotonic counter the mapping store would assign on
// // its next Create. Labeler reads the counter once on the first LabelFor and
// // advances an in-memory cursor on each subsequent fresh create within the
// // same emit run. The persisted counter is NOT advanced — emit is pure;
// // ingest commits the advance only after a successful create receipt is
// // reconciled.
// //
// // Re-runnable: a failed emit run (e.g., cycle error before changeset is
// // written) leaves the store untouched. The next attempt assigns the same
// // label range.
// type Labeler struct {
// 	MappingStore mapping.Store
//
// 	cursor      int
// 	initialized bool
// }
//
// // LabelFor returns the idempotency label for one create action,
// // branching on action class per spec/emit/arch_idempotency_labeler.md:
// //
// //   - Cleanup actions (Reason starts with "Code cleanup:"):
// //     return spex:cleanup-<action.SpecNodeID>. Cursor does NOT advance.
// //   - Modify-pair actions (OldBeadID != "" and not a cleanup):
// //     look up MappingStore.GetByBead(OldBeadID) and return
// //     spex:<existing-rec.ID>. Cursor does NOT advance.
// //   - Fresh creates: return spex:<cursor> and advance the cursor.
// //
// // Cleanup is checked before modify-pair because cleanup actions also
// // carry OldBeadID (lineage) but get a different label format.
// func (l *Labeler) LabelFor(action CreateAction) (string, error) {
// 	if err := l.ensureInit(); err != nil {
// 		return "", err
// 	}
// 	if action.IsCleanup() {
// 		return fmt.Sprintf("spex:cleanup-%s", action.SpecNodeID), nil
// 	}
// 	if action.OldBeadID != "" {
// 		rec, err := l.MappingStore.GetByBead(action.OldBeadID)
// 		if err != nil {
// 			return "", fmt.Errorf("emit: idempotency labeler: lookup record for %s: %w", action.OldBeadID, err)
// 		}
// 		return fmt.Sprintf("spex:%d", rec.ID), nil
// 	}
// 	label := fmt.Sprintf("spex:%d", l.cursor)
// 	l.cursor++
// 	return label, nil
// }
//
// // NextLabel returns the next record-id this Labeler would assign without
// // consuming it. Initializes the cursor from the mapping store if it has
// // not yet been read.
// func (l *Labeler) NextLabel() (int, error) {
// 	if err := l.ensureInit(); err != nil {
// 		return 0, err
// 	}
// 	return l.cursor, nil
// }
//
// func (l *Labeler) ensureInit() error {
// 	if l.initialized {
// 		return nil
// 	}
// 	start, err := l.MappingStore.NextRecordID()
// 	if err != nil {
// 		return fmt.Errorf("emit: idempotency labeler: read counter: %w", err)
// 	}
// 	l.cursor = start
// 	l.initialized = true
// 	return nil
// }
