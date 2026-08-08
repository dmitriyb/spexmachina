package emit

import "fmt"

// Labeler assigns each create action its idempotency.label — the value the
// adapter matches against the tracker before creating, so a re-run
// re-attaches to the task the last run made instead of duplicating it.
//
// Per spec/emit/arch_idempotency_labeler.md, LabelFor is a pure function of
// the action: no cursor, no store read, no state. Node-bearing creates
// (fresh and modify-pair alike) and epic creates share the
// spex:<spec_node_id> format — the node's identity hash for a node-bearing
// create (stable across a modify pair, since the hash does not change), the
// proposal slug for an epic (already carried as the epic action's own
// SpecNodeID) — while cleanup creates format spex:cleanup-<spec_node_id>,
// keyed by the removed node's identity hash so the cleanup task's label
// never collides with the node's own ordinary-task label.
type Labeler struct{}

// LabelFor returns the idempotency label for one create action, or an error
// if the action is too malformed to read a spec_node_id from.
//
// Cleanup is checked first: a cleanup action also carries OldBeadID
// (lineage to the closed task it dismantles), which would otherwise be
// indistinguishable from a modify-pair's node-bearing shape.
func (l *Labeler) LabelFor(action CreateAction) (string, error) {
	if action.SpecNodeID == "" {
		return "", fmt.Errorf("emit: idempotency labeler: action has no spec_node_id")
	}
	if action.IsCleanup() {
		return fmt.Sprintf("spex:cleanup-%s", action.SpecNodeID), nil
	}
	return fmt.Sprintf("spex:%s", action.SpecNodeID), nil
}
