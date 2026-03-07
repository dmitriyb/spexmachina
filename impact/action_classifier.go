package impact

import (
	"fmt"
	"path"
	"sort"

	"github.com/dmitriyb/spexmachina/merkle"
)

// Action represents a classified impact action for a spec node.
type Action struct {
	Type   string // "create", "close", "review"
	BeadID string // existing bead ID (empty for "create")
	Module string // affected module
	Node   string // affected spec node (component/impl_section name)
	Impact string // impact level from merkle classification
	Reason string // human-readable explanation
}

// ClassifyActions applies the decision table to match results from NodeMatcher.
// Each Match, Unmatched, and Orphaned entry produces one or more Actions.
// Results are sorted deterministically by (Type, Module, Node, BeadID).
func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action {
	var actions []Action

	for _, m := range matches {
		impact := m.Change.Impact.String()
		node := nodeFromChange(m.Change)

		switch m.Change.Type {
		case merkle.Added, merkle.Modified:
			reason := matchedChangeReason(m.Change.Type, impact, m.Change.Module, node)
			for _, b := range m.Beads {
				actions = append(actions, Action{
					Type:   "review",
					BeadID: b.ID,
					Module: m.Change.Module,
					Node:   node,
					Impact: impact,
					Reason: reason,
				})
			}
		}
		// Removed + matched beads are handled as orphaned by NodeMatcher, not here.
	}

	for _, u := range unmatched {
		impact := u.Change.Impact.String()
		node := nodeFromChange(u.Change)

		switch u.Change.Type {
		case merkle.Added:
			actions = append(actions, Action{
				Type:   "create",
				Module: u.Change.Module,
				Node:   node,
				Impact: impact,
				Reason: fmt.Sprintf("New spec node: %s/%s", u.Change.Module, node),
			})
		case merkle.Modified:
			// Modified node with no bead — create.
			actions = append(actions, Action{
				Type:   "create",
				Module: u.Change.Module,
				Node:   node,
				Impact: impact,
				Reason: fmt.Sprintf("New spec node: %s/%s", u.Change.Module, node),
			})
		}
		// Removed + no bead = no action (nothing to close).
	}

	for _, o := range orphaned {
		actions = append(actions, Action{
			Type:   "close",
			BeadID: o.Bead.ID,
			Module: o.Bead.Module,
			Node:   beadNode(o.Bead),
			Reason: fmt.Sprintf("Spec node removed: %s/%s", o.Bead.Module, beadNode(o.Bead)),
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

// nodeFromChange extracts the spec node name from the change path base filename.
func nodeFromChange(c merkle.ClassifiedChange) string {
	return path.Base(c.Path)
}

// beadNode returns the best available node name from a bead's spec metadata.
func beadNode(b BeadSpec) string {
	if b.Component != "" {
		return b.Component
	}
	if b.ImplSection != "" {
		return b.ImplSection
	}
	return ""
}
