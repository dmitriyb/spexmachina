package plan

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// fakeBuilderFold combines fakeFold (Resolver's dep/parent lookups, from
// resolver_test.go) and a removal map (IdempotencyLabeler's cleanup
// referent lookup, from labeler_test.go) into the single plan.Fold a
// Builder test seeds in one place (test_changeset_builder.md, "Fixtures":
// "a builder environment wrapping a fake journal fold").
type fakeBuilderFold struct {
	fakeFold
	removals fakeRemovalFold
}

func newFakeBuilderFold() fakeBuilderFold {
	return fakeBuilderFold{fakeFold: fakeFold{}, removals: fakeRemovalFold{}}
}

func (f fakeBuilderFold) Removal(specNodeID string) (RemovalEntry, bool) {
	e, ok := f.removals[specNodeID]
	return e, ok
}

// builderEnv bundles the fakes a Builder test typically needs: a fake
// journal fold and a spec graph, plus a per-scenario registration seeded
// independently of the fold so the epic verdicts can be told apart.
//
// reg is a pointer so most scenarios can leave it unset: build() then
// defaults it to a present registration keyed off the run's own
// proposal/head, letting a plain "new proposal" fixture synthesize an epic
// without spelling out a registration event by hand. Scenarios exercising
// the registration-absent path set reg explicitly to override the default.
type builderEnv struct {
	fold  fakeBuilderFold
	graph SpecGraph
	reg   *Registration
}

func newBuilderEnv() *builderEnv {
	return &builderEnv{
		fold:  newFakeBuilderFold(),
		graph: NewSpecGraph(schema.Project{}, map[string]schema.ModuleSpec{}),
	}
}

func (e *builderEnv) build(actions []Action, proposal, head string) (Changeset, error) {
	reg := e.reg
	if reg == nil {
		reg = &Registration{EID: head + ":" + proposal}
	}
	b := &Builder{
		SpecGraph:    e.graph,
		Fold:         e.fold,
		Registration: *reg,
		GitHead:      head,
		Proposal:     proposal,
	}
	return b.Build(actions)
}

func sampleComponentCreate(specID, module, node string, deps []string) Action {
	return Action{
		Type:           ActionCreate,
		Module:         module,
		Node:           node,
		NodeType:       KindComponent,
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

func opIndex(ops []Op, specNodeID string) int {
	for i, op := range ops {
		if op.SpecNodeID == specNodeID {
			return i
		}
	}
	return -1
}

// --- Canonical schema and field order ---

// canonicalOpFieldOrder is the wire field order the Op struct's tags pin
// (types.go): op_id, type, spec_node_kind, spec_node_id, spec_hash,
// idempotency, parent, deps, priority, title, body, target, labels,
// reason. A create, retarget or close op each surface only the subset of
// fields their shape uses — assertCanonicalFieldOrder checks that
// whichever fields are present appear in this relative order, since one
// order governs every op kind.
var canonicalOpFieldOrder = []string{
	`"op_id"`, `"type"`, `"spec_node_kind"`, `"spec_node_id"`, `"spec_hash"`,
	`"idempotency"`, `"parent"`, `"deps"`, `"priority"`, `"title"`, `"body"`,
	`"target"`, `"labels"`, `"reason"`,
}

// assertCanonicalFieldOrder marshals op and checks that whichever of
// canonicalOpFieldOrder's fields the wire JSON carries appear in that
// relative order; fields the op's shape omits are skipped, not required.
func assertCanonicalFieldOrder(t *testing.T, op Op) {
	t.Helper()
	raw, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	prev, prevField := -1, ""
	for _, f := range canonicalOpFieldOrder {
		idx := strings.Index(got, f)
		if idx < 0 {
			continue
		}
		if idx <= prev {
			t.Errorf("field %s out of order (want after %s) in op JSON:\n%s", f, prevField, got)
		}
		prev, prevField = idx, f
	}
}

func TestBuild_CanonicalSchemaAndFieldOrder(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{sampleComponentCreate("c1", "plan", "Foo", nil)}

	cs, err := env.build(actions, "p", "deadbeefcafe1234")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cs.Version != 4 {
		t.Errorf("version: want 4, got %d", cs.Version)
	}
	if cs.GitHead != "deadbeefcafe1234" {
		t.Errorf("git_head: want deadbeefcafe1234, got %q", cs.GitHead)
	}

	assertCanonicalFieldOrder(t, cs.Ops[0])

	raw, err := json.Marshal(cs.Ops[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"spec_hash"`) {
		t.Errorf("create op: want no spec_hash key (only retargets populate it), got %s", raw)
	}
}

// TestBuild_CanonicalFieldOrderMixedBatch runs the field-order assertion
// over a batch carrying one create, one retarget and one close, and pins
// that the retarget's deps are serialized before its target: one order
// governs every op kind, and a retarget is where a per-kind order would
// show itself (test_changeset_builder.md, "Canonical schema and field
// order").
func TestBuild_CanonicalFieldOrderMixedBatch(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["w-open"] = Pairing{TaskID: "spexmachina-w", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("c1", "m", "C1", nil),
		{
			Type:           ActionRetarget,
			TaskID:         "spexmachina-hun",
			Module:         "m",
			Node:           "X",
			NodeType:       KindComponent,
			SpecNodeID:     "x-node",
			SpecHash:       "new-hash",
			DepSpecNodeIDs: []string{"w-open"},
			Reason:         "Spec node modified (retarget): m/X",
		},
		{Type: ActionObsolete, TaskID: "spexmachina-old", Module: "m", Node: "B", NodeType: KindComponent, Reason: "removed"},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 3 {
		t.Fatalf("want 3 ops (create, retarget, close), got %d: %+v", len(cs.Ops), cs.Ops)
	}
	for _, op := range cs.Ops {
		assertCanonicalFieldOrder(t, op)
	}

	retarget := findOp(t, cs.Ops, "x-node")
	raw, err := json.Marshal(retarget)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	depsIdx := strings.Index(got, `"deps"`)
	targetIdx := strings.Index(got, `"target"`)
	if depsIdx < 0 || targetIdx < 0 || depsIdx >= targetIdx {
		t.Errorf("retarget: want deps serialized before target, got %s", got)
	}
}

// TestBuild_RefWireKeyNames pins each ref object's own key names on the
// wire: the adapter reads .op_id off an in-batch dep to resolve it against
// the ops it has already applied, so renaming the key silently resolves
// every in-batch dep to nothing rather than failing loudly. It also pins
// that no ref carries any further key — the edge-type field left the
// vocabulary with the lineage edge — so a create for a modified node
// carries no typed dep at all (test_changeset_builder.md, "Canonical
// schema and field order").
func TestBuild_RefWireKeyNames(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["y-open"] = Pairing{TaskID: "spexmachina-y", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("a1", "m", "A", nil),
		sampleComponentCreate("b1", "m", "B", []string{"a1", "y-open"}),
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	b := findOp(t, cs.Ops, "b1")
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `{"ref":"op","op_id":"`) {
		t.Errorf("in-batch dep: want literal {\"ref\":\"op\",\"op_id\":...} on the wire, got %s", got)
	}
	if !strings.Contains(got, `{"ref":"task","task_id":"spexmachina-y"}`) {
		t.Errorf("existing-task dep: want literal {\"ref\":\"task\",\"task_id\":...} on the wire, got %s", got)
	}
}

func TestBuild_DeterministicAcrossRuns(t *testing.T) {
	makeActions := func() []Action {
		return []Action{
			sampleComponentCreate("c2", "plan", "Bar", nil),
			sampleComponentCreate("c1", "plan", "Foo", nil),
			sampleComponentCreate("c3", "plan", "Baz", []string{"c1"}),
		}
	}

	env1 := newBuilderEnv()
	cs1, err := env1.build(makeActions(), "p", "abc123")
	if err != nil {
		t.Fatalf("Build #1: %v", err)
	}
	env2 := newBuilderEnv()
	cs2, err := env2.build(makeActions(), "p", "abc123")
	if err != nil {
		t.Fatalf("Build #2: %v", err)
	}

	out1, _ := json.MarshalIndent(cs1, "", "  ")
	out2, _ := json.MarshalIndent(cs2, "", "  ")
	if !bytes.Equal(out1, out2) {
		t.Errorf("non-deterministic output:\nrun1=%s\nrun2=%s", out1, out2)
	}
}

// --- Proposal epic parents every non-epic create ---

func TestBuild_EpicSynthesizedAndParentsNonEpicCreates(t *testing.T) {
	env := newBuilderEnv()
	env.reg = &Registration{EID: "reg-head1:p-ref"}
	actions := []Action{
		sampleComponentCreate("c1", "plan", "Comp", nil),
		{Type: ActionCreate, Module: "plan", Node: "Flow", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h-f1", Reason: "New spec node: plan/Flow"},
	}
	cs, err := env.build(actions, "p-ref", "head1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 3 {
		t.Fatalf("want 3 ops (epic + comp + flow), got %d", len(cs.Ops))
	}
	first := cs.Ops[0]
	if first.Type != OpCreate || first.SpecNodeKind != KindProposalEpic {
		t.Errorf("first op: want type=create kind=proposal_epic, got type=%s kind=%s", first.Type, first.SpecNodeKind)
	}
	if first.OpID != "op-proposal_epic-p-ref" {
		t.Errorf("epic op_id: want op-proposal_epic-p-ref (op-<kind>-<proposal ref>), got %q", first.OpID)
	}
	if first.Idempotency == nil || first.Idempotency.Label != "spex:reg-head1:p-ref" {
		t.Errorf("epic label: want spex:reg-head1:p-ref (registration eid, not git_head head1), got %+v", first.Idempotency)
	}
	for _, op := range cs.Ops[1:] {
		if op.Parent == nil || op.Parent.Kind != RefOp || op.Parent.OpID != first.OpID {
			t.Errorf("op %s: parent want ref:op %s, got %+v", op.OpID, first.OpID, op.Parent)
		}
	}
}

func TestBuild_ExistingEpicSkipsCreateAndParentsRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p-ref"] = Pairing{TaskID: "spexmachina-epic"}
	env.reg = &Registration{EID: "reg-head1:p-ref"}
	actions := []Action{sampleComponentCreate("c1", "plan", "Comp", nil)}

	cs, err := env.build(actions, "p-ref", "head1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, op := range cs.Ops {
		if op.SpecNodeKind == KindProposalEpic {
			t.Fatal("existing epic pairing must not synthesize a new epic op")
		}
	}
	c1 := findOp(t, cs.Ops, "c1")
	if c1.Parent == nil || c1.Parent.Kind != RefTask || c1.Parent.TaskID != "spexmachina-epic" {
		t.Errorf("c1 parent: want ref:task spexmachina-epic, got %+v", c1.Parent)
	}
}

func TestBuild_LegacyEpicNoRegistrationParentsRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p-ref"] = Pairing{TaskID: "spexmachina-legacy-epic"}
	env.reg = &Registration{} // no registration in the journal at all
	actions := []Action{sampleComponentCreate("c1", "plan", "Comp", nil)}

	cs, err := env.build(actions, "p-ref", "head1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, op := range cs.Ops {
		if op.SpecNodeKind == KindProposalEpic {
			t.Fatal("legacy epic (fold-paired, unregistered) must not synthesize a new epic op")
		}
	}
	c1 := findOp(t, cs.Ops, "c1")
	if c1.Parent == nil || c1.Parent.Kind != RefTask || c1.Parent.TaskID != "spexmachina-legacy-epic" {
		t.Errorf("c1 parent: want ref:task spexmachina-legacy-epic, got %+v", c1.Parent)
	}
}

func TestBuild_NoEpicNoRegistrationIsError(t *testing.T) {
	env := newBuilderEnv()
	env.reg = &Registration{}
	actions := []Action{sampleComponentCreate("c1", "plan", "Comp", nil)}

	cs, err := env.build(actions, "never-registered", "head1")
	if err == nil {
		t.Fatalf("want error naming the unregistered proposal, got nil and %d ops", len(cs.Ops))
	}
	if !strings.Contains(err.Error(), "never-registered") {
		t.Errorf("error must name the unregistered proposal slug: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset on registration error: %d ops", len(cs.Ops))
	}
}

// --- In-batch dep chain resolves to ref:op ---

func TestBuild_InBatchDepChainResolvesToRefOp(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"} // no epic op in the batch
	actions := []Action{
		sampleComponentCreate("A", "m", "A", nil),
		sampleComponentCreate("B", "m", "B", []string{"A"}),
		sampleComponentCreate("C", "m", "C", []string{"B"}),
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	a := findOp(t, cs.Ops, "A")
	b := findOp(t, cs.Ops, "B")
	c := findOp(t, cs.Ops, "C")
	if len(a.Deps) != 0 {
		t.Errorf("A.deps: want empty, got %+v", a.Deps)
	}
	if len(b.Deps) != 1 || b.Deps[0] != (Ref{Kind: RefOp, OpID: a.OpID}) {
		t.Errorf("B.deps: want [ref:op %s], got %+v", a.OpID, b.Deps)
	}
	if len(c.Deps) != 1 || c.Deps[0] != (Ref{Kind: RefOp, OpID: b.OpID}) {
		t.Errorf("C.deps: want [ref:op %s], got %+v", b.OpID, c.Deps)
	}
}

// --- Layer edges follow the profile's order ---

// TestBuild_LayerEdgesFollowProfileOrder pins
// test_changeset_builder.md's "Layer edges follow the profile's order"
// under the default profile: the epic first, then F1/F2 (lex order), then
// the component layer A/B/C, then T — with every create depending, as
// ref:op, on every create of the previous non-empty layer, and the epic
// carrying no edge of its own since it is no layer's predecessor.
func TestBuild_LayerEdgesFollowProfileOrder(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "F1", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h", Reason: "New spec node: m/F1"},
		{Type: ActionCreate, Module: "m", Node: "F2", NodeType: KindDataFlow, SpecNodeID: "f2", SpecHash: "h", Reason: "New spec node: m/F2"},
		sampleComponentCreate("a1", "m", "A", []string{"f1"}), // the data_flow add-on: F1 names A in its uses
		sampleComponentCreate("b1", "m", "B", []string{"a1"}), // B uses A
		sampleComponentCreate("c1", "m", "C", nil),
		{Type: ActionCreate, Module: "m", Node: "T", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h", DepSpecNodeIDs: []string{"b1"}, Reason: "New spec node: m/T"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 7 {
		t.Fatalf("want 7 ops (epic + 2 flows + 3 components + 1 test), got %d: %+v", len(cs.Ops), cs.Ops)
	}
	if cs.Ops[0].SpecNodeKind != KindProposalEpic {
		t.Fatalf("op[0]: want proposal_epic, got %+v", cs.Ops[0])
	}
	for i, specID := range []string{"f1", "f2", "a1", "b1", "c1", "t1"} {
		if cs.Ops[i+1].SpecNodeID != specID {
			t.Errorf("op[%d]: want spec_node_id %s, got %s", i+1, specID, cs.Ops[i+1].SpecNodeID)
		}
	}

	f1, f2 := findOp(t, cs.Ops, "f1"), findOp(t, cs.Ops, "f2")
	a, b, c, tSec := findOp(t, cs.Ops, "a1"), findOp(t, cs.Ops, "b1"), findOp(t, cs.Ops, "c1"), findOp(t, cs.Ops, "t1")

	if len(f1.Deps) != 0 || len(f2.Deps) != 0 {
		t.Errorf("F1/F2 must carry no deps (the epic is no layer's predecessor), got %+v / %+v", f1.Deps, f2.Deps)
	}
	wantA := []Ref{{Kind: RefOp, OpID: f1.OpID}, {Kind: RefOp, OpID: f2.OpID}}
	if !reflect.DeepEqual(a.Deps, wantA) {
		t.Errorf("A.deps: want %+v (F1 once, though the add-on and the layer edge both name it), got %+v", wantA, a.Deps)
	}
	wantB := []Ref{{Kind: RefOp, OpID: f1.OpID}, {Kind: RefOp, OpID: f2.OpID}, {Kind: RefOp, OpID: a.OpID}}
	if !reflect.DeepEqual(b.Deps, wantB) {
		t.Errorf("B.deps: want %+v, got %+v", wantB, b.Deps)
	}
	wantC := []Ref{{Kind: RefOp, OpID: f1.OpID}, {Kind: RefOp, OpID: f2.OpID}}
	if !reflect.DeepEqual(c.Deps, wantC) {
		t.Errorf("C.deps: want %+v, got %+v", wantC, c.Deps)
	}
	wantT := []Ref{{Kind: RefOp, OpID: a.OpID}, {Kind: RefOp, OpID: b.OpID}, {Kind: RefOp, OpID: c.OpID}}
	if !reflect.DeepEqual(tSec.Deps, wantT) {
		t.Errorf("T.deps: want the whole component layer %+v (not either flow — adjacent layers only), got %+v", wantT, tSec.Deps)
	}
}

// TestBuild_LayerEdgesSkipEmptyLayer reruns the fixture above with F1 and
// F2 struck from the batch (and, consistently, from A's own DepSpecNodeIDs
// — the add-on only ever fires when a changed data_flow is in the batch):
// A, B and C carry spec-graph deps alone, while T still carries A, B, C —
// the previous *non-empty* layer is the predecessor, and an empty layer is
// skipped, not waited on.
func TestBuild_LayerEdgesSkipEmptyLayer(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		sampleComponentCreate("a1", "m", "A", nil),
		sampleComponentCreate("b1", "m", "B", []string{"a1"}),
		sampleComponentCreate("c1", "m", "C", nil),
		{Type: ActionCreate, Module: "m", Node: "T", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h", DepSpecNodeIDs: []string{"b1"}, Reason: "New spec node: m/T"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	a, b, c, tSec := findOp(t, cs.Ops, "a1"), findOp(t, cs.Ops, "b1"), findOp(t, cs.Ops, "c1"), findOp(t, cs.Ops, "t1")
	if len(a.Deps) != 0 {
		t.Errorf("A.deps: want empty (no data_flow layer to wait on), got %+v", a.Deps)
	}
	wantB := []Ref{{Kind: RefOp, OpID: a.OpID}}
	if !reflect.DeepEqual(b.Deps, wantB) {
		t.Errorf("B.deps: want %+v (spec-graph dep alone), got %+v", wantB, b.Deps)
	}
	if len(c.Deps) != 0 {
		t.Errorf("C.deps: want empty, got %+v", c.Deps)
	}
	wantT := []Ref{{Kind: RefOp, OpID: a.OpID}, {Kind: RefOp, OpID: b.OpID}, {Kind: RefOp, OpID: c.OpID}}
	if !reflect.DeepEqual(tSec.Deps, wantT) {
		t.Errorf("T.deps: want the whole component layer %+v even with the data_flow layer empty, got %+v", wantT, tSec.Deps)
	}
}

// TestBuild_LayerEdgesRefuseForwardDepUnderReorderedProfile reruns under a
// profile whose plan-relevant list reads component, data_flow, test_section
// — swapping the default's first two entries. A now depends on F1
// (the add-on), but F1 sits in a *later* layer under this order, so
// Build() must refuse rather than emit a forward ref:op, naming both ops.
func TestBuild_LayerEdgesRefuseForwardDepUnderReorderedProfile(t *testing.T) {
	env := newBuilderEnv()
	env.graph = env.graph.WithProfile(&schema.Profile{PlanRelevant: []string{"component", "data_flow", "test_section"}})
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "F1", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h", Reason: "New spec node: m/F1"},
		sampleComponentCreate("a1", "m", "A", []string{"f1"}),
	}
	cs, err := env.build(actions, "p", "h")
	if err == nil {
		t.Fatalf("want a forward-layer-dep error, got %d ops", len(cs.Ops))
	}
	if !strings.Contains(err.Error(), "a1") || !strings.Contains(err.Error(), "f1") {
		t.Errorf("error must name both the dependent and the later-layer dep: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset: got %d ops", len(cs.Ops))
	}
}

// --- Cleanup layer waits for the batch's last layer and its retargets ---

// TestBuild_CleanupLayerWaitsForLastLayerAndRetargets pins
// test_changeset_builder.md's "Cleanup layer waits for the batch's last
// layer and its retargets": the two cleanup ops are the last creates,
// ordered by the lex tiebreak on spec_node_id, and each depends on every
// create of the nearest non-empty layer before them plus every retarget's
// target — but never on each other.
func TestBuild_CleanupLayerWaitsForLastLayerAndRetargets(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		sampleComponentCreate("compA", "m", "A", nil),
		sampleComponentCreate("compB", "m", "B", nil),
		{Type: ActionCreate, Module: "m", Node: "T", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h", DepSpecNodeIDs: []string{"compA", "compB"}, Reason: "New spec node: m/T"},
		{Type: ActionRetarget, TaskID: "spexmachina-hun", Module: "m", Node: "R", NodeType: KindComponent, SpecNodeID: "ret-node", SpecHash: "new-hash", Reason: "Spec node modified (retarget): m/R"},
		{Type: ActionCreate, Module: "m", Node: "X", NodeType: KindComponent, SpecNodeID: "aaa111", Reason: "Code cleanup: m/X"},
		{Type: ActionCreate, Module: "m", Node: "Y", NodeType: KindComponent, SpecNodeID: "bbb222", Reason: "Code cleanup: m/Y"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tSec := findOp(t, cs.Ops, "t1")
	cleanX := findOp(t, cs.Ops, "aaa111")
	cleanY := findOp(t, cs.Ops, "bbb222")

	if opIndex(cs.Ops, "aaa111") >= opIndex(cs.Ops, "bbb222") {
		t.Errorf("cleanups must sort by lex spec_node_id: want aaa111 before bbb222 in %+v", cs.Ops)
	}
	lastTwoCreates := 0
	for _, op := range cs.Ops {
		if op.Type == OpCreate {
			lastTwoCreates++
		}
	}
	if cs.Ops[lastTwoCreates-2].SpecNodeID != "aaa111" || cs.Ops[lastTwoCreates-1].SpecNodeID != "bbb222" {
		t.Errorf("the two cleanups must be the last two creates, got %+v", cs.Ops)
	}

	wantDeps := []Ref{{Kind: RefOp, OpID: tSec.OpID}, {Kind: RefTask, TaskID: "spexmachina-hun"}}
	if !reflect.DeepEqual(cleanX.Deps, wantDeps) {
		t.Errorf("cleanup X deps: want %+v (T alone — the components are two layers back — plus the retarget's target), got %+v", wantDeps, cleanX.Deps)
	}
	if !reflect.DeepEqual(cleanY.Deps, wantDeps) {
		t.Errorf("cleanup Y deps: want %+v, got %+v", wantDeps, cleanY.Deps)
	}
}

// TestBuild_CleanupLayerWaitsForNearestNonEmptyLayer reruns the fixture
// above with T struck from the batch: each cleanup names the two
// component creates instead — the layer before the cleanups is whichever
// non-empty layer is nearest.
func TestBuild_CleanupLayerWaitsForNearestNonEmptyLayer(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		sampleComponentCreate("compA", "m", "A", nil),
		sampleComponentCreate("compB", "m", "B", nil),
		{Type: ActionRetarget, TaskID: "spexmachina-hun", Module: "m", Node: "R", NodeType: KindComponent, SpecNodeID: "ret-node", SpecHash: "new-hash", Reason: "Spec node modified (retarget): m/R"},
		{Type: ActionCreate, Module: "m", Node: "X", NodeType: KindComponent, SpecNodeID: "aaa111", Reason: "Code cleanup: m/X"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	compA := findOp(t, cs.Ops, "compA")
	compB := findOp(t, cs.Ops, "compB")
	cleanX := findOp(t, cs.Ops, "aaa111")
	wantDeps := []Ref{{Kind: RefOp, OpID: compA.OpID}, {Kind: RefOp, OpID: compB.OpID}, {Kind: RefTask, TaskID: "spexmachina-hun"}}
	if !reflect.DeepEqual(cleanX.Deps, wantDeps) {
		t.Errorf("cleanup X deps: want %+v, got %+v", wantDeps, cleanX.Deps)
	}
}

// TestBuild_CleanupAloneInBatchHasNoDeps reruns with a batch holding one
// cleanup action and nothing else: what landed in an earlier run is
// outside the batch and is not named.
func TestBuild_CleanupAloneInBatchHasNoDeps(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "X", NodeType: KindComponent, SpecNodeID: "aaa111", Reason: "Code cleanup: m/X"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	cleanX := findOp(t, cs.Ops, "aaa111")
	if len(cleanX.Deps) != 0 {
		t.Errorf("cleanup alone in the batch: want empty deps, got %+v", cleanX.Deps)
	}
}

// --- Existing-task dep resolves to ref:task ---

func TestBuild_ExistingTaskDepResolvesToRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y", BeadStatus: "open"}
	actions := []Action{sampleComponentCreate("X", "m", "X", []string{"Y"})}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0] != (Ref{Kind: RefTask, TaskID: "spexmachina-y"}) {
		t.Fatalf("X.deps: want [ref:task spexmachina-y], got %+v", x.Deps)
	}
}

// TestBuild_ExistingTaskDepInProgressResolvesToRefBead repeats the open
// case with the paired task listed as in_progress: a claimed dependency
// is still live work to wait on, so the same ref:task shape holds
// (spec/plan/test_changeset_builder.md, "Live-task dep resolves to
// ref:task").
func TestBuild_ExistingTaskDepInProgressResolvesToRefBead(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y", BeadStatus: "in_progress"}
	actions := []Action{sampleComponentCreate("X", "m", "X", []string{"Y"})}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0] != (Ref{Kind: RefTask, TaskID: "spexmachina-y"}) {
		t.Fatalf("X.deps: want [ref:task spexmachina-y], got %+v", x.Deps)
	}
}

// --- Absent-task dep is dropped ---

func TestBuild_AbsentTaskDepIsDropped(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y"}
	actions := []Action{sampleComponentCreate("X", "m", "X", []string{"Y"})}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 0 {
		t.Errorf("X.deps: want empty (absent dep satisfied), got %+v", x.Deps)
	}
}

// --- Unresolvable dep is a plan error ---

func TestBuild_UnresolvableDepIsError(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{sampleComponentCreate("X", "m", "X", []string{"Z"})}

	cs, err := env.build(actions, "p", "h")
	if err == nil {
		t.Fatal("want error for unresolvable dep Z, got nil")
	}
	if !strings.Contains(err.Error(), "Z") {
		t.Errorf("error must name the unresolvable spec_node_id Z: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset: got %d ops", len(cs.Ops))
	}
}

// --- Retarget dep unresolvable is a plan error (S6) ---

// TestBuild_RetargetUnresolvableDepIsError pins S6's last bullet: "the
// retarget path gets no laxer resolution than the create path." retargetOp
// routes a retarget's freshly recomputed deps through the same ResolveDeps
// call a create uses, so a dep neither in-batch nor tracked by the fold
// must abort the whole build, exactly as TestBuild_UnresolvableDepIsError
// pins for a create.
func TestBuild_RetargetUnresolvableDepIsError(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{
			Type:           ActionRetarget,
			TaskID:         "spexmachina-hun",
			Module:         "m",
			Node:           "X",
			NodeType:       KindComponent,
			SpecNodeID:     "x-node",
			SpecHash:       "new-hash",
			DepSpecNodeIDs: []string{"ghost"},
			Reason:         "Spec node modified (retarget): m/X",
		},
	}
	cs, err := env.build(actions, "p", "h")
	if err == nil {
		t.Fatal("want error for unresolvable retarget dep ghost, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error must name the unresolvable spec_node_id ghost: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset: got %d ops", len(cs.Ops))
	}
}

// --- Retarget op shape ---

func TestBuild_RetargetOpShape(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["w-open"] = Pairing{TaskID: "spexmachina-w", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("y1", "m", "Y", nil), // in-batch predecessor for the retarget's dep
		{
			Type:           ActionRetarget,
			TaskID:         "spexmachina-hun",
			Module:         "m",
			Node:           "X",
			NodeType:       KindComponent,
			SpecNodeID:     "x-node",
			SpecHash:       "new-hash",
			DepSpecNodeIDs: []string{"y1", "w-open"},
			Reason:         "Spec node modified (retarget): m/X",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "x-node")
	if op.Type != OpRetarget {
		t.Errorf("type: want retarget, got %q", op.Type)
	}
	if op.Target == nil || op.Target.Kind != RefTask || op.Target.TaskID != "spexmachina-hun" {
		t.Errorf("target: want ref:task spexmachina-hun, got %+v", op.Target)
	}
	if op.SpecHash != "new-hash" {
		t.Errorf("spec_hash: want new-hash, got %q", op.SpecHash)
	}
	wantLabel := "spex:deadbeef:" + op.OpID
	if len(op.Labels) != 1 || op.Labels[0] != wantLabel {
		t.Errorf("labels: want [%s], got %+v", wantLabel, op.Labels)
	}
	if op.Idempotency != nil {
		t.Errorf("idempotency: want nil on a retarget, got %+v", op.Idempotency)
	}
	if op.Parent != nil || op.Priority != 0 || op.Body != "" {
		t.Errorf("retarget must carry no parent, priority or body, got parent=%+v priority=%d body=%q", op.Parent, op.Priority, op.Body)
	}
	y1 := findOp(t, cs.Ops, "y1")
	if len(op.Deps) != 2 {
		t.Fatalf("deps: want 2, got %+v", op.Deps)
	}
	if op.Deps[0] != (Ref{Kind: RefOp, OpID: y1.OpID}) {
		t.Errorf("deps[0]: want ref:op %s, got %+v", y1.OpID, op.Deps[0])
	}
	if op.Deps[1] != (Ref{Kind: RefTask, TaskID: "spexmachina-w"}) {
		t.Errorf("deps[1]: want ref:task spexmachina-w, got %+v", op.Deps[1])
	}
}

func TestBuild_TwoRetargetsCarryDistinctLabels(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionRetarget, TaskID: "spexmachina-a", NodeType: KindComponent, SpecNodeID: "a-node", SpecHash: "ha", Reason: "r"},
		{Type: ActionRetarget, TaskID: "spexmachina-b", NodeType: KindComponent, SpecNodeID: "b-node", SpecHash: "hb", Reason: "r"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	a := findOp(t, cs.Ops, "a-node")
	b := findOp(t, cs.Ops, "b-node")
	if a.Labels[0] == b.Labels[0] {
		t.Errorf("distinct retargets must not share a label, both got %q", a.Labels[0])
	}
}

func TestBuild_OnlyRetargetsNoCloseNoEpic(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionRetarget, TaskID: "spexmachina-a", NodeType: KindComponent, SpecNodeID: "a-node", SpecHash: "ha", Reason: "r"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 1 {
		t.Fatalf("want exactly 1 op (the retarget), got %d: %+v", len(cs.Ops), cs.Ops)
	}
	if cs.Ops[0].Type != OpRetarget {
		t.Errorf("want the sole op to be a retarget, got %+v", cs.Ops[0])
	}
}

// --- Absorbed array ---

func TestBuild_AbsorbedArrayCarriesEntriesVerbatim(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	b := &Builder{
		SpecGraph:    env.graph,
		Fold:         env.fold,
		Registration: Registration{EID: "h:p"},
		GitHead:      "h",
		Proposal:     "p",
		Absorbed:     []AbsorbedEntry{{Node: "n1", Before: "b1", After: "a1", Reason: "cosmetic rewording"}},
	}
	cs, err := b.Build([]Action{sampleComponentCreate("c1", "m", "C1", nil)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := AbsorbedEntry{Node: "n1", Before: "b1", After: "a1", Reason: "cosmetic rewording"}
	if len(cs.Absorbed) != 1 || cs.Absorbed[0] != want {
		t.Errorf("absorbed: want the entry verbatim %+v, got %+v", want, cs.Absorbed)
	}
	raw, err := json.Marshal(cs.Absorbed[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fieldOrder(t, string(raw), "absorbed entry", "node", "before", "after", "reason")
	for _, op := range cs.Ops {
		if op.SpecNodeID == "n1" {
			t.Fatal("no op should name an absorbed node")
		}
	}
}

func TestBuild_EmptyAbsorbedIsEmptyArrayNotNull(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	cs, err := env.build([]Action{sampleComponentCreate("c1", "m", "C1", nil)}, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"absorbed":[]`) {
		t.Errorf("want absorbed to marshal as an empty array, got: %s", raw)
	}
}

// --- Priority propagation ---

func TestBuild_PriorityPropagation(t *testing.T) {
	modID := schema.IdentityHash("module", "m")
	compID := schema.IdentityHash("m", "component", "X")
	reqA := schema.IdentityHash("m", "requirement", "ReqA")
	reqB := schema.IdentityHash("m", "requirement", "ReqB")
	preqA := schema.IdentityHash("project", "requirement", "PreqA")
	preqB := schema.IdentityHash("project", "requirement", "PreqB")

	p2 := 2
	p1 := 1
	proj := schema.Project{
		Modules: []schema.Module{{ID: modID, Name: "m"}},
		Requirements: []schema.Requirement{
			{ID: preqA, Priority: &p2},
			{ID: preqB, Priority: &p1},
		},
	}
	specs := map[string]schema.ModuleSpec{
		modID: {
			Name: "m",
			Requirements: []schema.ModuleRequirement{
				{ID: reqA, PreqID: preqA},
				{ID: reqB, PreqID: preqB},
			},
			Components: []schema.Component{
				{ID: compID, Name: "X", Implements: []string{reqA, reqB}},
			},
		},
	}

	env := newBuilderEnv()
	env.graph = NewSpecGraph(proj, specs)
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{sampleComponentCreate(compID, "m", "X", nil)}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, compID)
	if x.Priority != 1 {
		t.Errorf("priority: want min(2,1)=1, got %d", x.Priority)
	}
}

func TestBuild_PriorityFallback(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{sampleComponentCreate("X", "m", "X", nil)}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if x.Priority != FallbackPriority {
		t.Errorf("priority: want fallback %d, got %d", FallbackPriority, x.Priority)
	}
}

// TestBuild_PrioritySkipsUnreachableRequirement pins that one broken
// implements entry (a preq_id naming no project requirement) is skipped
// rather than collapsing the whole walk to the fallback: the minimum runs
// over the reachable requirements only, so the two that do resolve (2 and
// 4) still produce priority 2 (test_changeset_builder.md, "Priority
// propagation").
func TestBuild_PrioritySkipsUnreachableRequirement(t *testing.T) {
	modID := schema.IdentityHash("module", "m")
	compID := schema.IdentityHash("m", "component", "X")
	reqBroken := schema.IdentityHash("m", "requirement", "ReqBroken")
	reqB := schema.IdentityHash("m", "requirement", "ReqB")
	reqC := schema.IdentityHash("m", "requirement", "ReqC")
	preqGhost := schema.IdentityHash("project", "requirement", "GhostPreq") // named by reqBroken, absent from proj.Requirements
	preqB := schema.IdentityHash("project", "requirement", "PreqB")
	preqC := schema.IdentityHash("project", "requirement", "PreqC")

	p2 := 2
	p4 := 4
	proj := schema.Project{
		Modules: []schema.Module{{ID: modID, Name: "m"}},
		Requirements: []schema.Requirement{
			{ID: preqB, Priority: &p2},
			{ID: preqC, Priority: &p4},
		},
	}
	specs := map[string]schema.ModuleSpec{
		modID: {
			Name: "m",
			Requirements: []schema.ModuleRequirement{
				{ID: reqBroken, PreqID: preqGhost},
				{ID: reqB, PreqID: preqB},
				{ID: reqC, PreqID: preqC},
			},
			Components: []schema.Component{
				{ID: compID, Name: "X", Implements: []string{reqBroken, reqB, reqC}},
			},
		},
	}

	env := newBuilderEnv()
	env.graph = NewSpecGraph(proj, specs)
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{sampleComponentCreate(compID, "m", "X", nil)}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, compID)
	if x.Priority != 2 {
		t.Errorf("priority: want min(2,4)=2 skipping the unreachable ReqBroken, got %d", x.Priority)
	}
}

// --- A modified node's create carries no lineage ---

// TestBuild_ModifiedNodeCreateCarriesNoLineage pins
// test_changeset_builder.md's "A modified node's create carries no
// lineage": a create for a modified node whose earlier task finished is
// the same op a fresh node gets — no close, no dep naming the
// predecessor, no field carrying its id anywhere in the document.
func TestBuild_ModifiedNodeCreateCarriesNoLineage(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "Q",
			NodeType:   KindComponent,
			SpecNodeID: "Q",
			SpecHash:   "h-Q",
			Reason:     "Spec node modified (new): m/Q",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, op := range cs.Ops {
		if op.Type == OpClose {
			t.Errorf("changeset carries a close op %q: no close accompanies a modified node's create", op.OpID)
		}
	}

	q := findOp(t, cs.Ops, "Q")
	if len(q.Deps) != 0 {
		t.Errorf("Q.deps: want empty — no spec-graph edges and no lineage dep of any kind, got %+v", q.Deps)
	}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "spexmachina-abc") {
		t.Errorf("no field anywhere may carry the predecessor task's id, got %s", raw)
	}

	wantLabel := "spex:deadbeef:" + q.OpID
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (this run's own modified event, no lookup), got %+v", wantLabel, q.Idempotency)
	}
}

// TestBuild_ModifiedNodeCreateCarriesNoLineageWithEmptyFold reruns the
// fixture above against an empty fold: nothing needs seeding to make the
// no-lookup label derivation true.
func TestBuild_ModifiedNodeCreateCarriesNoLineageWithEmptyFold(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "Q",
			NodeType:   KindComponent,
			SpecNodeID: "Q",
			SpecHash:   "h-Q",
			Reason:     "Spec node modified (new): m/Q",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	q := findOp(t, cs.Ops, "Q")
	wantLabel := "spex:deadbeef:" + q.OpID
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (own op_id, no fold lookup), got %+v", wantLabel, q.Idempotency)
	}
}

// TestBuild_OpenPairingRetargetsWithNoLineage reruns the fixture above
// with the pairing's task open: a single retarget op, again with no close
// and no lineage dep — the two paths differ in whether the task moves or
// a successor is born, and neither writes history into the tracker,
// because the journal holds it.
func TestBuild_OpenPairingRetargetsWithNoLineage(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{
			Type:           ActionRetarget,
			TaskID:         "spexmachina-abc",
			Module:         "m",
			Node:           "Q",
			NodeType:       KindComponent,
			SpecNodeID:     "Q",
			SpecHash:       "h-Q2",
			DepSpecNodeIDs: nil,
			Reason:         "Spec node modified (retarget): m/Q",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 1 || cs.Ops[0].Type != OpRetarget {
		t.Fatalf("want a single retarget op, got %+v", cs.Ops)
	}
	if len(cs.Ops[0].Deps) != 0 {
		t.Errorf("retarget path must mint no lineage dep, got %+v", cs.Ops[0].Deps)
	}
}

// --- Cleanup-bead create ---

func TestBuild_CleanupBeadCreate(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			SpecNodeID: "abc123def456",
			Reason:     "Code cleanup: m/X",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "abc123def456")
	if op.SpecNodeKind != KindCleanup {
		t.Errorf("spec_node_kind: want cleanup, got %q", op.SpecNodeKind)
	}
	if op.Title != "Code cleanup: m/X" {
		t.Errorf("title: want Reason verbatim, got %q", op.Title)
	}
	if len(op.Labels) != 0 {
		t.Errorf("labels: want none (retired spex:cleanup discriminator), got %+v", op.Labels)
	}
	if len(op.Deps) != 0 {
		t.Errorf("deps: want empty — no close op accompanies a cleanup, got %+v", op.Deps)
	}
	wantLabel := "spex:deadbeef:" + op.OpID
	if op.Idempotency == nil || op.Idempotency.Label != wantLabel {
		t.Errorf("idempotency.label: want %q (the removal event the cleanup itself mints), got %+v", wantLabel, op.Idempotency)
	}
	if op.Priority != FallbackPriority {
		t.Errorf("priority: want fallback %d, got %d", FallbackPriority, op.Priority)
	}
	for _, o := range cs.Ops {
		if o.Type == OpClose {
			t.Errorf("changeset carries a close op %q: no close accompanies a cleanup", o.OpID)
		}
	}
}

func TestBuild_CleanupBeadCreatePriorBatchRemoval(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.removals["abc123def456"] = RemovalEntry{Removed: true, EID: "E1"}
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			SpecNodeID: "abc123def456",
			Reason:     "Code cleanup: m/X",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef-moved")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "abc123def456")
	if op.Idempotency == nil || op.Idempotency.Label != "spex:E1" {
		t.Errorf("idempotency.label: want spex:E1 (fold's removed event, not derived from any op in this batch), got %+v", op.Idempotency)
	}
	if len(op.Deps) != 0 {
		t.Errorf("deps: want empty — nothing names the finished task, got %+v", op.Deps)
	}
	if op.Priority != FallbackPriority {
		t.Errorf("priority: want fallback %d, got %d", FallbackPriority, op.Priority)
	}
}

// TestBuild_CleanupBeadCreateReAddedSinceRemovalSelfMints varies the fixture
// above: the node's removed event E1 was followed by an added event (the
// node was re-added and is now removed a second time), so the removal E1
// is history, not the node's latest state. The fold answers only when the
// removal is latest; here it is not, so the label falls back to this op's
// own (git_head, op_id) mint, exactly as the never-removed-before case
// does (test_changeset_builder.md, "Cleanup create for a prior-batch
// removal").
func TestBuild_CleanupBeadCreateReAddedSinceRemovalSelfMints(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.removals["abc123def456"] = RemovalEntry{Removed: false}
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			SpecNodeID: "abc123def456",
			Reason:     "Code cleanup: m/X",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef-moved")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "abc123def456")
	wantLabel := "spex:deadbeef-moved:" + op.OpID
	if op.Idempotency == nil || op.Idempotency.Label != wantLabel {
		t.Errorf("idempotency.label: want %q (this op's own mint, not the stale E1 removal), got %+v", wantLabel, op.Idempotency)
	}
}

// --- Title and body ---

func TestBuild_BodyLinksSpecFiles(t *testing.T) {
	modID := schema.IdentityHash("module", "plan")
	compID := schema.IdentityHash("plan", "component", "Foo")
	proj := schema.Project{Modules: []schema.Module{{ID: modID, Name: "plan", Path: "plan"}}}
	specs := map[string]schema.ModuleSpec{
		modID: {Name: "plan", Components: []schema.Component{{ID: compID, Name: "Foo", Content: "arch_foo.md"}}},
	}

	env := newBuilderEnv()
	env.graph = NewSpecGraph(proj, specs)
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	cs, err := env.build([]Action{sampleComponentCreate(compID, "plan", "Foo", nil)}, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, compID)
	if !strings.Contains(op.Body, "spec/plan/arch_foo.md") {
		t.Errorf("body must link the content leaf, got %q", op.Body)
	}
	if !strings.Contains(op.Body, "spec/plan/module.json") {
		t.Errorf("body must link module.json, got %q", op.Body)
	}
	if op.Title != "plan: Foo" {
		t.Errorf("title: want 'plan: Foo', got %q", op.Title)
	}
}

func TestBuild_BodyEmptyWithoutSpecPaths(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	cs, err := env.build([]Action{sampleComponentCreate("nopath", "m", "X", nil)}, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "nopath")
	if op.Body != "" {
		t.Errorf("op nopath: want empty body, got %q", op.Body)
	}
}

func TestBuild_TitleForDataFlowAndTestSection(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "Flow", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h", Reason: "r"},
		{Type: ActionCreate, Module: "m", Node: "Sec", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h", Reason: "r"},
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	flow := findOp(t, cs.Ops, "f1")
	if flow.Title != "m: data_flow Flow" {
		t.Errorf("data_flow title: got %q", flow.Title)
	}
	sec := findOp(t, cs.Ops, "t1")
	if sec.Title != "m: test Sec" {
		t.Errorf("test_section title: got %q", sec.Title)
	}
}

// --- Cross-component: byte-identical across runs ---

func crossComponentEnv() (*builderEnv, []Action) {
	env := newBuilderEnv()
	env.fold.fakeFold["z-open"] = Pairing{TaskID: "spexmachina-z", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("x1", "m", "X", []string{"y1", "z-open"}),
		sampleComponentCreate("y1", "m", "Y", nil),
		sampleComponentCreate("ab1", "m", "AB", nil),
		sampleComponentCreate("aa1", "m", "AA", nil),
		{
			Type:           ActionRetarget,
			TaskID:         "spexmachina-r",
			Module:         "m",
			Node:           "R",
			NodeType:       KindComponent,
			SpecNodeID:     "r1",
			SpecHash:       "h-r1",
			DepSpecNodeIDs: []string{"y1"},
			Reason:         "Spec node modified (retarget): m/R",
		},
	}
	return env, actions
}

func TestBuild_CrossComponent_ByteIdenticalAcrossRuns(t *testing.T) {
	env1, actions1 := crossComponentEnv()
	cs1, err := env1.build(actions1, "prop", "deadbeefcafe")
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	env2, actions2 := crossComponentEnv()
	cs2, err := env2.build(actions2, "prop", "deadbeefcafe")
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	json1, _ := json.MarshalIndent(cs1, "", "  ")
	json2, _ := json.MarshalIndent(cs2, "", "  ")
	if !bytes.Equal(json1, json2) {
		t.Fatalf("changesets not byte-identical:\n%s\n---\n%s", json1, json2)
	}

	if cs1.Ops[0].SpecNodeKind != KindProposalEpic {
		t.Fatalf("op[0]: want proposal_epic first, got %+v", cs1.Ops[0])
	}
	iAA, iAB, iY, iX := opIndex(cs1.Ops, "aa1"), opIndex(cs1.Ops, "ab1"), opIndex(cs1.Ops, "y1"), opIndex(cs1.Ops, "x1")
	if !(iAA < iAB && iAB < iY && iY < iX) {
		t.Errorf("sort order: want aa1 < ab1 < y1 < x1, got indices %d %d %d %d", iAA, iAB, iY, iX)
	}

	for _, op := range cs1.Ops {
		if op.Type == OpRetarget {
			want := "spex:deadbeefcafe:" + op.OpID
			if len(op.Labels) != 1 || op.Labels[0] != want {
				t.Errorf("retarget label: want %s, got %+v", want, op.Labels)
			}
			continue
		}
		if op.SpecNodeKind == KindProposalEpic {
			if op.Idempotency == nil || op.Idempotency.Label != "spex:deadbeefcafe:prop" {
				t.Errorf("epic label: want spex:deadbeefcafe:prop, got %+v", op.Idempotency)
			}
			continue
		}
		want := "spex:deadbeefcafe:" + op.OpID
		if op.Idempotency == nil || op.Idempotency.Label != want {
			t.Errorf("op %s label: want %s, got %+v", op.OpID, want, op.Idempotency)
		}
	}

	x := findOp(t, cs1.Ops, "x1")
	if len(x.Deps) != 2 {
		t.Fatalf("x1 deps: want 2, got %+v", x.Deps)
	}
	y := findOp(t, cs1.Ops, "y1")
	if x.Deps[0] != (Ref{Kind: RefOp, OpID: y.OpID}) {
		t.Errorf("x1 dep[0]: want ref:op %s, got %+v", y.OpID, x.Deps[0])
	}
	if x.Deps[1] != (Ref{Kind: RefTask, TaskID: "spexmachina-z"}) {
		t.Errorf("x1 dep[1]: want ref:task spexmachina-z, got %+v", x.Deps[1])
	}
}

// --- Cross-component: dep classification round-trip ---

func TestBuild_CrossComponent_DepClassificationRoundTrip(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["q-open"] = Pairing{TaskID: "spexmachina-q", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("d1", "m", "D", []string{"p1", "q-open"}),
		sampleComponentCreate("p1", "m", "P", nil),
	}
	cs, err := env.build(actions, "p", "deadbeefcafe")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	d := findOp(t, cs.Ops, "d1")
	p := findOp(t, cs.Ops, "p1")
	if len(d.Deps) != 2 {
		t.Fatalf("d1 deps: want exactly 2 shapes, got %+v", d.Deps)
	}
	if d.Deps[0] != (Ref{Kind: RefOp, OpID: p.OpID}) {
		t.Errorf("dep[0]: want pure ref:op %s, got %+v", p.OpID, d.Deps[0])
	}
	if d.Deps[1] != (Ref{Kind: RefTask, TaskID: "spexmachina-q"}) {
		t.Errorf("dep[1]: want pure ref:task spexmachina-q, got %+v", d.Deps[1])
	}
	if opIndex(cs.Ops, "p1") >= opIndex(cs.Ops, "d1") {
		t.Errorf("sorter must sequence in-batch predecessor p1 before dependent d1: %+v", cs.Ops)
	}
}

func TestBuild_CrossComponent_UnresolvableDepAbortsWithNoPartialChangeset(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["q-open"] = Pairing{TaskID: "spexmachina-q", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("d1", "m", "D", []string{"p1", "q-open", "r-none"}),
		sampleComponentCreate("p1", "m", "P", nil),
	}
	cs, err := env.build(actions, "p", "deadbeefcafe")
	if err == nil {
		t.Fatal("want error for unresolvable dep r-none, got nil")
	}
	if !strings.Contains(err.Error(), "r-none") {
		t.Errorf("error must name the unresolvable spec_node_id r-none: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset: got %d ops", len(cs.Ops))
	}
}

// --- Cross-component: cycle detection ---

func TestBuild_CrossComponent_CycleErrorNamesBothNodes(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		sampleComponentCreate("cycA", "m", "A", []string{"cycB"}),
		sampleComponentCreate("cycB", "m", "B", []string{"cycA"}),
	}
	cs, err := env.build(actions, "p", "deadbeefcafe")
	if err == nil {
		t.Fatal("want cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycA") || !strings.Contains(err.Error(), "cycB") {
		t.Errorf("error must name both spec_node_ids in the cycle: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset on cycle: got %d ops", len(cs.Ops))
	}
}

// --- Cross-component: spec_node_kind per declared type comes from the profile ---

// TestBuild_SpecNodeKindMatchesPlanRelevantTypesUnderDefaultProfile pins the
// default-profile half of "spec_node_kind per declared type comes from the
// profile": the default profile's plan-relevant set is exactly component,
// data_flow, test_section (schema.DefaultProfile), and a create's
// spec_node_kind is Action.NodeType copied verbatim, so the emitted
// vocabulary matches today's arch_changeset_builder.md table exactly —
// byte-identical to the pre-profile builder's over the same batch, since
// nothing about this composition depends on the profile at all.
func TestBuild_SpecNodeKindMatchesPlanRelevantTypesUnderDefaultProfile(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		sampleComponentCreate("c1", "m", "C1", nil),
		{Type: ActionCreate, Module: "m", Node: "Flow", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h-f1", Reason: "New spec node: m/Flow"},
		{Type: ActionCreate, Module: "m", Node: "Sec", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h-t1", Reason: "New spec node: m/Sec"},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []struct{ specID, kind string }{
		{"c1", KindComponent},
		{"f1", KindDataFlow},
		{"t1", KindTestSection},
	} {
		op := findOp(t, cs.Ops, want.specID)
		if op.SpecNodeKind != want.kind {
			t.Errorf("%s: spec_node_kind want %q, got %q", want.specID, want.kind, op.SpecNodeKind)
		}
		assertCanonicalFieldOrder(t, op)
	}
}

// TestBuild_ProfileDeclaredKindOutsideDefaultVocabularyIsPlaced pins the
// resolution of drift-spexmachina-h4gv.21.json: TopologicalSorter
// (spexmachina-swvx.38) now reads the resolved profile's plan-relevant list
// instead of a compiled-in tier table, so a profile-declared kind outside
// today's three-entry default vocabulary (e.g. "endpoint") places into its
// own layer instead of refusing the whole build; ChangesetBuilder copies
// spec_node_kind from Action.NodeType verbatim, same as any other kind.
func TestBuild_ProfileDeclaredKindOutsideDefaultVocabularyIsPlaced(t *testing.T) {
	env := newBuilderEnv()
	env.graph = env.graph.WithProfile(&schema.Profile{PlanRelevant: []string{"component", "data_flow", "test_section", "endpoint"}})
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "T", NodeType: KindTestSection, SpecNodeID: "t1", SpecHash: "h-t1", Reason: "New spec node: m/T"},
		{Type: ActionCreate, Module: "m", Node: "EP", NodeType: "endpoint", SpecNodeID: "e1", SpecHash: "h-e1", Reason: "New spec node: m/EP"},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	op := findOp(t, cs.Ops, "e1")
	if op.SpecNodeKind != "endpoint" {
		t.Errorf("spec_node_kind: got %q want %q", op.SpecNodeKind, "endpoint")
	}
	if opIndex(cs.Ops, "e1") <= opIndex(cs.Ops, "t1") {
		t.Errorf("endpoint must be emitted last among the non-cleanup creates, got %+v", cs.Ops)
	}
	tSec := findOp(t, cs.Ops, "t1")
	wantDeps := []Ref{{Kind: RefOp, OpID: tSec.OpID}}
	if !reflect.DeepEqual(op.Deps, wantDeps) {
		t.Errorf("endpoint deps: want the whole test_section layer %+v via the layer edge, got %+v", wantDeps, op.Deps)
	}
}

// TestBuild_KindStruckFromPlanRelevantListStillErrors pins the other half:
// a kind the resolved profile does not place is still refused, naming the
// kind, with no partial changeset — the profile places kinds, and an
// unplaced one is an error, not a silent drop.
func TestBuild_KindStruckFromPlanRelevantListStillErrors(t *testing.T) {
	env := newBuilderEnv()
	env.graph = env.graph.WithProfile(&schema.Profile{PlanRelevant: []string{"component", "data_flow", "test_section"}})
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionCreate, Module: "m", Node: "EP", NodeType: "endpoint", SpecNodeID: "e1", SpecHash: "h-e1", Reason: "New spec node: m/EP"},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err == nil {
		t.Fatalf("Build: want error for unplaced kind \"endpoint\", got %d ops", len(cs.Ops))
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error must name the unplaced kind: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("no partial changeset: got %d ops", len(cs.Ops))
	}
}

// --- Edge cases ---

func TestBuild_EmptyBatchWithExistingEpicYieldsNoOps(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	cs, err := env.build(nil, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 0 {
		t.Errorf("empty batch + existing epic: want 0 ops, got %d", len(cs.Ops))
	}
	if cs.Ops == nil {
		t.Error("ops must marshal as an empty array, not null")
	}
}

func TestBuild_EmptyBatchNewProposalSynthesizesEpicOnly(t *testing.T) {
	env := newBuilderEnv()
	cs, err := env.build(nil, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 1 || cs.Ops[0].SpecNodeKind != KindProposalEpic {
		t.Fatalf("want 1 op (the epic), got %+v", cs.Ops)
	}
}

func TestBuild_OnlyClosesNoCreates(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionObsolete, TaskID: "spexmachina-1", Module: "m", Node: "A", NodeType: KindComponent, Reason: "removed"},
		{Type: ActionObsolete, TaskID: "spexmachina-2", Module: "m", Node: "B", NodeType: KindComponent, Reason: "removed"},
	}
	cs, err := env.build(actions, "p", "head1")
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

func TestBuild_InBatchDepWinsOverAbsentFoldEntry(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y-old"}
	actions := []Action{
		sampleComponentCreate("Y", "m", "Y", nil),
		sampleComponentCreate("X", "m", "X", []string{"Y"}),
	}
	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	y := findOp(t, cs.Ops, "Y")
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 1 || x.Deps[0] != (Ref{Kind: RefOp, OpID: y.OpID}) {
		t.Errorf("X.deps: want [ref:op %s] (in-batch wins over absent fold entry), got %+v", y.OpID, x.Deps)
	}
}

// --- Op ids are canonical keys across a batch mixing every kind ---

// TestBuild_OpIDsCanonicalKeysAcrossMixedBatch mixes every op kind in one
// batch — a component create, a data_flow create, a cleanup create for a
// removed node, a retarget, and two closes (a removal close and a
// fold-back close) — and asserts every op_id is op-<kind>-<key>: never a
// position, so a node has at most one op per batch and a task at most one
// close, and no two ids collide (test_changeset_builder.md, "Op ids are
// canonical keys across a batch mixing every kind").
func TestBuild_OpIDsCanonicalKeysAcrossMixedBatch(t *testing.T) {
	baseActions := func() []Action {
		return []Action{
			sampleComponentCreate("c1", "m", "C1", nil),
			{Type: ActionCreate, Module: "m", Node: "Flow", NodeType: KindDataFlow, SpecNodeID: "f1", SpecHash: "h-f1", Reason: "New spec node: m/Flow"},
			{Type: ActionCreate, Module: "m", Node: "X", NodeType: KindComponent, SpecNodeID: "cleanup1", Reason: "Code cleanup: m/X"},
			{Type: ActionRetarget, TaskID: "spexmachina-hun", Module: "m", Node: "R", NodeType: KindComponent, SpecNodeID: "r-node", SpecHash: "new-hash", Reason: "Spec node modified (retarget): m/R"},
			{Type: ActionObsolete, TaskID: "spexmachina-old", Module: "m", Node: "W", NodeType: KindComponent, ChangeType: "removed", Reason: "Spec node removed: m/W"},
			{Type: ActionObsolete, TaskID: "spexmachina-ts", Module: "m", Node: "TS", NodeType: KindTestSection, ChangeType: "modified", Reason: "Spec node modified: m/TS"},
		}
	}

	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	cs, err := env.build(baseActions(), "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 6 {
		t.Fatalf("want 6 ops, got %d: %+v", len(cs.Ops), cs.Ops)
	}

	wantIDs := map[string]string{
		"c1":       "op-component-c1",
		"f1":       "op-data_flow-f1",
		"cleanup1": "op-cleanup-cleanup1",
		"r-node":   "op-retarget-r-node",
	}
	for specID, want := range wantIDs {
		op := findOp(t, cs.Ops, specID)
		if op.OpID != want {
			t.Errorf("%s: op_id want %q, got %q", specID, want, op.OpID)
		}
	}
	for _, taskID := range []string{"spexmachina-old", "spexmachina-ts"} {
		want := "op-close-" + taskID
		var found bool
		for _, op := range cs.Ops {
			if op.Type == OpClose && op.Target != nil && op.Target.TaskID == taskID {
				found = true
				if op.OpID != want {
					t.Errorf("close on %s: op_id want %q, got %q", taskID, want, op.OpID)
				}
			}
		}
		if !found {
			t.Fatalf("no close op targeting %q", taskID)
		}
	}

	seen := map[string]bool{}
	for _, op := range cs.Ops {
		if seen[op.OpID] {
			t.Errorf("op id %s reused", op.OpID)
		}
		seen[op.OpID] = true
	}

	var creates, retargetIdx, closeStart int
	for i, op := range cs.Ops {
		switch op.Type {
		case OpCreate:
			creates++
		case OpRetarget:
			retargetIdx = i
		case OpClose:
			if closeStart == 0 {
				closeStart = i
			}
		}
	}
	if creates != 3 {
		t.Errorf("want 3 creates, got %d", creates)
	}
	if retargetIdx != creates {
		t.Errorf("retarget must immediately follow the creates block: retarget at %d, creates=%d", retargetIdx, creates)
	}
	if closeStart != creates+1 {
		t.Errorf("closes must follow the retarget: closes start at %d, want %d", closeStart, creates+1)
	}

	cleanup := findOp(t, cs.Ops, "cleanup1")
	wantLabel := "spex:deadbeef:" + cleanup.OpID
	if cleanup.Idempotency == nil || cleanup.Idempotency.Label != wantLabel {
		t.Errorf("cleanup idempotency.label: want %s (the cleanup op's own op_id — no close accompanies a cleanup), got %+v", wantLabel, cleanup.Idempotency)
	}
	for _, op := range cs.Ops {
		if op.Type != OpClose {
			continue
		}
		if strings.Contains(cleanup.Idempotency.Label, op.OpID) {
			t.Errorf("cleanup label %q embeds close op %s's id: every label is derived from the op that carries it or read from the fold, never predicted from another op", cleanup.Idempotency.Label, op.OpID)
		}
	}

	// A second component create added to the batch must not rename any op
	// already present — every id is derived from its own action alone.
	env2 := newBuilderEnv()
	env2.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	moreActions := append(baseActions(), sampleComponentCreate("c2", "m", "C2", nil))
	cs2, err := env2.build(moreActions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build (extra create): %v", err)
	}
	for specID, want := range wantIDs {
		op := findOp(t, cs2.Ops, specID)
		if op.OpID != want {
			t.Errorf("after adding c2, %s kept a different op_id: want %q, got %q", specID, want, op.OpID)
		}
	}
}
