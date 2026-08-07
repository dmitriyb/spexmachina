package emit

// Builder composes a changeset.json from an impact report, the mapping
// store, the spec graph, and a caller-supplied git HEAD. It is the
// orchestration layer of the emit pipeline; topological ordering, dep
// classification, and label reservation are delegated to Sorter, Resolver,
// and Labeler.
//
// TODO(bead:spexmachina-y0wc.30): Build/lookupExistingEpic/collectCreates/
// makeCreateOp below (removed pending re-derivation) depended on the
// mapping-store field and on Resolver.ResolveParent/ResolveDeps and
// Labeler.LabelFor, all retired or gutted by spexmachina-y0wc.19's
// migration of MappingStore onto the journal. bodyFor/titleFor carry no
// mapping.Store dependency and are kept live for reuse.
type Builder struct {
	SpecGraph SpecGraph
	GitHead   string
	Proposal  string
}

// bodyFor renders a create op's markdown body: links to the spec files
// that define the node, per impl_changeset_building.md "Title and Body".
// The adapter passes body through to the tracker's description field.
// Nodes without an on-disk content leaf (proposal epics) yield an empty
// body, as do cleanup ops (never routed here — their shape pins body
// empty per arch_changeset_builder.md "Cleanup op shape").
func bodyFor(a CreateAction, graph SpecGraph) string {
	p, ok := graph.Paths(a.SpecNodeID)
	if !ok || p.Content == "" {
		return ""
	}
	body := "Spec context:\n\n- " + p.Content + "\n"
	if p.Module != "" {
		body += "- " + p.Module + "\n"
	}
	return body
}

// titleFor maps a CreateAction to its canonical bead title per the impl
// spec: "<module>: <component_name>" for components, "<module>: data_flow
// <flow_name>" for data flows, "<module>: test <test_name>" for
// multi-component test sections.
func titleFor(a CreateAction) string {
	switch a.NodeType {
	case "data_flow":
		return a.Module + ": data_flow " + a.Node
	case "test_section":
		return a.Module + ": test " + a.Node
	default:
		return a.Module + ": " + a.Node
	}
}

// TODO(bead:spexmachina-y0wc.30): re-derive against the journal-backed
// MappingStore/FoldEntry (see spec/map/arch_mapping_store.md) and
// re-enable.
//
// Original implementation, preserved for reference:
//
// import (
// 	"errors"
// 	"fmt"
//
// 	"github.com/dmitriyb/spexmachina/impact"
// 	"github.com/dmitriyb/spexmachina/mapping"
// )
//
// type Builder struct {
// 	SpecGraph    SpecGraph
// 	MappingStore mapping.Store
// 	GitHead      string
// 	Proposal     string
// }
//
// // Build runs the emit pipeline end-to-end and returns the v1 changeset.
// // On any sub-component error no partial changeset is returned — the caller
// // receives the zero value plus a wrapped error.
// func (b *Builder) Build(report impact.ImpactReport) (Changeset, error) {
// 	hasExistingEpic, err := b.lookupExistingEpic()
// 	if err != nil {
// 		return Changeset{}, err
// 	}
//
// 	creates := b.collectCreates(report, hasExistingEpic)
//
// 	sorter := &Sorter{}
// 	ordered, _, err := sorter.Sort(creates)
// 	if err != nil {
// 		return Changeset{}, fmt.Errorf("emit: build: %w", err)
// 	}
//
// 	totalOps := len(ordered) + len(report.Obsoletes)
// 	pad := digits(totalOps)
// 	batchMap := make(map[string]string, len(ordered)+1)
// 	for i := range ordered {
// 		ordered[i].OpID = fmt.Sprintf("op-%0*d", pad, i+1)
// 		batchMap[ordered[i].Action.SpecNodeID] = ordered[i].OpID
// 	}
// 	if !hasExistingEpic {
// 		for _, oo := range ordered {
// 			if oo.Action.NodeType == "proposal" {
// 				batchMap["proposal/"+b.Proposal+"/epic"] = oo.OpID
// 				break
// 			}
// 		}
// 	}
//
// 	labeler := &Labeler{MappingStore: b.MappingStore}
//
// 	resolver := &Resolver{
// 		SpecGraph:    b.SpecGraph,
// 		MappingStore: b.MappingStore,
// 		Batch:        batchMap,
// 	}
//
// 	ops := make([]Op, 0, totalOps)
// 	for _, oc := range ordered {
// 		// Per-action labelling: modify-pair creates reuse existing
// 		// record-id; cleanup creates use spex:cleanup-<spec_node_id>;
// 		// fresh creates consume the cursor. See
// 		// spec/emit/arch_idempotency_labeler.md.
// 		label, err := labeler.LabelFor(oc.Action)
// 		if err != nil {
// 			return Changeset{}, fmt.Errorf("emit: build: %w", err)
// 		}
// 		op, err := b.makeCreateOp(oc, label, resolver)
// 		if err != nil {
// 			return Changeset{}, err
// 		}
// 		ops = append(ops, op)
// 	}
// 	for i, ob := range report.Obsoletes {
// 		ops = append(ops, Op{
// 			OpID:   fmt.Sprintf("op-%0*d", pad, len(ordered)+i+1),
// 			Type:   OpClose,
// 			Target: &Ref{Kind: RefBead, BeadID: ob.BeadID},
// 			Labels: []string{"spex:obsolete", "commit:" + b.GitHead},
// 			Reason: ob.Reason,
// 		})
// 	}
//
// 	return Changeset{
// 		Version:  ChangesetVersion,
// 		GitHead:  b.GitHead,
// 		Proposal: b.Proposal,
// 		Ops:      ops,
// 	}, nil
// }
//
// // lookupExistingEpic queries the mapping store for an open proposal-epic
// // record. ErrNotFound is the steady-state "first emit for this proposal"
// // signal and returns hasExistingEpic=false without an error.
// func (b *Builder) lookupExistingEpic() (bool, error) {
// 	_, err := b.MappingStore.GetByProposalEpic(b.Proposal)
// 	if err == nil {
// 		return true, nil
// 	}
// 	if errors.Is(err, mapping.ErrNotFound) {
// 		return false, nil
// 	}
// 	return false, fmt.Errorf("emit: build: proposal epic lookup %q: %w", b.Proposal, err)
// }
//
// // collectCreates flattens report.Creates into CreateActions and prepends a
// // synthetic proposal-epic create when no open epic record exists.
// func (b *Builder) collectCreates(report impact.ImpactReport, hasExistingEpic bool) []CreateAction {
// 	creates := make([]CreateAction, 0, len(report.Creates)+1)
// 	if !hasExistingEpic {
// 		creates = append(creates, CreateAction{
// 			SpecNodeID: b.Proposal,
// 			NodeType:   "proposal",
// 			Module:     "",
// 			Node:       b.Proposal,
// 		})
// 	}
// 	for _, a := range report.Creates {
// 		creates = append(creates, CreateAction{
// 			SpecNodeID:     a.SpecNodeID,
// 			NodeType:       a.NodeType,
// 			Module:         a.Module,
// 			Node:           a.Node,
// 			SpecHash:       a.SpecHash,
// 			OldBeadID:      a.OldBeadID,
// 			DepSpecNodeIDs: a.DepSpecNodeIDs,
// 			Reason:         a.Reason,
// 		})
// 	}
// 	return creates
// }
//
// // makeCreateOp builds a single create Op, applying epic-specific shortcuts
// // (no parent, no deps, fixed title), cleanup-specific shape (distinct
// // spec_node_kind, title=Reason, labels=[spex:cleanup]), and otherwise
// // delegating to Resolver.
// func (b *Builder) makeCreateOp(oc OrderedOp, label string, resolver *Resolver) (Op, error) {
// 	if oc.Action.NodeType == "proposal" {
// 		return Op{
// 			OpID:         oc.OpID,
// 			Type:         OpCreate,
// 			SpecNodeKind: "proposal_epic",
// 			SpecNodeID:   oc.Action.SpecNodeID,
// 			Idempotency:  &Idem{Label: label},
// 			Priority:     FallbackPriority,
// 			Title:        "Proposal: " + b.Proposal,
// 		}, nil
// 	}
//
// 	parent, err := resolver.ResolveParent(b.Proposal)
// 	if err != nil {
// 		return Op{}, fmt.Errorf("emit: build: %w", err)
// 	}
// 	deps, err := resolver.ResolveDeps(oc.Action.DepSpecNodeIDs)
// 	if err != nil {
// 		return Op{}, fmt.Errorf("emit: build: %w", err)
// 	}
// 	if oc.Action.OldBeadID != "" {
// 		deps = append(deps, Ref{
// 			Kind:     RefBead,
// 			BeadID:   oc.Action.OldBeadID,
// 			EdgeType: "blocks",
// 		})
// 	}
//
// 	if oc.Action.IsCleanup() {
// 		// Cleanup op shape per spec/emit/arch_changeset_builder.md
// 		// "Cleanup op shape" — distinct from the conventional
// 		// component/data_flow form. Title comes from Reason verbatim;
// 		// labels carry the spex:cleanup discriminator; priority is
// 		// FallbackPriority (3) because the pre-decouple "-1 means
// 		// don't pass --priority" sentinel doesn't translate to the
// 		// changeset's plain-int schema.
// 		return Op{
// 			OpID:         oc.OpID,
// 			Type:         OpCreate,
// 			SpecNodeKind: "cleanup",
// 			SpecNodeID:   oc.Action.SpecNodeID,
// 			Idempotency:  &Idem{Label: label},
// 			Parent:       &parent,
// 			Deps:         deps,
// 			Priority:     FallbackPriority,
// 			Title:        oc.Action.Reason,
// 			Labels:       []string{"spex:cleanup"},
// 		}, nil
// 	}
//
// 	return Op{
// 		OpID:         oc.OpID,
// 		Type:         OpCreate,
// 		SpecNodeKind: oc.Action.NodeType,
// 		SpecNodeID:   oc.Action.SpecNodeID,
// 		Idempotency:  &Idem{Label: label},
// 		Parent:       &parent,
// 		Deps:         deps,
// 		Priority:     resolver.Priority(oc.Action.SpecNodeID),
// 		Title:        titleFor(oc.Action),
// 		Body:         bodyFor(oc.Action, b.SpecGraph),
// 	}, nil
// }
