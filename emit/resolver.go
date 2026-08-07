package emit

import (
	"fmt"
	"math"
)

// SpecGraph supplies Resolver's reads of the
// component.implements → module_requirement.preq_id → project_requirement
// chain, plus the on-disk paths Builder links in create-op bodies.
// ChangesetBuilder constructs a concrete SpecGraph from the parsed
// spec directory; tests use a fake.
type SpecGraph interface {
	Component(specNodeID string) (Component, bool)
	ModuleRequirement(reqID string) (ModuleRequirement, bool)
	ProjectRequirement(preqID string) (ProjectRequirement, bool)
	Paths(specNodeID string) (NodePaths, bool)
}

// NodePaths locates a spec node's files on disk for changeset body links.
// Both paths are repo-relative (e.g. "spec/emit/arch_resolver.md"), the
// same form the mapping store records in content_file.
type NodePaths struct {
	Content string // the node's content leaf
	Module  string // the owning module.json
}

// Component is the slice of a spec component Resolver needs: the list of
// module-requirement IDs it implements. Other component fields (name,
// description, uses) are not read by Resolver and are intentionally absent.
type Component struct {
	Implements []string
}

// ModuleRequirement is the slice Resolver needs: the project-requirement
// hash this module requirement derives from. Empty PreqID means the chain
// terminates here and the fallback applies.
type ModuleRequirement struct {
	PreqID string
}

// ProjectRequirement is the slice Resolver needs: the priority value the
// chain ultimately resolves to. Nil pointer means the requirement carries
// no priority, which triggers the fallback.
type ProjectRequirement struct {
	Priority *int
}

// FoldEntry is the slice of one task-journal fold entry Resolver needs:
// the key's current task id, and whether the key's last journal state is
// a removal. Per spec/map/flow_bead_mapping.md, a task is only ever closed
// with no live successor when its node is removed (a modify pair's closed
// predecessor is superseded by the pair's new task, which is what the fold
// reports); so Removed is Resolver's whole notion of "closed" — the
// dependency is satisfied because the node it named is gone. A key with no
// entry at all has never had a task-bearing event.
type FoldEntry struct {
	TaskID  string
	Removed bool
}

// JournalFold is Resolver's read surface onto the task journal's fold:
// point lookup by key, either a node's identity hash or a proposal-epic
// slug — whichever a create action's dep or the run's proposal ref names.
// ChangesetBuilder builds one from mapping.MappingStore's fold; tests
// substitute a fake.
type JournalFold interface {
	Entry(key string) (FoldEntry, bool)
}

// Resolver classifies create-action deps into the two Ref shapes v2
// supports and computes per-action priority via the implements → preq_id →
// project requirement chain. Callers populate Batch (via Sorter) and Fold
// (from the task journal) before calling any method.
type Resolver struct {
	SpecGraph SpecGraph
	Fold      JournalFold
	// Batch maps spec_node_id → op_id, populated by Sorter so Resolver can
	// classify in-batch deps as ref:op. ChangesetBuilder additionally
	// injects synthetic "proposal/<ref>/epic" keys for new-epic parent
	// resolution.
	Batch map[string]string
}

// ResolveDeps classifies each dep spec_node_id into one of the two Ref
// shapes v2 supports, in priority order:
//
//  1. ref:op   — another create op in this batch.
//  2. ref:bead — the fold's pairing for the spec_node_id, if its task is
//     not closed (Removed == false).
//
// A dep whose fold pairing is closed (Removed == true) is dropped — the
// work is satisfied, no edge needed. A dep that is neither in-batch nor in
// the fold is an emit error naming the spec_node_id: version 2 dropped the
// ref:spec_node adapter-time fallback, since the adapter no longer reads
// any spex-owned file to resolve it later.
//
// Iteration order is preserved: the output array index for each surviving
// dep matches its input index modulo dropped entries. This is the
// structural fix for the broken-dep-graph bug — same-batch deps become
// ref:op so the adapter forward-resolves them at exec time rather than
// pre-resolving stale bead IDs at emit time.
func (r *Resolver) ResolveDeps(depSpecNodeIDs []string) ([]Ref, error) {
	out := make([]Ref, 0, len(depSpecNodeIDs))
	for _, id := range depSpecNodeIDs {
		if opID, ok := r.Batch[id]; ok {
			out = append(out, Ref{Kind: RefOp, OpID: opID})
			continue
		}
		entry, ok := r.Fold.Entry(id)
		if !ok {
			return nil, fmt.Errorf("emit: resolver: unresolvable dep %q: neither an in-batch create nor a task journal pairing", id)
		}
		if entry.Removed {
			// The node is gone and its task closed with it — the work is
			// satisfied, drop the dep.
			continue
		}
		out = append(out, Ref{Kind: RefBead, BeadID: entry.TaskID})
	}
	return out, nil
}

// ResolveParent returns the proposal-epic Ref every non-epic create op
// parents under. Two cases:
//
//   - Existing epic: a re-run case. The journal fold already carries a
//     live (not Removed) epic entry, keyed by the proposal slug, for this
//     proposal ref; return ref:bead.
//   - New epic: the first emit for this proposal, or a fold entry whose
//     epic task closed with no live successor (Removed == true, so it
//     carries no TaskID — same convention ResolveDeps applies). ChangesetBuilder
//     has injected a synthetic "proposal/<ref>/epic" key into r.Batch
//     pointing at the epic op_id; return ref:op.
//
// An existing, live epic always wins over an in-batch synthetic key —
// defense against double-create on a re-run that misclassified the epic
// as new.
func (r *Resolver) ResolveParent(proposal string) (Ref, error) {
	if entry, ok := r.Fold.Entry(proposal); ok && !entry.Removed {
		return Ref{Kind: RefBead, BeadID: entry.TaskID}, nil
	}
	epicKey := "proposal/" + proposal + "/epic"
	if opID, ok := r.Batch[epicKey]; ok {
		return Ref{Kind: RefOp, OpID: opID}, nil
	}
	return Ref{}, fmt.Errorf("emit: resolver: proposal epic not found for %q", proposal)
}

// Priority walks component.implements → module_requirement.preq_id →
// project_requirement.priority and returns the minimum across all
// reachable project requirements. FallbackPriority applies when the chain
// cannot be walked — emit must not fail here, the validator's
// requirement_coverage_checker is the authoritative gate.
func (r *Resolver) Priority(specNodeID string) int {
	comp, ok := r.SpecGraph.Component(specNodeID)
	if !ok {
		return FallbackPriority
	}
	best := math.MaxInt
	for _, reqID := range comp.Implements {
		modReq, ok := r.SpecGraph.ModuleRequirement(reqID)
		if !ok || modReq.PreqID == "" {
			continue
		}
		projReq, ok := r.SpecGraph.ProjectRequirement(modReq.PreqID)
		if !ok || projReq.Priority == nil {
			continue
		}
		if *projReq.Priority < best {
			best = *projReq.Priority
		}
	}
	if best == math.MaxInt {
		return FallbackPriority
	}
	return best
}
