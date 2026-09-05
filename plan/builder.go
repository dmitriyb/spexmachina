package plan

import (
	"fmt"
	"path/filepath"
	"strconv"
)

// Fold is ChangesetBuilder's combined view onto the journal fold: Resolver's
// dep/parent classification (FoldLookup) and IdempotencyLabeler's
// cleanup-removal lookup (RemovalLookup) together, so a caller hands the
// builder one fold adapter instead of two
// (spec/plan/arch_changeset_builder.md, "Interface"). PlanCommand adapts the
// parsed journal fold onto this surface; tests substitute a stand-in.
type Fold interface {
	FoldLookup
	RemovalLookup
}

// Builder composes changeset.json v4 from one batch of classified actions
// and the run's fixed inputs — the spec graph, the journal fold, the run's
// registration, the git HEAD SHA, the proposal ref, and the absorbed
// entries PlanCommand composed off the diff's withheld marks
// (spec/plan/arch_changeset_builder.md, "Interface"). It is set up once per
// run and handed exactly one batch; Build answers with one v4 changeset or
// an error, never both.
type Builder struct {
	SpecGraph    SpecGraph
	Fold         Fold
	Registration Registration
	GitHead      string
	Proposal     string
	// Absorbed carries the finished entries PlanCommand composed off the
	// diff's withheld marks. Builder writes them into the top-level
	// absorbed array verbatim — it consults no absorb rule itself.
	Absorbed []AbsorbedEntry
}

// Build runs the four-component composition end to end
// (spec/plan/arch_changeset_builder.md, "Op Kinds" and "Canonical Output"):
// TopologicalSorter orders the create actions — the proposal epic first,
// when Resolver's ResolveEpicAction decides one is needed — IdempotencyLabeler
// assigns every create's label, Resolver classifies every dep and parent ref
// and computes priority, and Build assembles the retarget and close ops
// itself before appending the absorbed array. Any sub-component error
// aborts the whole build; no partial changeset is ever returned.
func (b *Builder) Build(actions []Action) (Changeset, error) {
	var creates, retargets, obsoletes []Action
	for _, a := range actions {
		switch a.Type {
		case ActionCreate:
			creates = append(creates, a)
		case ActionRetarget:
			retargets = append(retargets, a)
		case ActionObsolete:
			obsoletes = append(obsoletes, a)
		}
	}

	epic, err := ResolveEpicAction(b.Proposal, b.Fold, b.Registration)
	if err != nil {
		return Changeset{}, fmt.Errorf("plan: build: %w", err)
	}
	if epic != nil {
		creates = append([]Action{*epic}, creates...)
	}

	sorted, err := Sort(creates, b.SpecGraph.profileOrDefault().PlanRelevant)
	if err != nil {
		return Changeset{}, fmt.Errorf("plan: build: %w", err)
	}

	// Op ids are assigned here, over every op kind together — creates
	// first, then retargets, then closes — and zero-padded to the digit
	// width of the total. Sort/TopologicalSorter hands back the creates
	// alone, with no id of their own, and so cannot compute this width
	// itself.
	//
	// TODO(bead:spexmachina-swvx.20): spec/plan/flow_plan.md's 822b817
	// correction derives every op_id from its own canonical key — kind plus
	// the node or task id it acts on, e.g. "op-component-4c1146bb7287",
	// "op-retarget-80afb22dab75" — never from position (see the
	// changeset.json example in "changeset.json (output)"), and adds a
	// layer-boundary edge to every create: a ref:op dep on every create of
	// the previous non-empty layer, with the cleanup layer's creates also
	// carrying a ref:task dep on each retarget's target (module.json's
	// abfb10394fdd, "Layer order as blocking edges"). Neither the
	// digit-padded op-%0*d numbering below nor any layer-edge wiring exists
	// yet; both are ChangesetBuilder's own bead to add. Sort/
	// TopologicalSorter (spexmachina-swvx.38) returns a flat list of
	// actions with no layer boundaries of its own to read — this bead
	// derives them itself from the same PlanRelevant order and layerFor
	// (plan/sorter.go) Sort already applies internally.
	total := len(sorted) + len(retargets) + len(obsoletes)
	pad := digits(total)
	ordered := make([]OrderedOp, len(sorted))
	batch := make(map[string]string, len(sorted))
	for i, a := range sorted {
		opID := fmt.Sprintf("op-%0*d", pad, i+1)
		ordered[i] = OrderedOp{OpID: opID, Action: a}
		batch[a.SpecNodeID] = opID
	}

	labeler := &Labeler{GitHead: b.GitHead, Fold: b.Fold}

	ops := make([]Op, 0, total)
	for _, oo := range ordered {
		label, err := labeler.LabelFor(oo.Action, oo.OpID, b.Registration)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		op, err := b.createOp(oo, label, batch)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		ops = append(ops, op)
	}

	for i, a := range retargets {
		opID := fmt.Sprintf("op-%0*d", pad, len(ordered)+i+1)
		op, err := b.retargetOp(a, opID, batch, labeler)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		ops = append(ops, op)
	}

	for i, a := range obsoletes {
		ops = append(ops, Op{
			OpID:   fmt.Sprintf("op-%0*d", pad, len(ordered)+len(retargets)+i+1),
			Type:   OpClose,
			Target: &Ref{Kind: RefTask, TaskID: a.TaskID},
			Reason: a.Reason,
		})
	}

	absorbed := b.Absorbed
	if absorbed == nil {
		absorbed = []AbsorbedEntry{}
	}

	return Changeset{
		Version:  ChangesetVersion,
		GitHead:  b.GitHead,
		Proposal: b.Proposal,
		Ops:      ops,
		Absorbed: absorbed,
	}, nil
}

// createOp builds a single create Op: the epic shortcut (no parent, no
// deps, fixed priority, title = its Reason verbatim), the cleanup shape
// (distinct spec_node_kind, title = Reason, no labels, fallback priority,
// empty body), or the conventional component/data_flow/test_section
// shape — otherwise delegating parent, deps and priority to Resolver
// (spec/plan/arch_changeset_builder.md, "Cleanup op shape", "Title and
// body").
func (b *Builder) createOp(oo OrderedOp, label string, batch map[string]string) (Op, error) {
	a := oo.Action

	if a.NodeType == KindProposalEpic {
		return Op{
			OpID:         oo.OpID,
			Type:         OpCreate,
			SpecNodeKind: KindProposalEpic,
			SpecNodeID:   a.SpecNodeID,
			Idempotency:  &Idem{Label: label},
			Priority:     FallbackPriority,
			Title:        a.Reason,
		}, nil
	}

	parent, err := ResolveParent(b.Proposal, b.Fold, b.Registration, batch)
	if err != nil {
		return Op{}, err
	}
	deps, err := ResolveDeps(a.DepSpecNodeIDs, batch, b.Fold)
	if err != nil {
		return Op{}, err
	}
	if a.OldTaskID != "" {
		// Every create that replaces an obsoleted bead — cleanup and
		// modify-pair alike — carries one extra lineage dep naming the old
		// bead, so the replacement's lineage survives after the close op
		// runs.
		deps = append(deps, Ref{Kind: RefTask, TaskID: a.OldTaskID, EdgeType: "blocks"})
	}

	if isCleanup(a) {
		return Op{
			OpID:         oo.OpID,
			Type:         OpCreate,
			SpecNodeKind: KindCleanup,
			SpecNodeID:   a.SpecNodeID,
			Idempotency:  &Idem{Label: label},
			Parent:       &parent,
			Deps:         deps,
			Priority:     FallbackPriority,
			Title:        a.Reason,
		}, nil
	}

	return Op{
		OpID:         oo.OpID,
		Type:         OpCreate,
		SpecNodeKind: a.NodeType,
		SpecNodeID:   a.SpecNodeID,
		Idempotency:  &Idem{Label: label},
		Parent:       &parent,
		Deps:         deps,
		Priority:     ResolvePriority(a.Module, a.SpecNodeID, b.SpecGraph),
		Title:        titleFor(a),
		Body:         bodyFor(a, b.SpecGraph),
	}, nil
}

// retargetOp builds a retarget Op per the shape table in
// spec/plan/arch_changeset_builder.md, "Retarget op shape": no parent, no
// priority, no body — the task already sits where it sits, and only its
// target state and deps move. The label rides in Labels, not Idempotency:
// updates are naturally idempotent, so there is nothing to probe for. It
// carries the same (git_head, op_id) derivation as a node-bearing create's
// label, via Labeler.
func (b *Builder) retargetOp(a Action, opID string, batch map[string]string, labeler *Labeler) (Op, error) {
	deps, err := ResolveDeps(a.DepSpecNodeIDs, batch, b.Fold)
	if err != nil {
		return Op{}, err
	}
	label, err := labeler.LabelFor(a, opID, b.Registration)
	if err != nil {
		return Op{}, err
	}
	return Op{
		OpID:       opID,
		Type:       OpRetarget,
		SpecNodeID: a.SpecNodeID,
		SpecHash:   a.SpecHash,
		Target:     &Ref{Kind: RefTask, TaskID: a.TaskID},
		Deps:       deps,
		Labels:     []string{label},
		Reason:     a.Reason,
	}, nil
}

// titleFor maps a conventional create action to its canonical bead title
// (spec/plan/arch_changeset_builder.md, "Title and body"). The epic and
// cleanup shapes take their title from Reason verbatim instead, in
// createOp.
func titleFor(a Action) string {
	switch a.NodeType {
	case KindDataFlow:
		return fmt.Sprintf("%s: data_flow %s", a.Module, a.Node)
	case KindTestSection:
		return fmt.Sprintf("%s: test %s", a.Module, a.Node)
	default:
		return fmt.Sprintf("%s: %s", a.Module, a.Node)
	}
}

// bodyFor renders a create op's markdown body: the repo-relative path of
// the node's own content leaf, and the path of the module.json that
// declares it — the spec context a reader of the bead needs and nothing
// beyond it (spec/plan/arch_changeset_builder.md, "Title and body"). A node
// with no on-disk content leaf, or one the current spec graph cannot
// resolve, yields an empty body.
func bodyFor(a Action, graph SpecGraph) string {
	mod, ok := graph.moduleByName(a.Module)
	if !ok {
		return ""
	}

	var content string
	switch a.NodeType {
	case KindComponent:
		if c, ok := findComponent(mod.Spec, a.SpecNodeID); ok {
			content = c.Content
		}
	case KindDataFlow:
		if f, ok := findDataFlow(mod.Spec, a.SpecNodeID); ok {
			content = f.Content
		}
	case KindTestSection:
		if ts, ok := findTestSection(mod.Spec, a.SpecNodeID); ok {
			content = ts.Content
		}
	}
	if content == "" {
		return ""
	}

	return fmt.Sprintf("Spec context:\n\n- %s\n- %s\n",
		filepath.Join("spec", mod.Module.Path, content),
		filepath.Join("spec", mod.Module.Path, "module.json"))
}

// digits reports the decimal digit width of n, the zero-padding width
// Build applies to every op_id (spec/plan/arch_changeset_builder.md,
// "Canonical Output": "Nine ops number op-1 through op-9; forty number
// op-01 through op-40"). n <= 0 (an empty batch) still pads to width 1.
func digits(n int) int {
	if n <= 0 {
		return 1
	}
	return len(strconv.Itoa(n))
}
