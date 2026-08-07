package impact

import (
	"github.com/dmitriyb/spexmachina/mapping"
)

// Action represents a classified impact action for a spec node.
// Only two action types exist: "create" and "obsolete".
type Action struct {
	Type           string   `json:"type"`                        // "create" or "obsolete"
	BeadID         string   `json:"bead_id,omitempty"`           // existing bead ID (for "obsolete"); empty for "create"
	Module         string   `json:"module"`                      // affected module
	Node           string   `json:"node"`                        // affected spec node name
	NodeType       string   `json:"node_type,omitempty"`         // spec node type (component, data_flow, test_section, etc.)
	SpecNodeID     string   `json:"spec_node_id,omitempty"`      // identity hash of the affected node — lookup key into the mapping store
	SpecHash       string   `json:"spec_hash,omitempty"`         // current merkle hash (for "create")
	OldBeadID      string   `json:"old_bead_id,omitempty"`       // predecessor bead ID (for "create" replacing an obsoleted bead)
	DepSpecNodeIDs []string `json:"dep_spec_node_ids,omitempty"` // identity hashes of spec nodes this action's bead should depend on — resolved to refs by emit
	ChangeType     string   `json:"change_type,omitempty"`       // "modified" or "removed" (set by classifier for obsolete actions)
	Reason         string   `json:"reason"`                      // human-readable explanation
}

// beadProducingTypes are the node types that may produce a bead when changed.
// test_section is additionally gated by the describes-length check performed
// inside ClassifyActions using the current module spec.
var beadProducingTypes = map[string]bool{
	"module":       true,
	"component":    true,
	"data_flow":    true,
	"test_section": true,
}

// TODO(bead:spexmachina-y0wc.24): ClassifyActions took its old-bead
// correlation from Match/Unmatched/Orphaned (NodeMatcher, mapping.Record),
// both retired by spexmachina-y0wc.19's migration of MappingStore onto the
// journal. Re-derive the state-transition table against the journal-era
// NodeMatcher output per spec/impact/arch_action_classifier.md and
// re-enable this file. Action, beadProducingTypes above, and the
// graph-walking helpers below are kept live since they carry no
// mapping.Record dependency and ReportGenerator (spexmachina-y0wc.25)
// already depends on Action.
//
// Original implementation, preserved for reference:
//
// import (
// 	"fmt"
// 	"sort"
//
// 	"github.com/dmitriyb/spexmachina/mapping"
// 	"github.com/dmitriyb/spexmachina/merkle"
// )
//
// // ClassifyActions applies the state transition table to NodeMatcher output.
// // Modified or unexpectedly-matched-added nodes produce obsolete+create pairs;
// // unmatched added/modified nodes produce a single create; orphans produce an
// // obsolete (plus a cleanup create for closed beads).
// //
// // For each create action, DepSpecNodeIDs is populated with identity hashes
// // from: the component's direct `uses` edges, the transitive `requires_module`
// // closure (with cycle detection), and data_flow add-ons (components appearing
// // in a same-batch data_flow's `uses` array gain the data_flow's identity hash).
// // Bead-ID resolution is NOT performed — emit's Resolver classifies each
// // spec_node_id into ref:op / ref:bead / ref:spec_node at emit time.
// //
// // The graph argument drives both the test_section describes-length gate and
// // the DepSpecNodeIDs collection. When the graph is nil or a module lookup
// // fails, test_section changes default to producing actions (safer fallback)
// // and DepSpecNodeIDs is left empty for affected creates.
// //
// // Results are sorted deterministically by (Type, Module, Node, BeadID).
// func ClassifyActions(graph mapping.SpecGraph, matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action {
// 	var actions []Action
//
// 	for _, m := range matches {
// 		nodeType := m.Change.NodeType
// 		newHash := m.Change.NewHash
// 		specNodeID := m.Change.Key
//
// 		switch m.Change.Type {
// 		case merkle.Added, merkle.Modified:
// 			// test_section with describes==1 is coupled to its single
// 			// component's feature bead. Obsolete the old bead (it exists,
// 			// since this is the matched branch) but do not create a new one —
// 			// the component bead will own the test going forward.
// 			coupled := nodeType == "test_section" && !testSectionProducesBead(graph, m.Change.Module, specNodeID)
//
// 			for _, r := range m.Records {
// 				node := nodeName(r)
// 				// Obsolete the old bead.
// 				actions = append(actions, Action{
// 					Type:       "obsolete",
// 					BeadID:     r.BeadID,
// 					Module:     m.Change.Module,
// 					Node:       node,
// 					NodeType:   nodeType,
// 					SpecNodeID: r.SpecNodeID,
// 					ChangeType: "modified",
// 					Reason:     fmt.Sprintf("Spec node modified: %s/%s", m.Change.Module, node),
// 				})
// 				if coupled {
// 					continue
// 				}
// 				// Create a new replacement bead.
// 				actions = append(actions, Action{
// 					Type:       "create",
// 					Module:     m.Change.Module,
// 					Node:       node,
// 					NodeType:   nodeType,
// 					SpecNodeID: specNodeID,
// 					SpecHash:   newHash,
// 					OldBeadID:  r.BeadID,
// 					Reason:     fmt.Sprintf("Spec node modified (new): %s/%s", m.Change.Module, node),
// 				})
// 			}
// 		}
// 	}
//
// 	for _, u := range unmatched {
// 		nodeType := u.Change.NodeType
// 		node := resolveNodeName(graph, u.Change.Module, nodeType, u.Change.Key)
//
// 		// Only bead-trackable node types produce actions.
// 		if !beadProducingTypes[nodeType] {
// 			continue
// 		}
// 		// test_section gate: bundle single-describes sections into the
// 		// component feature bead.
// 		if nodeType == "test_section" && !testSectionProducesBead(graph, u.Change.Module, u.Change.Key) {
// 			continue
// 		}
//
// 		switch u.Change.Type {
// 		case merkle.Added:
// 			actions = append(actions, Action{
// 				Type:       "create",
// 				Module:     u.Change.Module,
// 				Node:       node,
// 				NodeType:   nodeType,
// 				SpecNodeID: u.Change.Key,
// 				SpecHash:   u.Change.NewHash,
// 				Reason:     fmt.Sprintf("New spec node: %s/%s", u.Change.Module, node),
// 			})
// 		case merkle.Modified:
// 			actions = append(actions, Action{
// 				Type:       "create",
// 				Module:     u.Change.Module,
// 				Node:       node,
// 				NodeType:   nodeType,
// 				SpecNodeID: u.Change.Key,
// 				SpecHash:   u.Change.NewHash,
// 				Reason:     fmt.Sprintf("Spec node modified (new): %s/%s", u.Change.Module, node),
// 			})
// 		}
// 		// Removed + no record = no action.
// 	}
//
// 	for _, o := range orphaned {
// 		node := nodeName(o.Record)
//
// 		// Always obsolete the orphaned bead.
// 		actions = append(actions, Action{
// 			Type:       "obsolete",
// 			BeadID:     o.Record.BeadID,
// 			Module:     o.Record.Module,
// 			Node:       node,
// 			NodeType:   o.NodeType,
// 			SpecNodeID: o.Record.SpecNodeID,
// 			ChangeType: "removed",
// 			Reason:     fmt.Sprintf("Spec node removed: %s/%s", o.Record.Module, node),
// 		})
//
// 		// If the bead is closed, code has shipped — create a cleanup bead.
// 		// OldBeadID carries the obsoleted bead so the downstream emitter
// 		// records --deps blocks:<old-bead-id>, giving the cleanup bead a
// 		// structural pointer back to what needs removing.
// 		if o.Record.BeadStatus == "closed" {
// 			actions = append(actions, Action{
// 				Type:       "create",
// 				Module:     o.Record.Module,
// 				Node:       node,
// 				NodeType:   o.NodeType,
// 				SpecNodeID: o.Record.SpecNodeID,
// 				OldBeadID:  o.Record.BeadID,
// 				Reason:     fmt.Sprintf("Code cleanup: %s/%s", o.Record.Module, node),
// 			})
// 		}
// 	}
//
// 	attachDepSpecNodeIDs(graph, actions)
//
// 	sort.Slice(actions, func(i, j int) bool {
// 		if actions[i].Type != actions[j].Type {
// 			return actions[i].Type < actions[j].Type
// 		}
// 		if actions[i].Module != actions[j].Module {
// 			return actions[i].Module < actions[j].Module
// 		}
// 		if actions[i].Node != actions[j].Node {
// 			return actions[i].Node < actions[j].Node
// 		}
// 		return actions[i].BeadID < actions[j].BeadID
// 	})
//
// 	return actions
// }
//
// // nodeName returns a human-readable name for the spec node from a mapping record.
// func nodeName(r mapping.Record) string {
// 	if r.Component != "" {
// 		return r.Component
// 	}
// 	return r.SpecNodeID
// }

// attachDepSpecNodeIDs populates DepSpecNodeIDs on create actions by walking
// the spec graph. Three sources contribute identity hashes:
//  1. The component's direct `uses` edges.
//  2. Every component in every module reachable by transitive `requires_module`
//     edges (cycle-safe).
//  3. Same-batch data_flow creates: if a data_flow action lists this
//     component's SpecNodeID in its graph-level `uses` array, the data_flow's
//     SpecNodeID is added as a dependency so emit can sequence the contract
//     op before the participating component ops.
//
// No mapping-store lookup, no bead-status filtering — emit's Resolver does
// that at emit time with full knowledge of the current batch's op_ids.
func attachDepSpecNodeIDs(graph mapping.SpecGraph, actions []Action) {
	if graph == nil {
		return
	}

	// Index same-batch data_flow creates by SpecNodeID for the add-on step.
	// Value is the data_flow's module name so we only need one graph lookup
	// per flow in the add-on loop below.
	type flowCreate struct {
		specNodeID string
		module     string
	}
	var flowCreates []flowCreate
	for _, a := range actions {
		if a.Type == "create" && a.NodeType == "data_flow" {
			flowCreates = append(flowCreates, flowCreate{specNodeID: a.SpecNodeID, module: a.Module})
		}
	}

	for i := range actions {
		a := &actions[i]
		if a.Type != "create" {
			continue
		}

		seen := make(map[string]bool)
		var deps []string
		addDep := func(id string) {
			if id != "" && id != a.SpecNodeID && !seen[id] {
				deps = append(deps, id)
				seen[id] = true
			}
		}

		if a.NodeType == "component" {
			mod, err := graph.ModuleByName(a.Module)
			if err == nil {
				var comp *mapping.ComponentInfo
				for ci := range mod.Components {
					if mod.Components[ci].ID == a.SpecNodeID {
						comp = &mod.Components[ci]
						break
					}
				}
				if comp != nil {
					for _, used := range comp.Uses {
						addDep(used)
					}
					visited := map[string]bool{}
					collectRequiresModule(graph, mod.ID, visited, addDep)
				}
			}

			// Data_flow add-ons: any same-batch data_flow create whose
			// graph-level uses array contains this component.
			for _, fc := range flowCreates {
				if fc.module != a.Module {
					continue
				}
				fmod, err := graph.ModuleByName(fc.module)
				if err != nil {
					continue
				}
				for _, flow := range fmod.DataFlows {
					if flow.ID != fc.specNodeID {
						continue
					}
					for _, participant := range flow.Uses {
						if participant == a.SpecNodeID {
							addDep(fc.specNodeID)
							break
						}
					}
				}
			}
		}

		a.DepSpecNodeIDs = deps
	}
}

// collectRequiresModule transitively walks requires_module edges, invoking
// add for every component identity hash found in each reachable module.
// Cycle-safe via visited set keyed by module identity hash.
func collectRequiresModule(graph mapping.SpecGraph, moduleID string, visited map[string]bool, add func(string)) {
	if visited[moduleID] {
		return
	}
	visited[moduleID] = true

	mod, err := graph.ModuleByID(moduleID)
	if err != nil {
		return
	}
	for _, reqID := range mod.RequiresModule {
		reqMod, err := graph.ModuleByID(reqID)
		if err != nil {
			continue
		}
		for _, comp := range reqMod.Components {
			add(comp.ID)
		}
		collectRequiresModule(graph, reqID, visited, add)
	}
}

// testSectionProducesBead returns true when the test_section identified by
// specNodeID in the named module has len(describes) >= 2 (a cross-component
// integration test). Returns true as a fallback when the graph is nil, the
// module cannot be resolved, or the test_section is not found — under those
// conditions the classifier cannot prove coupling so it preserves the action
// rather than drop it.
func testSectionProducesBead(graph mapping.SpecGraph, module, specNodeID string) bool {
	if graph == nil || module == "" {
		return true
	}
	mod, err := graph.ModuleByName(module)
	if err != nil {
		return true
	}
	for _, ts := range mod.TestSections {
		if ts.ID == specNodeID {
			return len(ts.Describes) >= 2
		}
	}
	return true
}

// resolveNodeName returns the human-readable name for a spec node referenced
// by identity hash, looked up in the current spec graph. Falls back to the
// identity hash when the graph is unavailable, the module cannot be resolved,
// or the node does not appear in the module — matches pre-lookup behavior and
// keeps Action.Node populated even when the graph is nil (existing callers
// may pass nil during early-boot tests).
func resolveNodeName(graph mapping.SpecGraph, module, nodeType, specNodeID string) string {
	if graph == nil || module == "" {
		return specNodeID
	}
	mod, err := graph.ModuleByName(module)
	if err != nil {
		return specNodeID
	}
	switch nodeType {
	case "component":
		for _, c := range mod.Components {
			if c.ID == specNodeID {
				return c.Name
			}
		}
	case "data_flow":
		for _, d := range mod.DataFlows {
			if d.ID == specNodeID {
				return d.Name
			}
		}
	case "test_section":
		for _, t := range mod.TestSections {
			if t.ID == specNodeID {
				return t.Name
			}
		}
	}
	return specNodeID
}
