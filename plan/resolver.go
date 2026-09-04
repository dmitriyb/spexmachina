package plan

import "fmt"

// FoldLookup is Resolver's narrow view onto the journal fold
// (spec/plan/arch_resolver.md, "Interface": "the fold is an equally narrow
// surface: node key in, latest task-bearing pairing out"). PlanCommand
// adapts the parsed journal fold onto this surface; tests substitute a
// stand-in. Lookup's key is a spec node's identity hash, or the proposal
// slug for the epic; of the returned Pairing, Resolver's dep and parent
// resolution consults only TaskID (and, for a dep, whether BeadStatus is
// "closed").
type FoldLookup interface {
	Lookup(key string) (Pairing, bool)
}

// Registration is the run's one fact about the proposal's lifecycle: the eid
// of its `registered` event, read from the journal's parsed events rather
// than the fold (spec/plan/arch_resolver.md, "Interface"). The zero value
// means the journal holds no registered event for this run's proposal.
type Registration struct {
	EID string
}

// ResolveDeps classifies each dep identity hash into a ref the adapter can
// apply blind, in the input order (spec/plan/arch_resolver.md, "The Two Ref
// Shapes" and "Determinism"). First match wins:
//
//  1. batch holds the id (another create op in this same run) -> ref:op.
//  2. Otherwise the fold holds a pairing for the id -> ref:task, unless that
//     pairing's BeadStatus is "closed", in which case the dep is dropped —
//     the work is already satisfied and needs no edge.
//
// A dep that is neither in-batch nor in the fold is a plan error naming the
// spec_node_id — v4 has no adapter-time fallback, so nothing downstream
// could resolve what this function cannot. Used identically for a create
// action's DepSpecNodeIDs and a retarget action's freshly recomputed ones
// (spec/plan/arch_resolver.md, "Retarget deps").
//
// TODO(bead:spexmachina-swvx.19): the BeadStatus == "closed" drop branch
// exists for the obsolete-then-recreate shape — a dep on a node that was
// itself obsoleted resolves to a task already closed by this run's own
// close op, so the edge is dropped rather than pointed at a dead task.
// ActionClassifier (spexmachina-swvx.16) now retargets in place instead of
// closing, so a changed node's task stays open across the change and this
// branch no longer fires for that case; Resolver's own bead is to confirm
// nothing else still relies on it before removing it.
func ResolveDeps(depSpecNodeIDs []string, batch map[string]string, fold FoldLookup) ([]Ref, error) {
	var refs []Ref
	for _, dep := range depSpecNodeIDs {
		if opID, ok := batch[dep]; ok {
			refs = append(refs, Ref{Kind: RefOp, OpID: opID})
			continue
		}
		if p, ok := fold.Lookup(dep); ok {
			if p.BeadStatus == "closed" {
				continue
			}
			refs = append(refs, Ref{Kind: RefTask, TaskID: p.TaskID})
			continue
		}
		return nil, fmt.Errorf("plan: resolve: dep %s is neither an in-batch create nor tracked by the journal fold", dep)
	}
	return refs, nil
}

// ResolveEpicAction decides, before the batch is handed to TopologicalSorter,
// whether this run's proposal epic needs a create action of its own
// (spec/plan/arch_resolver.md, "Parent Resolution"). The fold is consulted
// first: an epic task already paired with proposalRef needs no action, and
// wins even when the run also carries a registration — a live epic task is
// proof enough that the lifecycle is open. Only when the fold pairs nothing
// does the registration decide: present, the epic is a new create (labeled
// with its eid downstream, by IdempotencyLabeler); absent, that is a plan
// error naming the slug, because registration opens the lifecycle and the
// fix is `spex register`, not a synthesized epic.
//
// The returned action's SpecNodeID is the proposal ref itself rather than a
// 12-hex identity hash — the shape that tells every reader, and the fold,
// this create is the epic and not an ordinary spec node.
func ResolveEpicAction(proposalRef string, fold FoldLookup, registration Registration) (*Action, error) {
	if _, ok := fold.Lookup(proposalRef); ok {
		return nil, nil
	}
	if registration.EID == "" {
		return nil, fmt.Errorf("plan: resolve: proposal %q has no registered event in the journal and no existing epic task — run spex register first", proposalRef)
	}
	return &Action{
		Type:       ActionCreate,
		NodeType:   KindProposalEpic,
		SpecNodeID: proposalRef,
		Reason:     fmt.Sprintf("Proposal: %s", proposalRef),
	}, nil
}

// ResolveParent answers the ref every non-epic create in the run points its
// parent at (spec/plan/arch_resolver.md, "Parent Resolution"). It re-applies
// ResolveEpicAction's same fold-then-registration precedence, but this time
// batch already reflects TopologicalSorter's finished order — including the
// epic's own op_id, when ResolveEpicAction returned a new create action for
// callers to fold into that same sort. A retarget action takes no parent:
// callers never call this for one.
func ResolveParent(proposalRef string, fold FoldLookup, registration Registration, batch map[string]string) (Ref, error) {
	if p, ok := fold.Lookup(proposalRef); ok {
		return Ref{Kind: RefTask, TaskID: p.TaskID}, nil
	}
	if registration.EID == "" {
		return Ref{}, fmt.Errorf("plan: resolve: proposal %q has no registered event in the journal and no existing epic task — run spex register first", proposalRef)
	}
	opID, ok := batch[proposalRef]
	if !ok {
		return Ref{}, fmt.Errorf("plan: resolve: epic op for proposal %q missing from the sorted batch", proposalRef)
	}
	return Ref{Kind: RefOp, OpID: opID}, nil
}

// ResolvePriority walks component.implements -> module_requirement.preq_id
// -> project_requirement.priority for the component named by moduleName and
// specNodeID, and returns the minimum priority reachable across the whole
// implements set (spec/plan/arch_resolver.md, "Priority"). Its read surface
// on the spec graph is deliberately narrow: nothing about a component's
// name, description or uses edges can reach a priority number.
//
// Every way the chain can go missing — the node is not a component (a
// data_flow, a test_section, or a cleanup naming a node the current spec no
// longer carries), an implements entry names no module requirement, a
// module requirement carries no preq_id, or a preq_id names no project
// requirement or one with no priority set — falls back to FallbackPriority,
// silently: the validator's requirement_coverage_checker is the
// authoritative gate for upstream chain completeness, not this function. A
// retarget action takes no priority: callers never call this for one.
func ResolvePriority(moduleName, specNodeID string, graph SpecGraph) int {
	mod, ok := graph.moduleByName(moduleName)
	if !ok {
		return FallbackPriority
	}
	comp, ok := findComponent(mod.Spec, specNodeID)
	if !ok {
		return FallbackPriority
	}

	best := -1
	for _, reqID := range comp.Implements {
		modReq, ok := findModuleRequirement(mod.Spec, reqID)
		if !ok || modReq.PreqID == "" {
			continue
		}
		projReq, ok := graph.projectRequirement(modReq.PreqID)
		if !ok || projReq.Priority == nil {
			continue
		}
		if best == -1 || *projReq.Priority < best {
			best = *projReq.Priority
		}
	}
	if best == -1 {
		return FallbackPriority
	}
	return best
}
