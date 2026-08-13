package emit

import "fmt"

// Labeler assigns each create op its idempotency.label — the value the
// adapter matches against the tracker before creating, so a re-run
// re-attaches to the task the last run made instead of duplicating it.
//
// Per spec/emit/arch_idempotency_labeler.md, LabelFor answers one label at
// a time and is a pure function of the action, the op_id Sorter/Builder
// assigned it, and — for a cleanup or an epic — one referent lookup: the
// fold's removed event for a cleanup answering a prior-batch removal, or
// the run's registration for an epic. No cursor, no counter, no store
// write: each call is independent of every other, and calling order never
// changes an answer.
//
// One rule covers every action class: the label is spex:<eid> of the
// journal event the op's task_created will reference.
//   - Node-bearing (fresh and modify-pair creates alike): the change event
//     ingest will mint, eid derived from (git_head, this op's own op_id).
//   - Cleanup: the removed event the cleanup answers — the fold's latest
//     removed event for the node when the removal already landed, else
//     the event the same-batch close op implies, eid derived from the
//     close op's (git_head, op_id). This is the same resolution order the
//     reconciler pairs the receipt by, so label and referent stay one fact.
//   - Epic: the proposal's registered event, read from the run's
//     Registration — never from the fold, which only carries the epic
//     once its task exists.
type Labeler struct {
	// GitHead is this run's git HEAD SHA, embedded in every eid this
	// Labeler derives itself (node-bearing creates and same-batch-close
	// cleanups). The epic and prior-batch-removal cleanup branches use an
	// eid read whole from Registration/Fold instead.
	GitHead string
	// Fold is the task journal fold, consulted only for a cleanup
	// create's removed event, when the removal already landed in an
	// earlier batch.
	Fold JournalFold
	// CloseOpIDs maps a to-be-closed bead_id to the op_id Builder assigned
	// its close op in this batch. A cleanup action's OldBeadID indexes
	// into it when the fold carries no removed event yet — the removal
	// this run's own same-batch close op is about to record.
	CloseOpIDs map[string]string
}

// LabelFor returns the idempotency label for one create action, or an
// error if the action's referent event cannot be resolved deterministically.
//
// opID is the op_id Sorter/Builder assigned this action in the ordered
// batch — node-bearing creates key their own eid from it. reg is the run's
// registration, consulted only for epic actions.
//
// Epic actions are checked first: their label derives from reg, not from
// opID or GitHead. An epic action for an unregistered proposal is an
// error — the fix is `spex register`, not a guessed label; this is the
// same verdict Resolver's missing-parent error reads, both decided on the
// run's registration.
//
// Cleanup is checked next: a cleanup action also carries OldBeadID
// (lineage to the closed task it dismantles), which would otherwise be
// indistinguishable from a modify-pair's node-bearing shape.
func (l *Labeler) LabelFor(action CreateAction, opID string, reg Registration) (string, error) {
	if action.NodeType == "proposal" {
		if !reg.OK {
			return "", fmt.Errorf("emit: idempotency labeler: proposal %q has no registered event in the journal; run `spex register` first", action.SpecNodeID)
		}
		return fmt.Sprintf("spex:%s", reg.EID), nil
	}
	if action.SpecNodeID == "" {
		return "", fmt.Errorf("emit: idempotency labeler: action has no spec_node_id")
	}
	if action.IsCleanup() {
		return l.cleanupLabel(action)
	}
	return fmt.Sprintf("spex:%s:%s", l.GitHead, opID), nil
}

// cleanupLabel resolves a cleanup create's referent: the fold's removed
// event for the node when the removal already landed in an earlier batch,
// else the removal this run's own same-batch close op implies. The fold is
// checked first — the same order the reconciler pairs the receipt by, so
// label and referent stay one fact whichever run the removal actually
// landed in.
func (l *Labeler) cleanupLabel(action CreateAction) (string, error) {
	if entry, ok := l.Fold.Entry(action.SpecNodeID); ok && entry.Removed && entry.RemovedEID != "" {
		return fmt.Sprintf("spex:%s", entry.RemovedEID), nil
	}
	closeOpID, ok := l.CloseOpIDs[action.OldBeadID]
	if !ok {
		return "", fmt.Errorf("emit: idempotency labeler: cleanup for spec_node_id %q has no same-batch close op for old bead %q and no removed event in the journal fold", action.SpecNodeID, action.OldBeadID)
	}
	return fmt.Sprintf("spex:%s:%s", l.GitHead, closeOpID), nil
}
