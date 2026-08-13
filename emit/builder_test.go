package emit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
)

// builderEnv bundles the fakes a Builder test typically needs: a fake
// task-journal fold (open task pairings, removed nodes, proposal-epic
// entries — whatever the scenario seeds) and a fake spec graph.
//
// reg is a pointer so most scenarios can leave it unset: build() then
// defaults it to a present registration keyed off the run's own
// proposal/head, which is what lets a plain "new proposal" fixture
// synthesize an epic without every test having to spell out a
// registration event by hand. Scenarios that specifically exercise the
// registration-absent path (legacy epics, never-registered proposals) set
// reg explicitly to override the default.
type builderEnv struct {
	fold  fakeFold
	graph *fakeSpecGraph
	reg   *Registration
}

func newBuilderEnv() *builderEnv {
	return &builderEnv{
		fold:  fakeFold{},
		graph: newFakeSpecGraph(),
	}
}

func (e *builderEnv) build(report impact.ImpactReport, proposal, head string) (Changeset, error) {
	reg := e.reg
	if reg == nil {
		reg = &Registration{EID: head + ":" + proposal, OK: true}
	}
	b := &Builder{
		SpecGraph:    e.graph,
		Fold:         e.fold,
		Registration: *reg,
		GitHead:      head,
		Proposal:     proposal,
	}
	return b.Build(report)
}

func sampleComponentCreate(specID, module, node string, deps []string) impact.Action {
	return impact.Action{
		Type:           "create",
		Module:         module,
		Node:           node,
		NodeType:       "component",
		SpecNodeID:     specID,
		SpecHash:       "h-" + specID,
		DepSpecNodeIDs: deps,
		Reason:         "New spec node: " + module + "/" + node,
	}
}

// findOp locates the first op whose SpecNodeID matches the given id.
func findOp(t *testing.T, ops []Op, specNodeID string) Op {
	t.Helper()
	for _, op := range ops {
		if op.SpecNodeID == specNodeID {
			return op
		}
	}
	t.Fatalf("no op with spec_node_id %q in %+v", specNodeID, ops)
	return Op{}
}

// TestBuild_CanonicalTopLevelFields covers the spec scenario:
// "Build a changeset from a single-create impact report. Assert the output
// has version: 2 at the top, git_head set to the fixed SHA, and canonical
// field order."
func TestBuild_CanonicalTopLevelFields(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("c1", "emit", "Foo", nil),
		},
	}
	cs, err := env.build(report, "2026-04-foo", "deadbeefcafe1234")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cs.Version != 2 {
		t.Errorf("version: want 2, got %d", cs.Version)
	}
	if cs.GitHead != "deadbeefcafe1234" {
		t.Errorf("git_head: want deadbeefcafe1234, got %q", cs.GitHead)
	}
	if cs.Proposal != "2026-04-foo" {
		t.Errorf("proposal: want 2026-04-foo, got %q", cs.Proposal)
	}

	// Marshal and assert the top-level field order is version, git_head,
	// proposal, ops.
	raw, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	idxVersion := strings.Index(got, `"version"`)
	idxGitHead := strings.Index(got, `"git_head"`)
	idxProposal := strings.Index(got, `"proposal"`)
	idxOps := strings.Index(got, `"ops"`)
	if !(idxVersion < idxGitHead && idxGitHead < idxProposal && idxProposal < idxOps) {
		t.Errorf("top-level field order broken:\n%s", got)
	}
}

// TestBuild_DeterministicAcrossRuns covers the spec's byte-identical
// output requirement: same inputs → identical bytes on every run.
func TestBuild_DeterministicAcrossRuns(t *testing.T) {
	makeReport := func() impact.ImpactReport {
		return impact.ImpactReport{
			Creates: []impact.Action{
				sampleComponentCreate("c2", "emit", "Bar", nil),
				sampleComponentCreate("c1", "emit", "Foo", nil),
				sampleComponentCreate("c3", "emit", "Baz", []string{"c1"}),
			},
		}
	}

	env1 := newBuilderEnv()
	cs1, err := env1.build(makeReport(), "2026-04-foo", "abc123")
	if err != nil {
		t.Fatalf("Build #1: %v", err)
	}

	env2 := newBuilderEnv()
	cs2, err := env2.build(makeReport(), "2026-04-foo", "abc123")
	if err != nil {
		t.Fatalf("Build #2: %v", err)
	}

	out1, _ := json.MarshalIndent(cs1, "", "  ")
	out2, _ := json.MarshalIndent(cs2, "", "  ")
	if !bytes.Equal(out1, out2) {
		t.Errorf("non-deterministic output:\nrun1=%s\nrun2=%s", out1, out2)
	}
}

// TestBuild_ProposalEpicIsFirstAndParents covers the spec scenario:
// "Impact report with one component create, one data_flow create; the
// run's registration carries the proposal's registered event. Assert:
// first op is type: create with spec_node_kind: proposal_epic and
// idempotency.label: the registered event's eid, not the changeset's own
// git_head; following two creates' parent fields are {ref:op, op_id:<epic
// op_id>}." The registration's eid deliberately uses a head distinct from
// the run's --git-head to prove the label derives from the registration,
// not from GitHead.
func TestBuild_ProposalEpicIsFirstAndParents(t *testing.T) {
	env := newBuilderEnv()
	env.reg = &Registration{EID: "reg-head1:p-ref", OK: true}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("c1", "emit", "Comp", nil),
			{
				Type:       "create",
				Module:     "emit",
				Node:       "Flow",
				NodeType:   "data_flow",
				SpecNodeID: "f1",
				SpecHash:   "h-f1",
			},
		},
	}
	cs, err := env.build(report, "p-ref", "head1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) < 3 {
		t.Fatalf("expected 3+ ops (epic + comp + flow), got %d", len(cs.Ops))
	}
	first := cs.Ops[0]
	if first.Type != OpCreate || first.SpecNodeKind != "proposal_epic" {
		t.Errorf("first op: want type=create kind=proposal_epic, got type=%s kind=%s", first.Type, first.SpecNodeKind)
	}
	if first.Idempotency == nil || first.Idempotency.Label != "spex:reg-head1:p-ref" {
		t.Errorf("epic label: want spex:reg-head1:p-ref (registration eid, not git_head head1), got %+v", first.Idempotency)
	}
	epicOpID := first.OpID
	for _, op := range cs.Ops[1:] {
		if op.Type != OpCreate {
			continue
		}
		if op.Parent == nil {
			t.Errorf("op %s: missing parent", op.OpID)
			continue
		}
		if op.Parent.Kind != RefOp || op.Parent.OpID != epicOpID {
			t.Errorf("op %s: parent want ref:op %s, got %+v", op.OpID, epicOpID, op.Parent)
		}
	}
}

// TestBuild_InBatchDepChainResolvesToRefOp covers the spec scenario:
// "Three new components A, B, C where B uses A and C uses B. Impact emits
// DepSpecNodeIDs on each. Assert: A's create has empty deps. B's create
// has deps: [{ref:op, op_id:<A op>}]. C's create has deps: [{ref:op, op_id:<B op>}]."
func TestBuild_InBatchDepChainResolvesToRefOp(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("A", "m", "A", nil),
			sampleComponentCreate("B", "m", "B", []string{"A"}),
			sampleComponentCreate("C", "m", "C", []string{"B"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	a := findOp(t, cs.Ops, "A")
	b := findOp(t, cs.Ops, "B")
	c := findOp(t, cs.Ops, "C")
	if len(a.Deps) != 0 {
		t.Errorf("A.deps: want empty, got %+v", a.Deps)
	}
	if len(b.Deps) != 1 || b.Deps[0].Kind != RefOp || b.Deps[0].OpID != a.OpID {
		t.Errorf("B.deps: want [ref:op %s], got %+v", a.OpID, b.Deps)
	}
	if len(c.Deps) != 1 || c.Deps[0].Kind != RefOp || c.Deps[0].OpID != b.OpID {
		t.Errorf("C.deps: want [ref:op %s], got %+v", b.OpID, c.Deps)
	}
}

// TestBuild_ExistingBeadDepResolvesToRefBead covers the spec scenario:
// "New component X uses existing-and-open component Y. Assert X's create
// has deps: [{ref:bead, bead_id:<Y's bead>}]."
func TestBuild_ExistingBeadDepResolvesToRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold["Y"] = FoldEntry{TaskID: "br-Y"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", []string{"Y"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0].Kind != RefBead || x.Deps[0].BeadID != "br-Y" {
		t.Fatalf("X.deps: want [ref:bead br-Y], got %+v", x.Deps)
	}
}

// TestBuild_ClosedBeadDepIsDropped covers the spec scenario:
// "Y's mapping record has status: closed. Assert X's deps is empty."
func TestBuild_ClosedBeadDepIsDropped(t *testing.T) {
	env := newBuilderEnv()
	env.fold["Y"] = FoldEntry{Removed: true}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", []string{"Y"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 0 {
		t.Errorf("X.deps: want empty, got %+v", x.Deps)
	}
}

// TestBuild_UnresolvableDepIsError covers test_changeset_builder.md's
// "Unresolvable dep is an emit error" scenario: Z has no mapping record
// and no in-batch op. v2 dropped the ref:spec_node adapter-time fallback,
// so Build() must error naming Z instead of emitting a deferred ref.
func TestBuild_UnresolvableDepIsError(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", []string{"Z"}),
		},
	}
	_, err := env.build(report, "p", "h")
	if err == nil {
		t.Fatal("Build: want error for unresolvable dep Z, got nil")
	}
	if !strings.Contains(err.Error(), "Z") {
		t.Errorf("error must name the unresolvable spec_node_id Z: %v", err)
	}
}

// TestBuild_PriorityPropagation covers the spec scenario:
// "Component implements two module requirements with preq priorities 2
// and 1. Assert the create op's priority is 1 (lowest wins)."
func TestBuild_PriorityPropagation(t *testing.T) {
	env := newBuilderEnv()
	env.graph.components["X"] = Component{Implements: []string{"R1", "R2"}}
	env.graph.moduleReqs["R1"] = ModuleRequirement{PreqID: "P1"}
	env.graph.moduleReqs["R2"] = ModuleRequirement{PreqID: "P2"}
	env.graph.projectReqs["P1"] = ProjectRequirement{Priority: intPtr(2)}
	env.graph.projectReqs["P2"] = ProjectRequirement{Priority: intPtr(1)}

	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if x.Priority != 1 {
		t.Errorf("priority: want 1 (min of 2,1), got %d", x.Priority)
	}
}

// TestBuild_PriorityFallback covers the documented fallback path: a
// component with no walkable chain gets FallbackPriority.
func TestBuild_PriorityFallback(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if x.Priority != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d, got %d", FallbackPriority, x.Priority)
	}
}

// TestBuild_ObsoleteAndCreateLineage covers the spec scenario:
// "Modified component Q: old bead spexmachina-abc closed, new create op.
// Assert: close op carries target {ref:bead, bead_id:spexmachina-abc} and
// labels [spex:obsolete, commit:<git-head>]. Create op for the replacement
// includes deps [{ref:bead, bead_id:spexmachina-abc, type:blocks}] for
// lineage."
//
// Also covers the modify-pair rule from arch_idempotency_labeler.md: the
// create op's idempotency.label is spex:<git_head>:<its own op_id> — the
// eid of this run's own modified event, distinct from whatever label the
// closed predecessor carried, since each change in the lineage references
// its own event. No fold seeding is needed to make this true (see
// TestBuild_ObsoleteAndCreateLineageLabelHoldsWithEmptyFold for the
// empty-fold half of that assertion).
func TestBuild_ObsoleteAndCreateLineage(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "m",
				Node:       "Q",
				NodeType:   "component",
				SpecNodeID: "Q",
				SpecHash:   "h-Q",
				OldBeadID:  "spexmachina-abc",
				Reason:     "Spec node modified (new): m/Q",
			},
		},
		Obsoletes: []impact.Action{
			{
				Type:       "obsolete",
				BeadID:     "spexmachina-abc",
				Module:     "m",
				Node:       "Q",
				NodeType:   "component",
				SpecNodeID: "Q",
				ChangeType: "modified",
				Reason:     "Spec node modified: m/Q",
			},
		},
	}
	cs, err := env.build(report, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	q := findOp(t, cs.Ops, "Q")
	var foundLineage bool
	for _, d := range q.Deps {
		if d.Kind == RefBead && d.BeadID == "spexmachina-abc" && d.EdgeType == "blocks" {
			foundLineage = true
		}
	}
	if !foundLineage {
		t.Errorf("Q.deps: want ref:bead spexmachina-abc with type:blocks, got %+v", q.Deps)
	}

	var closeOp Op
	var closeFound bool
	for _, op := range cs.Ops {
		if op.Type == OpClose {
			closeOp = op
			closeFound = true
			break
		}
	}
	if !closeFound {
		t.Fatal("no close op emitted")
	}
	if closeOp.Target == nil || closeOp.Target.Kind != RefBead || closeOp.Target.BeadID != "spexmachina-abc" {
		t.Errorf("close target: want ref:bead spexmachina-abc, got %+v", closeOp.Target)
	}
	wantLabels := []string{"spex:obsolete", "commit:deadbeef"}
	if len(closeOp.Labels) != 2 || closeOp.Labels[0] != wantLabels[0] || closeOp.Labels[1] != wantLabels[1] {
		t.Errorf("close labels: want %v, got %v", wantLabels, closeOp.Labels)
	}
	if closeOp.Reason != "Spec node modified: m/Q" {
		t.Errorf("close reason: got %q", closeOp.Reason)
	}

	// Modify-pair label: the create's idempotency.label is spex:<git_head>
	// :<its own op_id> — this run's own change event, not a store-derived
	// value and not the close op's id.
	wantLabel := "spex:deadbeef:" + q.OpID
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (own op_id), got %+v", wantLabel, q.Idempotency)
	}
}

// TestBuild_ObsoleteAndCreateLineageLabelHoldsWithEmptyFold covers the other
// half of arch_idempotency_labeler.md's modify-pair assertion: the
// replacement create's idempotency.label is spex:<git_head>:<its own op_id>
// even when the journal fold has no record at all for OldBeadID — a
// modify-pair's label never consults the fold, only its own op_id.
func TestBuild_ObsoleteAndCreateLineageLabelHoldsWithEmptyFold(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "m",
				Node:       "Q",
				NodeType:   "component",
				SpecNodeID: "Q",
				SpecHash:   "h-Q",
				OldBeadID:  "spexmachina-abc",
				Reason:     "Spec node modified (new): m/Q",
			},
		},
	}
	cs, err := env.build(report, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	q := findOp(t, cs.Ops, "Q")
	wantLabel := "spex:deadbeef:" + q.OpID
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (own op_id, no fold lookup), got %+v", wantLabel, q.Idempotency)
	}
}

// TestBuild_CleanupOpShape covers the spec scenario from
// arch_changeset_builder.md "Cleanup op shape" and test_changeset_builder.md's
// "Cleanup-bead create": an action with Reason starting "Code cleanup:"
// produces an op with spec_node_kind="cleanup", title=Reason verbatim,
// labels=[spex:cleanup], deps carrying the OldBeadID lineage, priority=
// FallbackPriority, and idempotency.label=spex:<git_head>:<close op_id> —
// the eid of the removed event the same-batch close implies, so the
// cleanup's task_created referent and its label are the same event.
func TestBuild_CleanupOpShape(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "m",
				Node:       "X",
				NodeType:   "component",
				SpecNodeID: "abc123def456",
				OldBeadID:  "spexmachina-old",
				Reason:     "Code cleanup: m/X",
			},
		},
		Obsoletes: []impact.Action{
			{
				Type:       "obsolete",
				BeadID:     "spexmachina-old",
				Module:     "m",
				Node:       "X",
				NodeType:   "component",
				ChangeType: "removed",
				Reason:     "Spec node removed: m/X",
			},
		},
	}
	cs, err := env.build(report, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	op := findOp(t, cs.Ops, "abc123def456")

	if op.SpecNodeKind != "cleanup" {
		t.Errorf("spec_node_kind: want cleanup, got %q", op.SpecNodeKind)
	}
	if op.Title != "Code cleanup: m/X" {
		t.Errorf("title: want Reason verbatim, got %q", op.Title)
	}
	if len(op.Labels) != 1 || op.Labels[0] != "spex:cleanup" {
		t.Errorf("labels: want [spex:cleanup], got %+v", op.Labels)
	}
	var closeOp Op
	for _, o := range cs.Ops {
		if o.Type == OpClose {
			closeOp = o
			break
		}
	}
	if closeOp.OpID == "" {
		t.Fatal("no close op found for spexmachina-old")
	}
	wantLabel := "spex:deadbeef:" + closeOp.OpID
	if op.Idempotency == nil || op.Idempotency.Label != wantLabel {
		t.Errorf("idempotency.label: want %q (same-batch close op's eid), got %+v", wantLabel, op.Idempotency)
	}
	if op.Priority != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d, got %d", FallbackPriority, op.Priority)
	}
	var foundLineage bool
	for _, d := range op.Deps {
		if d.Kind == RefBead && d.BeadID == "spexmachina-old" && d.EdgeType == "blocks" {
			foundLineage = true
		}
	}
	if !foundLineage {
		t.Errorf("deps: want ref:bead spexmachina-old type:blocks, got %+v", op.Deps)
	}
}

// TestBuild_CleanupOpUsesFoldRemovedEventForPriorBatchRemoval covers
// test_changeset_builder.md's "Cleanup-bead create for a prior-batch
// removal" scenario: the journal already holds the removed event for the
// node (read from the fold), and the batch carries the cleanup create but
// no same-batch close op — its close landed last run. The label must read
// from the fold, not be derived from any op in this batch, so a re-run at
// a moved HEAD still carries the label of the removal it answers.
func TestBuild_CleanupOpUsesFoldRemovedEventForPriorBatchRemoval(t *testing.T) {
	env := newBuilderEnv()
	env.fold["abc123def456"] = FoldEntry{Removed: true, RemovedEID: "E1"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "m",
				Node:       "X",
				NodeType:   "component",
				SpecNodeID: "abc123def456",
				OldBeadID:  "spexmachina-old",
				Reason:     "Code cleanup: m/X",
			},
		},
	}
	cs, err := env.build(report, "p", "deadbeef-moved")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	op := findOp(t, cs.Ops, "abc123def456")
	if op.Idempotency == nil || op.Idempotency.Label != "spex:E1" {
		t.Errorf("idempotency.label: want spex:E1 (fold's removed event, not derived from any op in this batch), got %+v", op.Idempotency)
	}
	var foundLineage bool
	for _, d := range op.Deps {
		if d.Kind == RefBead && d.BeadID == "spexmachina-old" && d.EdgeType == "blocks" {
			foundLineage = true
		}
	}
	if !foundLineage {
		t.Errorf("deps: want ref:bead spexmachina-old type:blocks, got %+v", op.Deps)
	}
	if op.Priority != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d, got %d", FallbackPriority, op.Priority)
	}
}

// TestBuild_BodyLinksSpecFiles covers the impl spec's "Title and Body"
// contract: a create op's body is a markdown blob linking the node's spec
// files (content leaf + module.json), which the adapter passes through to
// the tracker's description field.
func TestBuild_BodyLinksSpecFiles(t *testing.T) {
	env := newBuilderEnv()
	env.graph.paths["c1"] = NodePaths{
		Content: "spec/emit/arch_foo.md",
		Module:  "spec/emit/module.json",
	}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("c1", "emit", "Foo", nil),
		},
	}
	cs, err := env.build(report, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	op := findOp(t, cs.Ops, "c1")
	if !strings.Contains(op.Body, "spec/emit/arch_foo.md") {
		t.Errorf("body must link the content leaf, got %q", op.Body)
	}
	if !strings.Contains(op.Body, "spec/emit/module.json") {
		t.Errorf("body must link module.json, got %q", op.Body)
	}
}

// TestBuild_BodyEmptyWithoutSpecPaths pins the two empty-body cases: the
// proposal epic (no on-disk content leaf) and cleanup ops (body empty per
// arch_changeset_builder.md "Cleanup op shape"), plus creates whose node
// is unknown to the spec graph.
func TestBuild_BodyEmptyWithoutSpecPaths(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("nopath", "m", "X", nil),
			{
				Type:       "create",
				Module:     "m",
				Node:       "Y",
				NodeType:   "component",
				SpecNodeID: "cleanup1",
				OldBeadID:  "spexmachina-old",
				Reason:     "Code cleanup: m/Y",
			},
		},
		Obsoletes: []impact.Action{
			{
				Type:       "obsolete",
				BeadID:     "spexmachina-old",
				Module:     "m",
				Node:       "Y",
				NodeType:   "component",
				ChangeType: "removed",
				Reason:     "Spec node removed: m/Y",
			},
		},
	}
	cs, err := env.build(report, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, id := range []string{"p", "nopath", "cleanup1"} {
		if op := findOp(t, cs.Ops, id); op.Body != "" {
			t.Errorf("op %s: want empty body, got %q", id, op.Body)
		}
	}
}

// TestBuild_LabelsDeriveIndependentlyPerAction covers
// arch_idempotency_labeler.md's "per-action, not per-batch" rule: Labeler
// carries no cursor or counter, so each create's label is computed solely
// from that create's own referent — never leaked from, or into, another
// action's label in the same batch. Build a batch with a cleanup create
// alongside a fresh create and assert each label matches its own referent
// formula and the two never collide, whatever op_ids Sorter/Builder happen
// to assign them.
func TestBuild_LabelsDeriveIndependentlyPerAction(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "m",
				Node:       "Cleanup",
				NodeType:   "component",
				SpecNodeID: "spec_cleanup",
				OldBeadID:  "spexmachina-old",
				Reason:     "Code cleanup: m/Cleanup",
			},
			sampleComponentCreate("spec_fresh", "m", "Fresh", nil),
		},
		Obsoletes: []impact.Action{
			{
				Type:       "obsolete",
				BeadID:     "spexmachina-old",
				Module:     "m",
				Node:       "Cleanup",
				NodeType:   "component",
				ChangeType: "removed",
				Reason:     "Spec node removed: m/Cleanup",
			},
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	freshOp := findOp(t, cs.Ops, "spec_fresh")
	wantFresh := "spex:h:" + freshOp.OpID
	if freshOp.Idempotency == nil || freshOp.Idempotency.Label != wantFresh {
		t.Errorf("fresh.idempotency.label: want %q (own op_id), got %+v", wantFresh, freshOp.Idempotency)
	}

	var closeOp Op
	for _, o := range cs.Ops {
		if o.Type == OpClose {
			closeOp = o
			break
		}
	}
	if closeOp.OpID == "" {
		t.Fatal("no close op found for spexmachina-old")
	}
	cleanupOp := findOp(t, cs.Ops, "spec_cleanup")
	wantCleanup := "spex:h:" + closeOp.OpID
	if cleanupOp.Idempotency == nil || cleanupOp.Idempotency.Label != wantCleanup {
		t.Errorf("cleanup.idempotency.label: want %q (same-batch close op's id), got %+v", wantCleanup, cleanupOp.Idempotency)
	}
	if cleanupOp.Idempotency.Label == freshOp.Idempotency.Label {
		t.Errorf("cleanup and fresh labels must not collide, both got %q", cleanupOp.Idempotency.Label)
	}
}

// TestBuild_CycleErrorAborts covers the spec scenario:
// "Constructed impact report with in-batch cycle (A uses B, B uses A).
// Assert ChangesetBuilder returns a structured error naming the cycle; no
// partial changeset written."
func TestBuild_CycleErrorAborts(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("A", "m", "A", []string{"B"}),
			sampleComponentCreate("B", "m", "B", []string{"A"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err == nil {
		t.Fatalf("want cycle error, got nil and %d ops", len(cs.Ops))
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error must mention cycle, got: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("partial changeset emitted on error: %d ops", len(cs.Ops))
	}
}

// TestBuild_ExistingProposalEpicResolvesToRefBead covers the re-run path:
// when the mapping store already has an open epic record for the proposal,
// every non-epic create's parent points at that bead, and no new epic op
// is synthesized.
func TestBuild_ExistingProposalEpicResolvesToRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p-ref"] = FoldEntry{TaskID: "spexmachina-existing-epic"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p-ref", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, op := range cs.Ops {
		if op.SpecNodeKind == "proposal_epic" {
			t.Fatal("re-run must not synthesize a new proposal_epic op")
		}
	}
	x := findOp(t, cs.Ops, "X")
	if x.Parent == nil || x.Parent.Kind != RefBead || x.Parent.BeadID != "spexmachina-existing-epic" {
		t.Errorf("X parent: want ref:bead spexmachina-existing-epic, got %+v", x.Parent)
	}
}

// TestBuild_LegacyEpicNoRegistrationResolvesToRefBead covers the legacy
// shape from the spec's "Proposal epic parents every non-epic create"
// scenario: the fold pairs an epic task with the proposal, but the run
// carries no registration at all — a lifecycle that predates the
// registered event entirely. The same assertion as the plain re-run case
// holds: parents are ref:bead and no error is raised, because the fold is
// asked first and a live epic task settles the question before the
// registration is even consulted.
func TestBuild_LegacyEpicNoRegistrationResolvesToRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p-ref"] = FoldEntry{TaskID: "spexmachina-legacy-epic"}
	env.reg = &Registration{} // no registration in the journal at all
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p-ref", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, op := range cs.Ops {
		if op.SpecNodeKind == "proposal_epic" {
			t.Fatal("legacy epic (fold-paired, unregistered) must not synthesize a new proposal_epic op")
		}
	}
	x := findOp(t, cs.Ops, "X")
	if x.Parent == nil || x.Parent.Kind != RefBead || x.Parent.BeadID != "spexmachina-legacy-epic" {
		t.Errorf("X parent: want ref:bead spexmachina-legacy-epic, got %+v", x.Parent)
	}
}

// TestBuild_NoEpicNoRegistrationIsError covers the spec's fourth case: no
// epic pairing in the fold and no registration for the proposal in the
// journal. Builder.Build() returns an error naming the slug — registration
// opens the lifecycle, so the fix is `spex register`, not a synthesized
// epic. No epic op is added to the batch (unlike the registered case), so
// the error comes from the first non-epic create's own parent resolution,
// and no partial changeset is returned.
func TestBuild_NoEpicNoRegistrationIsError(t *testing.T) {
	env := newBuilderEnv()
	env.reg = &Registration{}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "never-registered", "h")
	if err == nil {
		t.Fatalf("Build: want error naming the unregistered proposal, got nil and %d ops", len(cs.Ops))
	}
	if !strings.Contains(err.Error(), "never-registered") {
		t.Errorf("error must name the unregistered proposal slug: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset on registration error: %d ops", len(cs.Ops))
	}
}

// TestBuild_ClosedProposalEpicSynthesizesFreshEpicParent covers the
// regression from PR #217: a closed proposal-epic fold entry (a prior
// run's epic bead was closed, so the journal's latest state for the
// proposal slug is Removed) leaves no live epic, so hasExistingEpic
// treats this as a new proposal and Build synthesizes a fresh
// proposal_epic op. Every non-epic create must parent under the freshly
// synthesized epic op, not an empty bead_id.
func TestBuild_ClosedProposalEpicSynthesizesFreshEpicParent(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p"] = FoldEntry{Removed: true}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	epic := findOp(t, cs.Ops, "p")
	if epic.SpecNodeKind != "proposal_epic" {
		t.Fatalf("want synthesized proposal_epic op for %q, got %+v", "p", epic)
	}
	x := findOp(t, cs.Ops, "X")
	if x.Parent == nil || x.Parent.Kind != RefOp || x.Parent.OpID != epic.OpID {
		t.Errorf("X parent: want ref:op %s (fresh epic), got %+v", epic.OpID, x.Parent)
	}
}

// TestBuild_EmptyReportWithExistingEpic covers the edge case from the spec:
// "Empty impact report → changeset with only the proposal epic op (if any)
// or an empty op list." With an existing epic in the mapping store and no
// actions, the result is empty.
func TestBuild_EmptyReportWithExistingEpic(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p-ref"] = FoldEntry{TaskID: "br-epic"}
	cs, err := env.build(impact.ImpactReport{}, "p-ref", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("empty report + existing epic: want 0 ops, got %d", len(cs.Ops))
	}
}

// TestBuild_EmptyReportNewProposalSynthesizesEpic covers the other half of
// the empty-report edge case: with no existing epic the only op is the
// synthesized proposal epic.
func TestBuild_EmptyReportNewProposalSynthesizesEpic(t *testing.T) {
	env := newBuilderEnv()
	cs, err := env.build(impact.ImpactReport{}, "p-ref", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 1 {
		t.Fatalf("want 1 op (proposal_epic), got %d (%+v)", len(cs.Ops), cs.Ops)
	}
	if cs.Ops[0].SpecNodeKind != "proposal_epic" {
		t.Errorf("op kind: want proposal_epic, got %s", cs.Ops[0].SpecNodeKind)
	}
}

// TestBuild_OnlyClosesNoCreates covers the spec edge case:
// "Impact report with only closes (no creates) → no parent/dep resolution
// needed." With an existing epic, the changeset contains only close ops.
func TestBuild_OnlyClosesNoCreates(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p"] = FoldEntry{TaskID: "br-epic"}
	report := impact.ImpactReport{
		Obsoletes: []impact.Action{
			{Type: "obsolete", BeadID: "br-1", Module: "m", Node: "A", NodeType: "component", Reason: "removed"},
			{Type: "obsolete", BeadID: "br-2", Module: "m", Node: "B", NodeType: "component", Reason: "removed"},
		},
	}
	cs, err := env.build(report, "p", "head1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 2 {
		t.Fatalf("want 2 close ops, got %d", len(cs.Ops))
	}
	for _, op := range cs.Ops {
		if op.Type != OpClose {
			t.Errorf("op %s: want type=close, got %s", op.OpID, op.Type)
		}
	}
}

// TestBuild_BatchOpenBeadOverridesClosed covers the spec edge case:
// "DepSpecNodeIDs with a spec_node_id that is itself a closed bead AND a
// fresh create in the same batch → the open in-batch op wins (ref:op) over
// the closed bead (skipped)."
func TestBuild_BatchOpenBeadOverridesClosed(t *testing.T) {
	env := newBuilderEnv()
	env.fold["Y"] = FoldEntry{Removed: true}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("Y", "m", "Y", nil),
			sampleComponentCreate("X", "m", "X", []string{"Y"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	y := findOp(t, cs.Ops, "Y")
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0].Kind != RefOp || x.Deps[0].OpID != y.OpID {
		t.Errorf("X.deps: want [ref:op %s], got %+v", y.OpID, x.Deps)
	}
}

// TestBuild_IdempotencyLabelsMatchOwnOpID covers the integration with
// IdempotencyLabeler: every node-bearing create op's idempotency.label is
// spex:<git_head>:<its own op_id> — read off the op itself. The synthesized
// epic is the one exception: its label is spex:<eid> of the run's
// registration, not derived from git_head/op_id at all.
func TestBuild_IdempotencyLabelsMatchOwnOpID(t *testing.T) {
	env := newBuilderEnv()
	b := &Builder{
		SpecGraph:    env.graph,
		Fold:         env.fold,
		Registration: Registration{EID: "h:p", OK: true},
		GitHead:      "h",
		Proposal:     "p",
	}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("a", "m", "a", nil),
			sampleComponentCreate("b", "m", "b", nil),
		},
	}
	cs, err := b.Build(report)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Three creates total: synthesized epic + a + b.
	if len(cs.Ops) != 3 {
		t.Fatalf("want 3 ops, got %d", len(cs.Ops))
	}
	if cs.Ops[0].Idempotency == nil || cs.Ops[0].Idempotency.Label != "spex:h:p" {
		t.Errorf("epic label: want spex:h:p (registration eid), got %+v", cs.Ops[0].Idempotency)
	}
	for _, op := range cs.Ops[1:] {
		want := "spex:h:" + op.OpID
		if op.Idempotency == nil || op.Idempotency.Label != want {
			t.Errorf("op %s label: want %s (own op_id), got %+v", op.OpID, want, op.Idempotency)
		}
	}
}

// TestBuild_OpFieldOrderInJSON asserts the canonical field order on a
// create op. Marshal a single-op changeset and grep the offsets of each
// field name in the JSON.
func TestBuild_OpFieldOrderInJSON(t *testing.T) {
	env := newBuilderEnv()
	env.fold["p"] = FoldEntry{TaskID: "br-epic"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", nil),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw, err := json.MarshalIndent(cs.Ops[0], "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	canonicalFields := []string{
		`"op_id"`,
		`"type"`,
		`"spec_node_kind"`,
		`"spec_node_id"`,
		`"idempotency"`,
		`"parent"`,
		`"priority"`,
	}
	prev := -1
	for _, f := range canonicalFields {
		idx := strings.Index(got, f)
		if idx < 0 {
			t.Errorf("field %s missing in op JSON:\n%s", f, got)
			continue
		}
		if idx <= prev {
			t.Errorf("field %s out of order in op JSON:\n%s", f, got)
		}
		prev = idx
	}
}

// opIndex returns the position of the op targeting specNodeID, or -1.
func opIndex(ops []Op, specNodeID string) int {
	for i, op := range ops {
		if op.SpecNodeID == specNodeID {
			return i
		}
	}
	return -1
}

// crossComponentEnv seeds the fixture shared by the byte-identical
// cross-component scenario: a fresh proposal, four component creates (x1
// depending on in-batch y1 and existing-bead z-open; aa1/ab1 independent
// for the lex tiebreak), and a mapping store with one open record for
// z-open. No unresolvable dep here — that is a separate emit-error scenario
// (TestBuild_UnresolvableDepIsError,
// TestBuild_CrossComponent_UnresolvableDepAbortsWithNoPartialChangeset).
func crossComponentEnv() (*builderEnv, impact.ImpactReport) {
	env := newBuilderEnv()
	env.fold["z-open"] = FoldEntry{TaskID: "br-z"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("x1", "m", "X", []string{"y1", "z-open"}),
			sampleComponentCreate("y1", "m", "Y", nil),
			sampleComponentCreate("ab1", "m", "AB", nil),
			sampleComponentCreate("aa1", "m", "AA", nil),
		},
	}
	return env, report
}

// TestBuild_CrossComponent_ByteIdenticalAcrossRuns covers the spec's
// "Resolver + Sorter + Labeler + Builder produce byte-identical output
// across runs" scenario. Two independently constructed Builders (fresh
// stores, fresh labelers — no shared in-memory state) over identical
// inputs must serialize to byte-identical JSON. Resolver classifies both
// v2 dep shapes, TopologicalSorter orders epic-first with lex tiebreak,
// IdempotencyLabeler stamps each op's label from its own (git_head, op_id),
// and ChangesetBuilder composes the canonical output.
func TestBuild_CrossComponent_ByteIdenticalAcrossRuns(t *testing.T) {
	env1, report1 := crossComponentEnv()
	cs1, err := env1.build(report1, "prop", "deadbeefcafe")
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	env2, report2 := crossComponentEnv()
	cs2, err := env2.build(report2, "prop", "deadbeefcafe")
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	json1, err := json.MarshalIndent(cs1, "", "  ")
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	json2, err := json.MarshalIndent(cs2, "", "  ")
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(json1, json2) {
		t.Fatalf("changesets not byte-identical:\n%s\n---\n%s", json1, json2)
	}

	// Sorter: epic first; lex tiebreak among independent peers aa1 < ab1
	// < y1; dependent x1 after its in-batch predecessor y1.
	if cs1.Ops[0].SpecNodeKind != "proposal_epic" {
		t.Fatalf("op[0]: want proposal_epic first, got %+v", cs1.Ops[0])
	}
	iAA, iAB, iY, iX := opIndex(cs1.Ops, "aa1"), opIndex(cs1.Ops, "ab1"), opIndex(cs1.Ops, "y1"), opIndex(cs1.Ops, "x1")
	if !(iAA < iAB && iAB < iY && iY < iX) {
		t.Errorf("sort order: want aa1 < ab1 < y1 < x1, got indices %d %d %d %d", iAA, iAB, iY, iX)
	}

	// Labeler: each node-bearing op's label is spex:<git_head>:<its own
	// op_id>; the epic's is spex:<eid> of the run's registration
	// (deadbeefcafe:prop, env.build's default).
	if cs1.Ops[0].Idempotency == nil || cs1.Ops[0].Idempotency.Label != "spex:deadbeefcafe:prop" {
		t.Errorf("epic label: want spex:deadbeefcafe:prop (registration eid), got %+v", cs1.Ops[0].Idempotency)
	}
	for _, op := range cs1.Ops[1:] {
		want := "spex:deadbeefcafe:" + op.OpID
		if op.Idempotency == nil || op.Idempotency.Label != want {
			t.Errorf("op %s label: want %s (own op_id), got %+v", op.OpID, want, op.Idempotency)
		}
	}

	// Resolver: the two v2 dep shapes on x1, input order preserved.
	x := findOp(t, cs1.Ops, "x1")
	if len(x.Deps) != 2 {
		t.Fatalf("x1 deps: want 2, got %+v", x.Deps)
	}
	y := findOp(t, cs1.Ops, "y1")
	if x.Deps[0].Kind != RefOp || x.Deps[0].OpID != y.OpID {
		t.Errorf("x1 dep[0]: want ref:op %s, got %+v", y.OpID, x.Deps[0])
	}
	if x.Deps[1].Kind != RefBead || x.Deps[1].BeadID != "br-z" {
		t.Errorf("x1 dep[1]: want ref:bead br-z, got %+v", x.Deps[1])
	}
}

// TestBuild_CrossComponent_DepClassificationRoundTrip covers the spec's
// "dep classification round-trip through Builder" scenario: one dependent
// create whose deps array carries both v2 ref shapes — ref:op and
// ref:bead, neither flattened or coerced — with the in-batch predecessor
// sequenced earlier by the Sorter.
func TestBuild_CrossComponent_DepClassificationRoundTrip(t *testing.T) {
	env := newBuilderEnv()
	env.fold["q-open"] = FoldEntry{TaskID: "br-q"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("d1", "m", "D", []string{"p1", "q-open"}),
			sampleComponentCreate("p1", "m", "P", nil),
		},
	}
	cs, err := env.build(report, "prop", "deadbeefcafe")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	d := findOp(t, cs.Ops, "d1")
	p := findOp(t, cs.Ops, "p1")
	if len(d.Deps) != 2 {
		t.Fatalf("d1 deps: want exactly 2 shapes, got %+v", d.Deps)
	}
	if d.Deps[0].Kind != RefOp || d.Deps[0].OpID != p.OpID || d.Deps[0].BeadID != "" {
		t.Errorf("dep[0]: want pure ref:op %s, got %+v", p.OpID, d.Deps[0])
	}
	if d.Deps[1].Kind != RefBead || d.Deps[1].BeadID != "br-q" || d.Deps[1].OpID != "" {
		t.Errorf("dep[1]: want pure ref:bead br-q, got %+v", d.Deps[1])
	}
	if opIndex(cs.Ops, "p1") >= opIndex(cs.Ops, "d1") {
		t.Errorf("sorter must sequence in-batch predecessor p1 before dependent d1: %+v", cs.Ops)
	}
}

// TestBuild_CrossComponent_UnresolvableDepAbortsWithNoPartialChangeset
// covers the same scenario's unresolvable-dep-present half: the same
// batch, with an additional dep (r-none) that is neither in-batch nor in
// the fold. Build() must error naming it, and no partial changeset comes
// back.
func TestBuild_CrossComponent_UnresolvableDepAbortsWithNoPartialChangeset(t *testing.T) {
	env := newBuilderEnv()
	env.fold["q-open"] = FoldEntry{TaskID: "br-q"}
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("d1", "m", "D", []string{"p1", "q-open", "r-none"}),
			sampleComponentCreate("p1", "m", "P", nil),
		},
	}
	cs, err := env.build(report, "prop", "deadbeefcafe")
	if err == nil {
		t.Fatal("Build: want error for unresolvable dep r-none, got nil")
	}
	if !strings.Contains(err.Error(), "r-none") {
		t.Errorf("error must name the unresolvable spec_node_id r-none: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset on unresolvable dep: got %d ops", len(cs.Ops))
	}
}

// TestBuild_CrossComponent_LabelsPairWithTopoOrder covers the spec's
// "idempotency label reservation paired with sort order" scenario, updated
// for the store-free Labeler: an A → B → C in-batch chain labels the topo
// order A, B, C as spex:<head>:op-1/op-2/op-3. There is no store-derived
// cursor left to vary — Labeler reads each label off its own op's
// (git_head, op_id) — so two independently constructed envs over identical
// inputs must still pair identically, proving the label depends on the op
// alone. An existing proposal epic keeps the epic op out of the batch,
// matching the spec's three-label expectation.
func TestBuild_CrossComponent_LabelsPairWithTopoOrder(t *testing.T) {
	run := func() Changeset {
		env := newBuilderEnv()
		env.fold["prop"] = FoldEntry{TaskID: "br-epic"}
		report := impact.ImpactReport{
			Creates: []impact.Action{
				sampleComponentCreate("cC", "m", "C", []string{"cB"}),
				sampleComponentCreate("cB", "m", "B", []string{"cA"}),
				sampleComponentCreate("cA", "m", "A", nil),
			},
		}
		cs, err := env.build(report, "prop", "deadbeefcafe")
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return cs
	}

	assertPairing := func(cs Changeset) {
		t.Helper()
		wantOrder := []string{"cA", "cB", "cC"}
		if len(cs.Ops) != 3 {
			t.Fatalf("ops: want 3 (existing epic emits no op), got %+v", cs.Ops)
		}
		for i, id := range wantOrder {
			if cs.Ops[i].SpecNodeID != id {
				t.Errorf("op[%d]: want %s (topo order A,B,C), got %s", i, id, cs.Ops[i].SpecNodeID)
			}
			want := "spex:deadbeefcafe:" + cs.Ops[i].OpID
			if cs.Ops[i].Idempotency == nil || cs.Ops[i].Idempotency.Label != want {
				t.Errorf("op[%d] label: want %s, got %+v", i, want, cs.Ops[i].Idempotency)
			}
		}
	}

	assertPairing(run())
	assertPairing(run())
}

// TestBuild_CrossComponent_CycleErrorNamesBothNodes covers the spec's
// "cycle detection surfaces through Builder.Build error" scenario:
// Resolver classifies both deps in-batch, Sorter's Kahn pass detects the
// cycle, and Builder propagates a structured error naming BOTH
// spec_node_ids with no partial changeset.
func TestBuild_CrossComponent_CycleErrorNamesBothNodes(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("cycA", "m", "A", []string{"cycB"}),
			sampleComponentCreate("cycB", "m", "B", []string{"cycA"}),
		},
	}
	cs, err := env.build(report, "prop", "deadbeefcafe")
	if err == nil {
		t.Fatal("want cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error must mention the cycle: %v", err)
	}
	if !strings.Contains(err.Error(), "cycA") || !strings.Contains(err.Error(), "cycB") {
		t.Errorf("error must name both spec_node_ids in the cycle: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset on cycle: got %d ops", len(cs.Ops))
	}
}
