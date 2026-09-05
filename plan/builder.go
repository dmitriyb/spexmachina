package plan

import (
	"fmt"
	"path/filepath"
	"sort"
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
// when Resolver's ResolveEpicAction decides one is needed — every op_id is
// derived from its own canonical key, IdempotencyLabeler assigns every
// create's label, Resolver classifies every dep and parent ref and computes
// priority, and Build assembles the retarget and close ops itself — adding
// the layer-boundary edges "Layer edges" describes — before appending the
// absorbed array. Any sub-component error aborts the whole build; no
// partial changeset is ever returned.
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

	planRelevant := b.SpecGraph.profileOrDefault().PlanRelevant
	sorted, err := Sort(creates, planRelevant)
	if err != nil {
		return Changeset{}, fmt.Errorf("plan: build: %w", err)
	}

	// Op ids are canonical keys — op-<kind>-<key> — derived from each
	// action alone, never from its position in the batch (spec/plan/
	// arch_changeset_builder.md, "Canonical Output"). batch maps every
	// create's spec_node_id to its op_id for Resolver's ref classification;
	// opIndex maps every op_id back to its file position, the "file order"
	// a ref:op dep group sorts by.
	opIDs := make([]string, len(sorted))
	batch := make(map[string]string, len(sorted))
	for i, a := range sorted {
		opIDs[i] = createOpID(a)
		batch[a.SpecNodeID] = opIDs[i]
	}
	opIndex := make(map[string]int, len(sorted))
	for i, id := range opIDs {
		opIndex[id] = i
	}

	layerEdges, err := layerEdgesFor(sorted, opIDs, planRelevant)
	if err != nil {
		return Changeset{}, fmt.Errorf("plan: build: %w", err)
	}

	retargetTargets := make([]string, 0, len(retargets))
	for _, a := range retargets {
		retargetTargets = append(retargetTargets, a.TaskID)
	}
	sort.Strings(retargetTargets)

	labeler := &Labeler{GitHead: b.GitHead, Fold: b.Fold}

	total := len(sorted) + len(retargets) + len(obsoletes)
	ops := make([]Op, 0, total)
	for i, a := range sorted {
		label, err := labeler.LabelFor(a, opIDs[i], b.Registration)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		op, err := b.createOp(a, opIDs[i], label, batch, opIndex, layerEdges[i], retargetTargets)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		ops = append(ops, op)
	}

	for _, a := range retargets {
		op, err := b.retargetOp(a, batch, opIndex, labeler)
		if err != nil {
			return Changeset{}, fmt.Errorf("plan: build: %w", err)
		}
		ops = append(ops, op)
	}

	for _, a := range obsoletes {
		ops = append(ops, Op{
			OpID:   closeOpID(a),
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

// createOpID derives a create action's canonical op_id — op-<kind>-<key>
// — from the action alone, never from its position in the batch
// (spec/plan/arch_changeset_builder.md, "Canonical Output"). kind is the
// emitted spec_node_kind: proposal_epic, cleanup, or the node's own kind.
// key is the proposal ref for the epic and the node's identity hash for
// every other create, cleanups included — the removed node's hash, kept
// for traceability.
func createOpID(a Action) string {
	switch {
	case a.NodeType == KindProposalEpic:
		return "op-" + KindProposalEpic + "-" + a.SpecNodeID
	case isCleanup(a):
		return "op-" + KindCleanup + "-" + a.SpecNodeID
	default:
		return "op-" + a.NodeType + "-" + a.SpecNodeID
	}
}

// retargetOpID derives a retarget action's canonical op_id: its type,
// "retarget", plus the modified node's identity hash.
func retargetOpID(a Action) string {
	return "op-" + OpRetarget + "-" + a.SpecNodeID
}

// closeOpID derives a close action's canonical op_id: its type, "close",
// plus the task id the close targets — a task has at most one close, so
// this id is unique within the document.
func closeOpID(a Action) string {
	return "op-" + OpClose + "-" + a.TaskID
}

// layerGroup is one contiguous run of sorted's create indices sharing the
// same layer number. Sort emits only non-empty layers, low to high, each
// layer's actions contiguous, so a single pass over sorted recovers the
// groups without recomputing Sort's own partition.
type layerGroup struct {
	layer int
	idx   []int
}

// layerEdgesFor answers, for every index in sorted, the op ids a layer
// edge attaches to that create's deps: every create op of the previous
// non-empty layer (spec/plan/arch_changeset_builder.md, "Layer edges").
// The epic's own layer (0) is never a predecessor and never receives
// edges of its own — "the epic is no layer" — so the first non-empty
// layer after it carries no layer edges at all.
func layerEdgesFor(sorted []Action, opIDs []string, planRelevant []string) ([][]string, error) {
	layerIndex, cleanupLayer := layerPlan(planRelevant)

	var groups []layerGroup
	for i, a := range sorted {
		l, err := layerFor(a, layerIndex, cleanupLayer)
		if err != nil {
			return nil, err
		}
		if len(groups) == 0 || groups[len(groups)-1].layer != l {
			groups = append(groups, layerGroup{layer: l})
		}
		groups[len(groups)-1].idx = append(groups[len(groups)-1].idx, i)
	}

	edges := make([][]string, len(sorted))
	var prevGroupOpIDs []string
	for _, g := range groups {
		if g.layer == 0 {
			continue // the epic's layer: no predecessor, and never one itself
		}
		for _, i := range g.idx {
			edges[i] = prevGroupOpIDs
		}
		ids := make([]string, len(g.idx))
		for j, i := range g.idx {
			ids[j] = opIDs[i]
		}
		prevGroupOpIDs = ids
	}
	return edges, nil
}

// layerPlan mirrors the layer numbering Sort (plan/sorter.go) computes
// internally from the same planRelevant list: layer 0 is reserved for the
// proposal epic, one layer per planRelevant entry follows in declared
// order, and cleanupLayer is the last layer, one past planRelevant's end.
// Duplicated here (rather than exported from sorter.go) because it is two
// lines the layer-edge computation needs to reproduce, not a shared
// service the two components split responsibility over.
func layerPlan(planRelevant []string) (map[string]int, int) {
	layerIndex := make(map[string]int, len(planRelevant))
	for i, t := range planRelevant {
		layerIndex[t] = i + 1
	}
	return layerIndex, len(planRelevant) + 1
}

// createOp builds a single create Op: the epic shortcut (no parent, no
// deps, fixed priority, title = its Reason verbatim), the cleanup shape
// (distinct spec_node_kind, title = Reason, no labels, fallback priority,
// empty body, deps = layer edges plus every retarget's target), or the
// conventional component/data_flow/test_section shape — otherwise
// delegating parent and priority to Resolver and composing deps per
// composeDeps (spec/plan/arch_changeset_builder.md, "Cleanup op shape",
// "Title and body").
func (b *Builder) createOp(a Action, opID, label string, batch map[string]string, opIndex map[string]int, layerEdgeOpIDs, retargetTargets []string) (Op, error) {
	if a.NodeType == KindProposalEpic {
		return Op{
			OpID:         opID,
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

	cleanup := isCleanup(a)
	var extraTasks []string
	if cleanup {
		extraTasks = retargetTargets
	}
	deps, err := composeDeps(a, b.Fold, batch, opIndex, layerEdgeOpIDs, extraTasks)
	if err != nil {
		return Op{}, err
	}

	if cleanup {
		return Op{
			OpID:         opID,
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
		OpID:         opID,
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

// composeDeps builds one op's full deps array per the canonical ordering
// rule in "Canonical Output": resolve the action's spec-graph
// DepSpecNodeIDs via Resolver, union in the layer-edge ref:op targets and
// (for a cleanup) every retarget's target as ref:task, dedupe, then emit
// every ref:op in file order followed by every ref:task in task-id order
// — "whatever order Resolver answered in"
// (spec/plan/arch_changeset_builder.md, "Layer edges").
func composeDeps(a Action, fold FoldLookup, batch map[string]string, opIndex map[string]int, layerEdgeOpIDs, extraTaskIDs []string) ([]Ref, error) {
	specDeps, err := ResolveDeps(a.DepSpecNodeIDs, batch, fold)
	if err != nil {
		return nil, err
	}

	opSeen := map[string]bool{}
	var opRefs []string
	addOp := func(id string) {
		if !opSeen[id] {
			opSeen[id] = true
			opRefs = append(opRefs, id)
		}
	}
	taskSeen := map[string]bool{}
	var taskRefs []string
	addTask := func(id string) {
		if !taskSeen[id] {
			taskSeen[id] = true
			taskRefs = append(taskRefs, id)
		}
	}

	for _, r := range specDeps {
		if r.Kind == RefOp {
			addOp(r.OpID)
		} else {
			addTask(r.TaskID)
		}
	}
	for _, id := range layerEdgeOpIDs {
		addOp(id)
	}
	for _, id := range extraTaskIDs {
		addTask(id)
	}

	if len(opRefs) == 0 && len(taskRefs) == 0 {
		return nil, nil
	}

	sort.Slice(opRefs, func(i, j int) bool { return opIndex[opRefs[i]] < opIndex[opRefs[j]] })
	sort.Strings(taskRefs)

	deps := make([]Ref, 0, len(opRefs)+len(taskRefs))
	for _, id := range opRefs {
		deps = append(deps, Ref{Kind: RefOp, OpID: id})
	}
	for _, id := range taskRefs {
		deps = append(deps, Ref{Kind: RefTask, TaskID: id})
	}
	return deps, nil
}

// retargetOp builds a retarget Op per the shape table in
// spec/plan/arch_changeset_builder.md, "Retarget op shape": no parent, no
// priority, no body — the task already sits where it sits, and only its
// target state and deps move. Deps carry no layer edges — only a create
// gets those — but follow the same composeDeps ordering rule as any other
// op's. The label rides in Labels, not Idempotency: updates are naturally
// idempotent, so there is nothing to probe for. It carries the same
// (git_head, op_id) derivation as a node-bearing create's label, via
// Labeler.
func (b *Builder) retargetOp(a Action, batch map[string]string, opIndex map[string]int, labeler *Labeler) (Op, error) {
	opID := retargetOpID(a)
	deps, err := composeDeps(a, b.Fold, batch, opIndex, nil, nil)
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
