package emit

import (
	"errors"
	"fmt"
	"math"

	"github.com/dmitriyb/spexmachina/mapping"
)

// SpecGraph supplies Resolver's reads of the
// component.implements → module_requirement.preq_id → project_requirement
// chain. ChangesetBuilder constructs a concrete SpecGraph from the parsed
// spec directory; tests use a fake.
type SpecGraph interface {
	Component(specNodeID string) (Component, bool)
	ModuleRequirement(reqID string) (ModuleRequirement, bool)
	ProjectRequirement(preqID string) (ProjectRequirement, bool)
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

// Resolver classifies create-action deps into the three Ref shapes and
// computes per-action priority via the implements → preq_id → project
// requirement chain. Callers populate Batch (via Sorter) before calling
// any method.
type Resolver struct {
	SpecGraph    SpecGraph
	MappingStore mapping.Store
	// Batch maps spec_node_id → op_id, populated by Sorter so Resolver can
	// classify in-batch deps as ref:op. ChangesetBuilder additionally
	// injects synthetic "proposal/<ref>/epic" keys for new-epic parent
	// resolution.
	Batch map[string]string
}

// ResolveDeps classifies each dep spec_node_id into one of three Ref shapes
// in priority order:
//
//  1. ref:op       — another create op in this batch.
//  2. ref:bead     — an open mapping-store record exists.
//  3. ref:spec_node — fallback; adapter resolves at exec time.
//
// Closed-bead deps are dropped (the work is satisfied). Iteration order
// is preserved: the output array index for each surviving dep matches its
// input index modulo dropped entries. This is the structural fix for the
// broken-dep-graph bug — same-batch deps become ref:op so the adapter
// forward-resolves them at exec time rather than pre-resolving stale bead
// IDs at emit time.
func (r *Resolver) ResolveDeps(depSpecNodeIDs []string) ([]Ref, error) {
	out := make([]Ref, 0, len(depSpecNodeIDs))
	for _, id := range depSpecNodeIDs {
		if opID, ok := r.Batch[id]; ok {
			out = append(out, Ref{Kind: RefOp, OpID: opID})
			continue
		}
		recs, err := r.MappingStore.GetBySpecNode(id)
		if err == nil {
			chosen, anyOpen := pickOpenRecord(recs)
			if anyOpen {
				out = append(out, Ref{Kind: RefBead, BeadID: chosen.BeadID})
				continue
			}
			// All records closed — work is satisfied, drop the dep.
			continue
		}
		if !errors.Is(err, mapping.ErrNotFound) {
			return nil, fmt.Errorf("emit: resolver: lookup %q: %w", id, err)
		}
		out = append(out, Ref{Kind: RefSpecNode, SpecNodeID: id})
	}
	return out, nil
}

// pickOpenRecord returns the highest-ID record whose BeadStatus is not
// "closed". Empty status counts as open per the impl spec — conservative
// default attaches an edge rather than silently dropping. Highest-ID
// wins so the latest re-implementation supersedes earlier records.
func pickOpenRecord(recs []mapping.Record) (mapping.Record, bool) {
	var chosen mapping.Record
	var found bool
	for _, r := range recs {
		if r.BeadStatus == "closed" {
			continue
		}
		if !found || r.ID > chosen.ID {
			chosen = r
			found = true
		}
	}
	return chosen, found
}

// ResolveParent returns the proposal-epic Ref every non-epic create op
// parents under. Two cases:
//
//   - Existing epic: a re-run case. The mapping store already holds an open
//     proposal record for this proposal ref; return ref:bead.
//   - New epic: the first emit for this proposal. ChangesetBuilder has
//     injected a synthetic "proposal/<ref>/epic" key into r.Batch pointing
//     at the epic op_id; return ref:op.
//
// An existing epic always wins over an in-batch synthetic key — defense
// against double-create on a re-run that misclassified the epic as new.
func (r *Resolver) ResolveParent(proposal string) (Ref, error) {
	rec, err := r.MappingStore.GetByProposalEpic(proposal)
	if err == nil {
		return Ref{Kind: RefBead, BeadID: rec.BeadID}, nil
	}
	if !errors.Is(err, mapping.ErrNotFound) {
		return Ref{}, fmt.Errorf("emit: resolver: proposal epic lookup %q: %w", proposal, err)
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
