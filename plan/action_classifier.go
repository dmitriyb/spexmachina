package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
)

// ClassifyActions turns NodeMatcher's three lists into a flat, ordered list
// of create, obsolete and retarget actions (spec/plan/arch_action_classifier.md).
// graph is the current spec directory, consulted for what the diff alone
// cannot answer: a test_section's current describes length, a component's
// uses edges, a module's requires_module edges, and a data_flow's uses
// edges.
//
// When a matched change's pairing is claimed (in_progress), the run refuses
// entirely: the error names every such task and no action list is returned
// — a partial classification never leaks. The claim check runs over every
// matched entry before any action is returned, so the error always names
// every claimed task at once, not just the first.
func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned, graph SpecGraph) ([]Action, error) {
	flowUses := changedDataFlowUses(matches, unmatched, graph)

	var actions []Action
	var claimed []string

	for _, m := range matches {
		acts, claims := classifyMatch(m, graph)
		actions = append(actions, acts...)
		claimed = append(claimed, claims...)
	}

	if len(claimed) > 0 {
		sort.Strings(claimed)
		return nil, fmt.Errorf("plan: classify: claimed task(s) refuse the run — node changed under: %s", strings.Join(claimed, ", "))
	}

	for _, u := range unmatched {
		if a, ok := classifyUnmatched(u, graph); ok {
			actions = append(actions, a)
		}
	}

	for _, o := range orphaned {
		actions = append(actions, classifyOrphaned(o)...)
	}

	applyDataFlowAddOn(actions, flowUses)
	sortActions(actions)

	return actions, nil
}

// classifyMatch applies the state transition table to every pairing a
// matched change carries. The test_section fold-back and the already-
// tracked cell are both checked before the open/in_progress/closed status
// split, in that order — fold-back first, because a section that no longer
// owes a task of its own never reaches a status split; already-tracked
// second, because it is the only cell within the split's scope that
// short-circuits it.
func classifyMatch(m Match, graph SpecGraph) (actions []Action, claimed []string) {
	change := m.Change
	for _, rec := range m.Records {
		if isTestSectionFoldback(change, graph) {
			actions = append(actions, obsoleteAction(change.Module, matchedNodeName(change, graph), change.NodeType, change.Key, rec.TaskID, "modified"))
			continue
		}

		if rec.After != "" && rec.After == change.NewHash {
			continue // already tracked: partial-run resurfacing, not new work — whatever the pairing's status
		}

		switch rec.BeadStatus {
		case "open":
			actions = append(actions, retargetAction(change, rec, graph))
		case "in_progress":
			claimed = append(claimed, rec.TaskID)
		default:
			// closed, or a status that never joined (no --beads file, or the
			// bead absent from the listing) — not known-open, so it takes
			// the direction that never moves a task silently.
			actions = append(actions, obsoleteAction(change.Module, matchedNodeName(change, graph), change.NodeType, change.Key, rec.TaskID, "modified"))
			actions = append(actions, createSuccessorAction(change, rec.TaskID, graph))
		}
	}
	return actions, claimed
}

// classifyUnmatched applies the node-type gate to an added or modified
// change with no existing pairing, admitting it as a fresh create or
// dropping it silently.
func classifyUnmatched(u Unmatched, graph SpecGraph) (Action, bool) {
	change := u.Change
	if !gateAdmits(change, graph) {
		return Action{}, false
	}

	name := nodeName(change.Module, change.Key, change.NodeType, graph)
	reason := fmt.Sprintf("New spec node: %s/%s", change.Module, name)
	if change.Type == merkle.Modified {
		reason = fmt.Sprintf("Spec node modified (new): %s/%s", change.Module, name)
	}

	return Action{
		Type:           ActionCreate,
		Module:         change.Module,
		Node:           name,
		NodeType:       change.NodeType,
		SpecNodeID:     change.Key,
		SpecHash:       change.NewHash,
		DepSpecNodeIDs: depsFor(change, graph),
		Reason:         reason,
	}, true
}

// classifyOrphaned always obsoletes the removed node's bead, and mints a
// cleanup create alongside it when the bead was closed — code shipped to
// main with no spec node left to answer for it.
func classifyOrphaned(o Orphaned) []Action {
	rec := o.Record
	obsolete := obsoleteAction(rec.Module, rec.Name, o.NodeType, rec.SpecNodeID, rec.TaskID, "removed")
	if rec.BeadStatus != "closed" {
		return []Action{obsolete}
	}

	cleanup := Action{
		Type:       ActionCreate,
		Module:     rec.Module,
		Node:       rec.Name,
		NodeType:   o.NodeType,
		SpecNodeID: rec.SpecNodeID,
		OldBeadID:  rec.TaskID,
		Reason:     fmt.Sprintf("Code cleanup: %s/%s", rec.Module, rec.Name),
	}
	return []Action{obsolete, cleanup}
}

// isTestSectionFoldback reports whether a matched test_section's current
// describes array has dropped to one component: its coverage folds back
// into that component's feature bead rather than owing a task of its own.
// Coupling that cannot be established (module or section not found in the
// current spec graph) is treated as still needing its own task, matching
// the node-type gate's "admitted rather than dropped" default.
func isTestSectionFoldback(change merkle.ClassifiedChange, graph SpecGraph) bool {
	if change.NodeType != "test_section" {
		return false
	}
	mod, ok := graph.moduleByName(change.Module)
	if !ok {
		return false
	}
	ts, ok := findTestSection(mod.Spec, change.Key)
	if !ok {
		return false
	}
	return len(ts.Describes) == 1
}

// gateAdmits applies the node-type gate to an unmatched change: the one
// path where node type decides whether a change produces an action at all
// (spec/plan/arch_action_classifier.md, "Node-Type Gate").
func gateAdmits(change merkle.ClassifiedChange, graph SpecGraph) bool {
	switch change.NodeType {
	case "component", "data_flow":
		return true
	case "module":
		// Admitted per the gate table, though no change ever carries this
		// node type — a module is an interior merkle node, never a leaf.
		return true
	case "test_section":
		mod, ok := graph.moduleByName(change.Module)
		if !ok {
			return true // coupling cannot be established: admitted
		}
		ts, ok := findTestSection(mod.Spec, change.Key)
		if !ok {
			return true
		}
		return len(ts.Describes) >= 2
	default: // api, meta, requirement
		return false
	}
}

// nodeName resolves an added or modified node's human-readable name from
// the current spec graph — the diff carries only the identity hash. Falls
// back to the identity hash itself if the graph cannot resolve it.
func nodeName(moduleName, nodeID, nodeType string, graph SpecGraph) string {
	mod, ok := graph.moduleByName(moduleName)
	if !ok {
		return nodeID
	}
	switch nodeType {
	case "component":
		if c, ok := findComponent(mod.Spec, nodeID); ok {
			return c.Name
		}
	case "data_flow":
		if f, ok := findDataFlow(mod.Spec, nodeID); ok {
			return f.Name
		}
	case "test_section":
		if ts, ok := findTestSection(mod.Spec, nodeID); ok {
			return ts.Name
		}
	}
	return nodeID
}

// matchedNodeName is nodeName for a matched change: the node still exists
// in the current spec (matched entries are never removed changes).
func matchedNodeName(change merkle.ClassifiedChange, graph SpecGraph) string {
	return nodeName(change.Module, change.Key, change.NodeType, graph)
}

// depsFor collects DepSpecNodeIDs for a create or retarget action. Only
// components carry uses/requires_module edges; data_flow and test_section
// actions collect nothing here (the data_flow add-on runs separately, and
// only ever targets components).
func depsFor(change merkle.ClassifiedChange, graph SpecGraph) []string {
	if change.NodeType != "component" {
		return nil
	}
	return collectDeps(change.Module, change.Key, graph)
}

// collectDeps walks a component's direct uses edges and its module's
// transitive requires_module edges, returning the deduplicated, sorted
// union of identity hashes.
func collectDeps(moduleName, nodeID string, graph SpecGraph) []string {
	mod, ok := graph.moduleByName(moduleName)
	if !ok {
		return nil
	}

	deps := map[string]bool{}

	if comp, ok := findComponent(mod.Spec, nodeID); ok {
		for _, u := range comp.Uses {
			if u != nodeID {
				deps[u] = true
			}
		}
	}

	visited := map[string]bool{mod.Module.ID: true}
	queue := append([]string(nil), mod.Module.RequiresModule...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true

		req, ok := graph.moduleByID(id)
		if !ok {
			continue
		}
		for _, c := range req.Spec.Components {
			deps[c.ID] = true
		}
		queue = append(queue, req.Module.RequiresModule...)
	}

	if len(deps) == 0 {
		return nil
	}
	out := make([]string, 0, len(deps))
	for id := range deps {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// changedDataFlowUses maps each component identity hash to the identity
// hashes of every changed (added or modified) data_flow in this batch that
// names it in its current uses array — the contract-layer add-on that lets
// TopologicalSorter place the data_flow's create first and the component
// gain a ref:op dep on it.
func changedDataFlowUses(matches []Match, unmatched []Unmatched, graph SpecGraph) map[string][]string {
	result := map[string][]string{}
	add := func(change merkle.ClassifiedChange) {
		if change.NodeType != "data_flow" {
			return
		}
		mod, ok := graph.moduleByName(change.Module)
		if !ok {
			return
		}
		flow, ok := findDataFlow(mod.Spec, change.Key)
		if !ok {
			return
		}
		for _, compID := range flow.Uses {
			result[compID] = append(result[compID], change.Key)
		}
	}
	for _, m := range matches {
		add(m.Change)
	}
	for _, u := range unmatched {
		add(u.Change)
	}
	return result
}

// applyDataFlowAddOn augments every component create action (never a
// retarget, never a cleanup — recognised by a non-empty SpecHash, since a
// cleanup's node no longer exists in the current spec) with the changed
// data_flows that name it.
func applyDataFlowAddOn(actions []Action, flowUses map[string][]string) {
	for i := range actions {
		a := &actions[i]
		if a.Type != ActionCreate || a.NodeType != "component" || a.SpecHash == "" {
			continue
		}
		flows := flowUses[a.SpecNodeID]
		if len(flows) == 0 {
			continue
		}
		deps := map[string]bool{}
		for _, d := range a.DepSpecNodeIDs {
			deps[d] = true
		}
		for _, f := range flows {
			deps[f] = true
		}
		out := make([]string, 0, len(deps))
		for d := range deps {
			out = append(out, d)
		}
		sort.Strings(out)
		a.DepSpecNodeIDs = out
	}
}

// obsoleteAction builds an obsolete action. changeType is "modified" for
// every matched-path obsolete (an added change matched to a differing hash
// behaves exactly as a modified one — see spec/plan/test_classification.md
// S3) or "removed" for an orphaned one.
func obsoleteAction(module, node, nodeType, specNodeID, beadID, changeType string) Action {
	reason := fmt.Sprintf("Spec node modified: %s/%s", module, node)
	if changeType == "removed" {
		reason = fmt.Sprintf("Spec node removed: %s/%s", module, node)
	}
	return Action{
		Type:       ActionObsolete,
		BeadID:     beadID,
		Module:     module,
		Node:       node,
		NodeType:   nodeType,
		SpecNodeID: specNodeID,
		ChangeType: changeType,
		Reason:     reason,
	}
}

// createSuccessorAction builds the create half of an obsolete+create pair.
func createSuccessorAction(change merkle.ClassifiedChange, oldBeadID string, graph SpecGraph) Action {
	name := matchedNodeName(change, graph)
	return Action{
		Type:           ActionCreate,
		Module:         change.Module,
		Node:           name,
		NodeType:       change.NodeType,
		SpecNodeID:     change.Key,
		SpecHash:       change.NewHash,
		OldBeadID:      oldBeadID,
		DepSpecNodeIDs: depsFor(change, graph),
		Reason:         fmt.Sprintf("Spec node modified (new): %s/%s", change.Module, name),
	}
}

// retargetAction builds the single action produced when a genuinely
// changed node's pairing is still open: the task's target moves, no bead
// is opened or closed.
func retargetAction(change merkle.ClassifiedChange, rec Pairing, graph SpecGraph) Action {
	name := matchedNodeName(change, graph)
	return Action{
		Type:           ActionRetarget,
		BeadID:         rec.TaskID,
		Module:         change.Module,
		Node:           name,
		NodeType:       change.NodeType,
		SpecNodeID:     change.Key,
		SpecHash:       change.NewHash,
		DepSpecNodeIDs: depsFor(change, graph),
		Reason:         fmt.Sprintf("Spec node modified (retarget): %s/%s", change.Module, name),
	}
}

// sortActions orders the final list deterministically by (Type, Module,
// Node, BeadID, SpecNodeID) so that the same diff and bead state always
// produce the same action list in the same order, regardless of input
// order.
func sortActions(actions []Action) {
	sort.SliceStable(actions, func(i, j int) bool {
		a, b := actions[i], actions[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.BeadID != b.BeadID {
			return a.BeadID < b.BeadID
		}
		return a.SpecNodeID < b.SpecNodeID
	})
}
