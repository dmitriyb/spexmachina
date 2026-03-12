package impact

import (
	"fmt"
	"sort"

	"github.com/dmitriyb/spexmachina/merkle"
)

// Action represents a classified impact action for a spec node.
type Action struct {
	Type   string `json:"type"`              // "create", "close", "review"
	BeadID string `json:"bead_id,omitempty"` // existing bead ID (empty for "create")
	Module string `json:"module"`            // affected module
	Node   string `json:"node"`              // affected spec node (spec node ID path)
	Impact string `json:"impact,omitempty"`  // impact level from merkle classification
	Reason string `json:"reason"`            // human-readable explanation
}

// ClassifyActions applies the decision table to match results from NodeMatcher.
// Each Match, Unmatched, and Orphaned entry produces one or more Actions.
// Results are sorted deterministically by (Type, Module, Node, BeadID).
func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action {
	var actions []Action

	for _, m := range matches {
		impact := m.Change.Impact.String()
		node := m.Change.Path

		switch m.Change.Type {
		case merkle.Added, merkle.Modified:
			reason := matchedChangeReason(m.Change.Type, impact, m.Change.Module, node)
			for _, r := range m.Records {
				actions = append(actions, Action{
					Type:   "review",
					BeadID: r.BeadID,
					Module: m.Change.Module,
					Node:   node,
					Impact: impact,
					Reason: reason,
				})
			}
		}
		// Removed + matched records are handled as orphaned by NodeMatcher, not here.
	}

	for _, u := range unmatched {
		impact := u.Change.Impact.String()
		node := u.Change.Path

		switch u.Change.Type {
		case merkle.Added, merkle.Modified:
			actions = append(actions, Action{
				Type:   "create",
				Module: u.Change.Module,
				Node:   node,
				Impact: impact,
				Reason: fmt.Sprintf("New spec node: %s/%s", u.Change.Module, node),
			})
		}
		// Removed + no record = no action (nothing to close).
	}

	for _, o := range orphaned {
		actions = append(actions, Action{
			Type:   "close",
			BeadID: o.Record.BeadID,
			Module: o.Record.Module,
			Node:   o.Record.SpecNodeID,
			Reason: fmt.Sprintf("Spec node removed: %s/%s", o.Record.Module, o.Record.SpecNodeID),
		})
	}

	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Type != actions[j].Type {
			return actions[i].Type < actions[j].Type
		}
		if actions[i].Module != actions[j].Module {
			return actions[i].Module < actions[j].Module
		}
		if actions[i].Node != actions[j].Node {
			return actions[i].Node < actions[j].Node
		}
		return actions[i].BeadID < actions[j].BeadID
	})

	return actions
}

// matchedChangeReason returns a human-readable reason for a matched change action.
func matchedChangeReason(changeType merkle.ChangeType, impact, module, node string) string {
	switch changeType {
	case merkle.Added:
		return fmt.Sprintf("Existing bead for added node (%s): %s/%s", impact, module, node)
	default:
		return fmt.Sprintf("Spec node modified (%s): %s/%s", impact, module, node)
	}
}
