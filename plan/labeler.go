package plan

import "fmt"

// RemovalEntry is the journal fold's answer for one spec node's removal
// state: whether the node's latest journal state is a removal, and if so,
// the eid of the `removed` event that landed it.
type RemovalEntry struct {
	Removed bool
	EID     string
}

// RemovalLookup is IdempotencyLabeler's narrow view onto the journal fold —
// spec_node_id in, removal state out (spec/plan/arch_idempotency_labeler.md,
// "One rule, three referents"). It answers a different question than
// Resolver's FoldLookup, which pairs a node with its live task: a cleanup
// create's referent is the removal itself, so Labeler needs the fold's
// removed-event eid rather than a task id.
type RemovalLookup interface {
	Removal(specNodeID string) (RemovalEntry, bool)
}

// Labeler assigns each create (and retarget) op its idempotency label — the
// value the adapter matches against the tracker before creating, so a
// re-run of the same changeset re-attaches to the task the last run made
// instead of making a second one (spec/plan/arch_idempotency_labeler.md).
//
// One rule covers every action class: the label is spex:<eid> of the
// journal event the op's task_created will reference. LabelFor is a pure
// function of the action, the op_id TopologicalSorter/ChangesetBuilder
// assigned it, and — for a cleanup or an epic — one referent lookup: the
// fold's removed event for a cleanup answering a prior-batch removal, or
// the run's registration for an epic. No cursor, no counter, no store
// write: each call is independent of every other, and calling order never
// changes an answer.
//
// A retarget action's label carries the same derivation as a node-bearing
// create's — LabelFor does not need to know that the builder will place the
// result under Op.Labels rather than Op.Idempotency for a retarget; a
// retarget's Reason never carries the "Code cleanup:" prefix and its
// NodeType is never KindProposalEpic, so the node-bearing branch answers it
// unchanged.
type Labeler struct {
	// GitHead is this run's git HEAD SHA, embedded in every eid this
	// Labeler derives itself (node-bearing creates, retargets, and
	// same-batch-close cleanups). The epic and prior-batch-removal cleanup
	// branches use an eid read whole from Registration/Fold instead.
	GitHead string
	// Fold is the task journal fold, consulted only for a cleanup create's
	// removed event, when the removal already landed in an earlier batch.
	Fold RemovalLookup
	// CloseOpIDs maps a to-be-closed task_id to the op_id the builder
	// assigned its close op in this batch — consulted by cleanupLabel's
	// same-batch fallback for the modify-pair cleanup shape spexmachina-swvx.16
	// retired. A cleanup action carries no OldTaskID from this package's own
	// classifier output any more, so the lookup never matches in-process; it
	// stays only because CloseOpIDs and its plan/builder.go wiring are not
	// this bead's to retire (see cleanupLabel's TODO).
	CloseOpIDs map[string]string
}

// LabelFor returns the idempotency label for one create or retarget action,
// or an error if the action's referent event cannot be resolved
// deterministically.
//
// opID is the op_id this action was assigned in the ordered batch —
// node-bearing creates and retargets key their own eid from it. reg is the
// run's registration, consulted only for epic actions.
//
// Epic actions are checked first: their label derives from reg, not from
// opID or GitHead. An epic action for an unregistered proposal is an
// error — the fix is `spex register`, not a guessed label; this is the same
// verdict Resolver's missing-parent error reads, both decided on the run's
// registration.
//
// Cleanup is checked next, discriminated by isCleanup's Reason prefix
// rather than by OldTaskID — a cleanup action carries no OldTaskID any
// more (spexmachina-swvx.16), which is exactly why cleanupLabel's
// self-mint fallback exists for when neither the fold nor CloseOpIDs
// answers.
func (l *Labeler) LabelFor(action Action, opID string, reg Registration) (string, error) {
	if action.NodeType == KindProposalEpic {
		if reg.EID == "" {
			return "", fmt.Errorf("plan: label: proposal %q has no registered event in the journal; run `spex register` first", action.SpecNodeID)
		}
		return IdempotencyLabelPrefix + reg.EID, nil
	}
	if action.SpecNodeID == "" {
		return "", fmt.Errorf("plan: label: action has no spec_node_id to derive a label from")
	}
	if isCleanup(action) {
		return l.cleanupLabel(action, opID)
	}
	return IdempotencyLabelPrefix + DeriveEID(l.GitHead, opID), nil
}

// cleanupLabel resolves a cleanup create's referent: the fold's removed
// event for the node when the removal already landed in an earlier batch,
// else the removal this run's own same-batch close op implies, else — a
// removed node whose task was already absent from the artifact when this
// run first observed the removal, so no close accompanies the cleanup at
// all (spexmachina-swvx.16 retired that pairing per
// spec/plan/test_classification.md S4: "carries no old task id") — the
// removed event the cleanup op itself mints, from its own (git_head, op_id)
// exactly as any node-bearing create's (module.json's 885096d4941c). The
// fold is checked first — the same order the reconciler pairs the receipt
// by, so label and referent stay one fact whichever run the removal
// actually landed in.
//
// TODO(bead:spexmachina-swvx.13): the CloseOpIDs[action.OldTaskID] fallback
// is the pre-task-lifecycle obsolete-then-recreate shape's same-batch
// close op — a modify-pair cleanup's OldTaskID named the task
// ActionClassifier's obsolete path used to close in the same run. That
// path is retired (spexmachina-swvx.16), so a modify-pair cleanup create no
// longer exists and OldTaskID is always empty from this package's own
// classifier output; the branch is dead in-process but left in place since
// CloseOpIDs and its builder-side wiring (plan/builder.go) are this
// package's own concern to retire, not this bead's;
// IdempotencyLabeler's own bead is to drop it then.
func (l *Labeler) cleanupLabel(action Action, opID string) (string, error) {
	if l.Fold != nil {
		if entry, ok := l.Fold.Removal(action.SpecNodeID); ok && entry.Removed && entry.EID != "" {
			return IdempotencyLabelPrefix + entry.EID, nil
		}
	}
	if closeOpID, ok := l.CloseOpIDs[action.OldTaskID]; ok {
		return IdempotencyLabelPrefix + DeriveEID(l.GitHead, closeOpID), nil
	}
	return IdempotencyLabelPrefix + DeriveEID(l.GitHead, opID), nil
}

// isCleanup reports whether the action is a code-cleanup create — the same
// Reason-prefix discriminator ActionClassifier stamps it with
// (spec/plan/arch_action_classifier.md).
func isCleanup(a Action) bool {
	const prefix = "Code cleanup:"
	return len(a.Reason) >= len(prefix) && a.Reason[:len(prefix)] == prefix
}
