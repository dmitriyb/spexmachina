package impact

import (
	"fmt"
	"sort"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Action represents a classified impact action for a spec node.
// Only two action types exist: "create" and "obsolete".
type Action struct {
	Type       string   `json:"type"`                   // "create" or "obsolete"
	BeadID     string   `json:"bead_id,omitempty"`      // existing bead ID (for "obsolete"); empty for "create"
	Module     string   `json:"module"`                 // affected module
	Node       string   `json:"node"`                   // affected spec node name
	NodeType   string   `json:"node_type,omitempty"`    // spec node type (component, impl_section, etc.)
	SpecNodeID string   `json:"spec_node_id,omitempty"` // identity hash of the affected node — lookup key into the mapping store
	SpecHash   string   `json:"spec_hash,omitempty"`    // current merkle hash (for "create")
	OldBeadID  string   `json:"old_bead_id,omitempty"`  // predecessor bead ID (for "create" replacing an obsoleted bead)
	DepBeadIDs []string `json:"dep_bead_ids,omitempty"` // bead IDs this action depends on (from spec graph)
	ChangeType string   `json:"change_type,omitempty"`  // "modified" or "removed" (set by classifier for obsolete actions)
	Reason     string   `json:"reason"`                 // human-readable explanation
}

// ClassifyActions applies the state transition table to match results from NodeMatcher.
// Modified/added matched nodes produce obsolete+create pairs. Unmatched added/modified
// nodes produce creates. Orphaned records produce obsoletes (with cleanup creates for
// closed beads). Results are sorted deterministically by (Type, Module, Node, BeadID).
func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action {
	var actions []Action

	for _, m := range matches {
		nodeType := m.Change.NodeType
		newHash := m.Change.NewHash
		specNodeID := m.Change.Path

		switch m.Change.Type {
		case merkle.Added, merkle.Modified:
			for _, r := range m.Records {
				node := nodeName(r)
				// Obsolete the old bead.
				actions = append(actions, Action{
					Type:       "obsolete",
					BeadID:     r.BeadID,
					Module:     m.Change.Module,
					Node:       node,
					NodeType:   nodeType,
					SpecNodeID: r.SpecNodeID,
					ChangeType: "modified",
					Reason:     fmt.Sprintf("Spec node modified: %s/%s", m.Change.Module, node),
				})
				// Create a new replacement bead.
				actions = append(actions, Action{
					Type:       "create",
					Module:     m.Change.Module,
					Node:       node,
					NodeType:   nodeType,
					SpecNodeID: specNodeID,
					SpecHash:   newHash,
					OldBeadID:  r.BeadID,
					Reason:     fmt.Sprintf("Spec node modified (new): %s/%s", m.Change.Module, node),
				})
			}
		}
	}

	for _, u := range unmatched {
		nodeType := u.Change.NodeType
		node := u.Change.Path

		// Only bead-trackable node types produce actions.
		if nodeType != "module" && nodeType != "component" && nodeType != "test_section" {
			continue
		}

		switch u.Change.Type {
		case merkle.Added:
			actions = append(actions, Action{
				Type:       "create",
				Module:     u.Change.Module,
				Node:       node,
				NodeType:   nodeType,
				SpecNodeID: u.Change.Path,
				SpecHash:   u.Change.NewHash,
				Reason:     fmt.Sprintf("New spec node: %s/%s", u.Change.Module, node),
			})
		case merkle.Modified:
			actions = append(actions, Action{
				Type:       "create",
				Module:     u.Change.Module,
				Node:       node,
				NodeType:   nodeType,
				SpecNodeID: u.Change.Path,
				SpecHash:   u.Change.NewHash,
				Reason:     fmt.Sprintf("Spec node modified (new): %s/%s", u.Change.Module, node),
			})
		}
		// Removed + no record = no action.
	}

	for _, o := range orphaned {
		node := nodeName(o.Record)

		// Always obsolete the orphaned bead.
		actions = append(actions, Action{
			Type:       "obsolete",
			BeadID:     o.Record.BeadID,
			Module:     o.Record.Module,
			Node:       node,
			NodeType:   o.NodeType,
			SpecNodeID: o.Record.SpecNodeID,
			ChangeType: "removed",
			Reason:     fmt.Sprintf("Spec node removed: %s/%s", o.Record.Module, node),
		})

		// If the bead is closed, code has shipped — create a cleanup bead.
		if o.Record.BeadStatus == "closed" {
			actions = append(actions, Action{
				Type:       "create",
				Module:     o.Record.Module,
				Node:       node,
				NodeType:   o.NodeType,
				SpecNodeID: o.Record.SpecNodeID,
				Reason:     fmt.Sprintf("Code cleanup: %s/%s", o.Record.Module, node),
			})
		}
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

// ResolveDeps resolves spec-graph dependencies for a component create action.
// It walks component `uses` edges (direct only) and module `requires_module`
// edges (transitive), looking up identity hashes directly against mapping
// records. Closed beads are skipped. Returns nil for non-component actions
// or when the action's spec node cannot be located in the graph.
func ResolveDeps(graph mapping.SpecGraph, records []mapping.Record, action Action) []string {
	if action.NodeType != "component" || action.SpecNodeID == "" {
		return nil
	}

	mod, err := graph.ModuleByName(action.Module)
	if err != nil {
		return nil
	}

	// Find the component in the spec graph by identity hash.
	var comp *mapping.ComponentInfo
	for i := range mod.Components {
		if mod.Components[i].ID == action.SpecNodeID {
			comp = &mod.Components[i]
			break
		}
	}
	if comp == nil {
		return nil
	}

	// Index records by SpecNodeID (identity hash) for direct lookup.
	recordIdx := make(map[string]mapping.Record, len(records))
	for _, r := range records {
		recordIdx[r.SpecNodeID] = r
	}

	seen := make(map[string]bool)
	var deps []string

	// 1. Resolve direct component `uses` edges (NOT transitive).
	for _, usedHash := range comp.Uses {
		if r, ok := recordIdx[usedHash]; ok && r.BeadStatus != "closed" {
			if !seen[r.BeadID] {
				deps = append(deps, r.BeadID)
				seen[r.BeadID] = true
			}
		}
	}

	// 2. Resolve `requires_module` edges (transitive, with cycle detection).
	visited := make(map[string]bool)
	moduleDeps := resolveModuleDeps(graph, recordIdx, mod.ID, visited)
	for _, depID := range moduleDeps {
		if !seen[depID] {
			deps = append(deps, depID)
			seen[depID] = true
		}
	}

	return deps
}

// resolveModuleDeps transitively walks requires_module edges, collecting
// open bead IDs from each required module's components. Components are
// looked up directly by their identity hash against the record index.
func resolveModuleDeps(graph mapping.SpecGraph, recordIdx map[string]mapping.Record, moduleID string, visited map[string]bool) []string {
	if visited[moduleID] {
		return nil
	}
	visited[moduleID] = true

	mod, err := graph.ModuleByID(moduleID)
	if err != nil {
		return nil
	}

	var deps []string
	for _, reqID := range mod.RequiresModule {
		reqMod, err := graph.ModuleByID(reqID)
		if err != nil {
			continue
		}

		// Collect open component beads in the required module by direct hash lookup.
		for _, comp := range reqMod.Components {
			if r, ok := recordIdx[comp.ID]; ok && r.BeadStatus != "closed" {
				deps = append(deps, r.BeadID)
			}
		}

		// Recurse into transitive dependencies.
		deps = append(deps, resolveModuleDeps(graph, recordIdx, reqID, visited)...)
	}

	return deps
}

// nodeName returns a human-readable name for the spec node from a mapping record.
func nodeName(r mapping.Record) string {
	if r.Component != "" {
		return r.Component
	}
	return r.SpecNodeID
}
