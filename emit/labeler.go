package emit

import "fmt"

// Labeler assigns each create action its idempotency.label — the value the
// adapter matches against the tracker before creating, so a re-run
// re-attaches to the task the last run made instead of duplicating it.
//
// Per spec/emit/arch_idempotency_labeler.md, LabelFor is a pure function of
// the action (plus, for an epic, the run's registration): no cursor, no
// store read, no state. Node-bearing creates (fresh and modify-pair alike)
// format spex:<spec_node_id> — the node's identity hash, stable across a
// modify pair since the hash does not change — while cleanup creates format
// spex:cleanup-<spec_node_id>, keyed by the removed node's identity hash so
// the cleanup task's label never collides with the node's own ordinary-task
// label. An epic create formats spex:<eid> of the proposal's registered
// event, read from the caller-supplied Registration — never from the fold,
// which only carries the epic once its task exists.
type Labeler struct{}

// LabelFor returns the idempotency label for one create action, or an error
// if the action cannot be labeled deterministically.
//
// Epic actions are checked first: their label derives from reg, not the
// action's own SpecNodeID. An epic action for an unregistered proposal is
// an error — the fix is `spex register`, not a guessed label; this is the
// same verdict Resolver's missing-parent error reads, both decided on the
// run's registration.
//
// Cleanup is checked next: a cleanup action also carries OldBeadID
// (lineage to the closed task it dismantles), which would otherwise be
// indistinguishable from a modify-pair's node-bearing shape.
func (l *Labeler) LabelFor(action CreateAction, reg Registration) (string, error) {
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
		return fmt.Sprintf("spex:cleanup-%s", action.SpecNodeID), nil
	}
	return fmt.Sprintf("spex:%s", action.SpecNodeID), nil
}
