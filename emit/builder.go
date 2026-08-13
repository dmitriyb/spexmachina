package emit

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/impact"
)

// Builder composes a changeset.json from an impact report, the task
// journal's fold, the run's registration, the spec graph, and a
// caller-supplied git HEAD. It is the orchestration layer of the emit
// pipeline; topological ordering, dep classification, and label assignment
// are delegated to Sorter, Resolver, and Labeler. Builder is set up once
// per run from the five values that do not change while it runs —
// SpecGraph, Fold, Registration, GitHead, Proposal — and is then handed
// exactly one impact report per Build call.
type Builder struct {
	SpecGraph SpecGraph
	Fold      JournalFold
	// Registration is the run's registration fact, resolved by EmitCommand
	// from the journal's parsed events before Builder is assembled. It
	// decides, together with Fold, whether a proposal_epic create is
	// synthesized for this run — see hasExistingEpic and Resolver.ResolveParent.
	Registration Registration
	GitHead      string
	Proposal     string
}

// Build runs the emit pipeline end-to-end and returns the v2 changeset.
// On any sub-component error no partial changeset is returned — the caller
// receives the zero value plus a wrapped error.
func (b *Builder) Build(report impact.ImpactReport) (Changeset, error) {
	hasExistingEpic := b.hasExistingEpic()
	// A fresh epic is only synthesized once the run has a registration to
	// label it with — the fold is asked first (hasExistingEpic), and the
	// registration decides only what the fold's silence means. With no
	// live epic and no registration, no epic create is added; every
	// non-epic create's parent resolution then surfaces the
	// never-registered error itself (Resolver.ResolveParent).
	synthesizeEpic := !hasExistingEpic && b.Registration.OK

	creates := b.collectCreates(report, synthesizeEpic)

	sorter := &Sorter{}
	ordered, _, err := sorter.Sort(creates)
	if err != nil {
		return Changeset{}, fmt.Errorf("emit: build: %w", err)
	}

	totalOps := len(ordered) + len(report.Obsoletes)
	pad := digits(totalOps)
	batchMap := make(map[string]string, len(ordered)+1)
	for i := range ordered {
		ordered[i].OpID = fmt.Sprintf("op-%0*d", pad, i+1)
		batchMap[ordered[i].Action.SpecNodeID] = ordered[i].OpID
	}
	if synthesizeEpic {
		for _, oo := range ordered {
			if oo.Action.NodeType == "proposal" {
				batchMap["proposal/"+b.Proposal+"/epic"] = oo.OpID
				break
			}
		}
	}

	labeler := &Labeler{}

	resolver := &Resolver{
		SpecGraph:    b.SpecGraph,
		Fold:         b.Fold,
		Registration: b.Registration,
		Batch:        batchMap,
	}

	ops := make([]Op, 0, totalOps)
	for _, oc := range ordered {
		// Per-action labelling: node-bearing creates (fresh and
		// modify-pair alike) format spex:<spec_node_id>; cleanup creates
		// format spex:cleanup-<spec_node_id>; the epic create formats
		// spex:<eid> of the run's registration. Pure function of the
		// action and the registration — no cursor, no store read. See
		// spec/emit/arch_idempotency_labeler.md.
		label, err := labeler.LabelFor(oc.Action, b.Registration)
		if err != nil {
			return Changeset{}, fmt.Errorf("emit: build: %w", err)
		}
		op, err := b.makeCreateOp(oc, label, resolver)
		if err != nil {
			return Changeset{}, err
		}
		ops = append(ops, op)
	}
	for i, ob := range report.Obsoletes {
		ops = append(ops, Op{
			OpID:   fmt.Sprintf("op-%0*d", pad, len(ordered)+i+1),
			Type:   OpClose,
			Target: &Ref{Kind: RefBead, BeadID: ob.BeadID},
			Labels: []string{"spex:obsolete", "commit:" + b.GitHead},
			Reason: ob.Reason,
		})
	}

	return Changeset{
		Version:  ChangesetVersion,
		GitHead:  b.GitHead,
		Proposal: b.Proposal,
		Ops:      ops,
	}, nil
}

// hasExistingEpic reports whether the task journal fold already carries a
// live (not Removed) epic entry for this run's proposal slug — the same
// check Resolver.ResolveParent applies. A miss, or a fold entry whose epic
// task closed with no live successor, means there is no live epic; whether
// Builder then synthesizes a fresh proposal_epic create additionally
// depends on the run's registration — see synthesizeEpic in Build.
func (b *Builder) hasExistingEpic() bool {
	entry, ok := b.Fold.Entry(b.Proposal)
	return ok && !entry.Removed
}

// collectCreates flattens report.Creates into CreateActions and prepends a
// synthetic proposal-epic create when synthesizeEpic is true (no live epic
// in the fold, and the run has a registration to label it with).
func (b *Builder) collectCreates(report impact.ImpactReport, synthesizeEpic bool) []CreateAction {
	creates := make([]CreateAction, 0, len(report.Creates)+1)
	if synthesizeEpic {
		creates = append(creates, CreateAction{
			SpecNodeID: b.Proposal,
			NodeType:   "proposal",
			Module:     "",
			Node:       b.Proposal,
		})
	}
	for _, a := range report.Creates {
		creates = append(creates, CreateAction{
			SpecNodeID:     a.SpecNodeID,
			NodeType:       a.NodeType,
			Module:         a.Module,
			Node:           a.Node,
			SpecHash:       a.SpecHash,
			OldBeadID:      a.OldBeadID,
			DepSpecNodeIDs: a.DepSpecNodeIDs,
			Reason:         a.Reason,
		})
	}
	return creates
}

// makeCreateOp builds a single create Op, applying epic-specific shortcuts
// (no parent, no deps, fixed title), cleanup-specific shape (distinct
// spec_node_kind, title=Reason, labels=[spex:cleanup]), and otherwise
// delegating to Resolver.
func (b *Builder) makeCreateOp(oc OrderedOp, label string, resolver *Resolver) (Op, error) {
	if oc.Action.NodeType == "proposal" {
		return Op{
			OpID:         oc.OpID,
			Type:         OpCreate,
			SpecNodeKind: "proposal_epic",
			SpecNodeID:   oc.Action.SpecNodeID,
			Idempotency:  &Idem{Label: label},
			Priority:     FallbackPriority,
			Title:        "Proposal: " + b.Proposal,
		}, nil
	}

	parent, err := resolver.ResolveParent(b.Proposal)
	if err != nil {
		return Op{}, fmt.Errorf("emit: build: %w", err)
	}
	deps, err := resolver.ResolveDeps(oc.Action.DepSpecNodeIDs)
	if err != nil {
		return Op{}, fmt.Errorf("emit: build: %w", err)
	}
	if oc.Action.OldBeadID != "" {
		deps = append(deps, Ref{
			Kind:     RefBead,
			BeadID:   oc.Action.OldBeadID,
			EdgeType: "blocks",
		})
	}

	if oc.Action.IsCleanup() {
		// Cleanup op shape per spec/emit/arch_changeset_builder.md
		// "Cleanup op shape" — distinct from the conventional
		// component/data_flow form. Title comes from Reason verbatim;
		// labels carry the spex:cleanup discriminator; priority is
		// FallbackPriority (3) because the pre-decouple "-1 means
		// don't pass --priority" sentinel doesn't translate to the
		// changeset's plain-int schema.
		return Op{
			OpID:         oc.OpID,
			Type:         OpCreate,
			SpecNodeKind: "cleanup",
			SpecNodeID:   oc.Action.SpecNodeID,
			Idempotency:  &Idem{Label: label},
			Parent:       &parent,
			Deps:         deps,
			Priority:     FallbackPriority,
			Title:        oc.Action.Reason,
			Labels:       []string{"spex:cleanup"},
		}, nil
	}

	return Op{
		OpID:         oc.OpID,
		Type:         OpCreate,
		SpecNodeKind: oc.Action.NodeType,
		SpecNodeID:   oc.Action.SpecNodeID,
		Idempotency:  &Idem{Label: label},
		Parent:       &parent,
		Deps:         deps,
		Priority:     resolver.Priority(oc.Action.SpecNodeID),
		Title:        titleFor(oc.Action),
		Body:         bodyFor(oc.Action, b.SpecGraph),
	}, nil
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
		return fmt.Sprintf("%s: data_flow %s", a.Module, a.Node)
	case "test_section":
		return fmt.Sprintf("%s: test %s", a.Module, a.Node)
	default:
		return fmt.Sprintf("%s: %s", a.Module, a.Node)
	}
}
