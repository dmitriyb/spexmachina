package emit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
)

// builderEnv bundles the fakes a Builder test typically needs.
type builderEnv struct {
	store *fakeStore
	graph *fakeSpecGraph
}

func newBuilderEnv() *builderEnv {
	return &builderEnv{
		store: newFakeStore(),
		graph: newFakeSpecGraph(),
	}
}

func (e *builderEnv) build(report impact.ImpactReport, proposal, head string) (Changeset, error) {
	b := &Builder{
		SpecGraph:    e.graph,
		MappingStore: e.store,
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
// has version: 1 at the top, git_head set to the fixed SHA, and canonical
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
	if cs.Version != 1 {
		t.Errorf("version: want 1, got %d", cs.Version)
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
// "Impact report with one component create, one data_flow create. Assert:
// first op is type: create with spec_node_kind: proposal_epic; following
// two creates' parent fields are {ref:op, op_id:<epic op_id>}."
func TestBuild_ProposalEpicIsFirstAndParents(t *testing.T) {
	env := newBuilderEnv()
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
	env.store.bySpecNode["Y"] = []mapping.Record{
		{ID: 1, SpecNodeID: "Y", BeadID: "br-Y", BeadStatus: "open"},
	}
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
	env.store.bySpecNode["Y"] = []mapping.Record{
		{ID: 1, SpecNodeID: "Y", BeadID: "br-Y", BeadStatus: "closed"},
	}
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

// TestBuild_SpecNodeFallback covers the spec scenario:
// "Z has no mapping record and no in-batch op. Assert X's deps include
// {ref:spec_node, spec_node_id:Z}."
func TestBuild_SpecNodeFallback(t *testing.T) {
	env := newBuilderEnv()
	report := impact.ImpactReport{
		Creates: []impact.Action{
			sampleComponentCreate("X", "m", "X", []string{"Z"}),
		},
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0].Kind != RefSpecNode || x.Deps[0].SpecNodeID != "Z" {
		t.Fatalf("X.deps: want [ref:spec_node Z], got %+v", x.Deps)
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
// Also covers the modify-pair record-id-reuse rule from
// arch_idempotency_labeler.md: the create op's idempotency.label MUST
// equal the existing record's id (looked up via OldBeadID), NOT a
// freshly-reserved sequential label. Without this, Reconciler treats the
// create as a fresh insert at a new recID and invariant 3 fails.
func TestBuild_ObsoleteAndCreateLineage(t *testing.T) {
	env := newBuilderEnv()
	// Seed an existing record whose bead_id matches OldBeadID. Labeler's
	// modify-pair branch reuses this record's id (42) rather than the
	// cursor (which would otherwise return 1 from the empty store's
	// default).
	const existingRecordID = 42
	env.store.byBead["spexmachina-abc"] = mapping.Record{
		ID:         existingRecordID,
		BeadID:     "spexmachina-abc",
		SpecNodeID: "Q",
	}
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

	// Modify-pair label reuse: the create's idempotency.label must equal
	// the existing record's id, not a fresh sequential value.
	wantLabel := "spex:42"
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (existing record id), got %+v", wantLabel, q.Idempotency)
	}
}

// TestBuild_CleanupOpShape covers the spec scenario from
// arch_changeset_builder.md "Cleanup op shape": an action with Reason
// starting "Code cleanup:" produces an op with spec_node_kind="cleanup",
// title=Reason verbatim, labels=[spex:cleanup], idempotency=spex:cleanup-
// <spec_node_id>, deps carrying the OldBeadID lineage, priority=
// FallbackPriority.
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
	if op.Idempotency == nil || op.Idempotency.Label != "spex:cleanup-abc123def456" {
		t.Errorf("idempotency.label: want spex:cleanup-abc123def456, got %+v", op.Idempotency)
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

// TestBuild_CleanupDoesNotAdvanceCursor covers the spec scenario:
// "Cursor non-advancement: build a changeset containing one cleanup
// create AND one fresh component create. Assert the fresh create's
// spex:<n> label uses the cursor value the Labeler would have returned
// WITHOUT the cleanup op present."
func TestBuild_CleanupDoesNotAdvanceCursor(t *testing.T) {
	env := newBuilderEnv()
	env.store.nextID = 100
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
	}
	cs, err := env.build(report, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Three creates expected: the proposal epic (spex:100), then either
	// cleanup or fresh in some order. The fresh-create label MUST be
	// spex:101 (the cursor value right after the epic), not spex:102 —
	// proving the cleanup op did not advance the cursor.
	freshOp := findOp(t, cs.Ops, "spec_fresh")
	if freshOp.Idempotency == nil || freshOp.Idempotency.Label != "spex:101" {
		t.Errorf("fresh.idempotency.label: want spex:101 (cleanup did not advance cursor), got %+v", freshOp.Idempotency)
	}
	cleanupOp := findOp(t, cs.Ops, "spec_cleanup")
	if cleanupOp.Idempotency == nil || cleanupOp.Idempotency.Label != "spex:cleanup-spec_cleanup" {
		t.Errorf("cleanup.idempotency.label: want spex:cleanup-spec_cleanup, got %+v", cleanupOp.Idempotency)
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
	env.store.epic["p-ref"] = mapping.Record{
		ID:         42,
		SpecNodeID: "p-ref",
		BeadID:     "spexmachina-existing-epic",
		NodeType:   "proposal",
		BeadStatus: "open",
	}
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

// TestBuild_EmptyReportWithExistingEpic covers the edge case from the spec:
// "Empty impact report → changeset with only the proposal epic op (if any)
// or an empty op list." With an existing epic in the mapping store and no
// actions, the result is empty.
func TestBuild_EmptyReportWithExistingEpic(t *testing.T) {
	env := newBuilderEnv()
	env.store.epic["p-ref"] = mapping.Record{
		ID:         1,
		SpecNodeID: "p-ref",
		BeadID:     "br-epic",
		NodeType:   "proposal",
		BeadStatus: "open",
	}
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
	env.store.epic["p"] = mapping.Record{
		ID:         1,
		SpecNodeID: "p",
		BeadID:     "br-epic",
		NodeType:   "proposal",
		BeadStatus: "open",
	}
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
	env.store.bySpecNode["Y"] = []mapping.Record{
		{ID: 1, SpecNodeID: "Y", BeadID: "br-old", BeadStatus: "closed"},
	}
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

// TestBuild_IdempotencyLabelsAreSequential covers the integration with
// IdempotencyLabeler: every create op carries a spex:N label and labels
// run sequentially starting from the mapping store's NextRecordID.
func TestBuild_IdempotencyLabelsAreSequential(t *testing.T) {
	env := newBuilderEnv()
	env.store.nextID = 100
	b := &Builder{
		SpecGraph:    env.graph,
		MappingStore: env.store,
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
	want := []string{"spex:100", "spex:101", "spex:102"}
	for i, op := range cs.Ops {
		if op.Idempotency == nil {
			t.Errorf("op %d: missing idempotency", i)
			continue
		}
		if op.Idempotency.Label != want[i] {
			t.Errorf("op %d label: want %s, got %s", i, want[i], op.Idempotency.Label)
		}
	}
}

// TestBuild_OpFieldOrderInJSON asserts the canonical field order on a
// create op. Marshal a single-op changeset and grep the offsets of each
// field name in the JSON.
func TestBuild_OpFieldOrderInJSON(t *testing.T) {
	env := newBuilderEnv()
	env.store.epic["p"] = mapping.Record{
		ID: 1, SpecNodeID: "p", BeadID: "br-epic", NodeType: "proposal", BeadStatus: "open",
	}
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

