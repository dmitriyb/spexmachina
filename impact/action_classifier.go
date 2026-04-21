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
	NodeType   string   `json:"node_type,omitempty"`    // spec node type (component, data_flow, test_section, impl_section, etc.)
	SpecNodeID string   `json:"spec_node_id,omitempty"` // identity hash of the affected node — lookup key into the mapping store
	SpecHash   string   `json:"spec_hash,omitempty"`    // current merkle hash (for "create")
	OldBeadID  string   `json:"old_bead_id,omitempty"`  // predecessor bead ID (for "create" replacing an obsoleted bead)
	DepBeadIDs []string `json:"dep_bead_ids,omitempty"` // bead IDs this action depends on (from spec graph)
	ChangeType string   `json:"change_type,omitempty"`  // "modified" or "removed" (set by classifier for obsolete actions)
	Reason     string   `json:"reason"`                 // human-readable explanation
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

// ClassifyActions applies the state transition table to NodeMatcher output.
// Modified or unexpectedly-matched-added nodes produce obsolete+create pairs;
// unmatched added/modified nodes produce a single create; orphans produce an
// obsolete (plus a cleanup create for closed beads).
//
// The graph argument is used to gate test_section changes by the describes
// array length — a test_section with len(describes) == 1 is bundled into its
// single described component's feature bead and produces no action. When the
// graph is nil or the module lookup fails, test_section changes default to
// producing actions (the safer fallback). Other node types are gated purely
// by the beadProducingTypes set.
//
// Results are sorted deterministically by (Type, Module, Node, BeadID).
func ClassifyActions(graph mapping.SpecGraph, matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action {
	var actions []Action

	for _, m := range matches {
		nodeType := m.Change.NodeType
		newHash := m.Change.NewHash
		specNodeID := m.Change.Path

		switch m.Change.Type {
		case merkle.Added, merkle.Modified:
			// test_section with describes==1 is coupled to its single
			// component's feature bead. Obsolete the old bead (it exists,
			// since this is the matched branch) but do not create a new one —
			// the component bead will own the test going forward.
			coupled := nodeType == "test_section" && !testSectionProducesBead(graph, m.Change.Module, specNodeID)

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
				if coupled {
					continue
				}
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
		if !beadProducingTypes[nodeType] {
			continue
		}
		// test_section gate: bundle single-describes sections into the
		// component feature bead.
		if nodeType == "test_section" && !testSectionProducesBead(graph, u.Change.Module, u.Change.Path) {
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
		// OldBeadID carries the obsoleted bead so BeadCreator emits
		// --deps blocks:<old-bead-id>, giving the cleanup bead a structural
		// pointer back to what needs removing.
		if o.Record.BeadStatus == "closed" {
			actions = append(actions, Action{
				Type:       "create",
				Module:     o.Record.Module,
				Node:       node,
				NodeType:   o.NodeType,
				SpecNodeID: o.Record.SpecNodeID,
				OldBeadID:  o.Record.BeadID,
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

// ResolveDeps resolves spec-graph dependencies for a create action. Three
// edge sources contribute to DepBeadIDs:
//   - component `uses` edges (direct, intra-module)
//   - module `requires_module` edges (transitive, inter-module)
//   - data_flow participation: for each data_flow in the component's module
//     whose `uses` array contains the component's identity hash, the data_flow
//     bead is added as a dependency so apply runs the contract bead first.
//
// Closed beads are skipped. Returns nil for non-component actions or when the
// action's spec node cannot be located in the graph.
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

	addDep := func(id string) {
		if id != "" && !seen[id] {
			deps = append(deps, id)
			seen[id] = true
		}
	}

	// 1. Direct component `uses` edges (NOT transitive).
	for _, usedHash := range comp.Uses {
		if r, ok := recordIdx[usedHash]; ok && r.BeadStatus != "closed" {
			addDep(r.BeadID)
		}
	}

	// 2. Data_flow participation: any data_flow in the owning module that
	//    lists this component in its uses array becomes a dependency — the
	//    contract bead must be complete before participating component beads
	//    gain their --deps depends: wiring via apply's topological order.
	for _, flow := range mod.DataFlows {
		for _, participant := range flow.Uses {
			if participant != action.SpecNodeID {
				continue
			}
			if r, ok := recordIdx[flow.ID]; ok && r.BeadStatus != "closed" {
				addDep(r.BeadID)
			}
			break
		}
	}

	// 3. `requires_module` edges (transitive, with cycle detection).
	visited := make(map[string]bool)
	for _, depID := range resolveModuleDeps(graph, recordIdx, mod.ID, visited) {
		addDep(depID)
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
