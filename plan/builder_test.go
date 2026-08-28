package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func findClose(t *testing.T, ops []Op, beadID string) Op {
	t.Helper()
	for _, op := range ops {
		if op.Type == OpClose && op.Target != nil && op.Target.BeadID == beadID {
			return op
		}
	}
	t.Fatalf("no close op targeting %q in %+v", beadID, ops)
	return Op{}
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
	if cs.Version != 3 {
		t.Errorf("version: want 3, got %d", cs.Version)
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
			BeadID:         "spexmachina-hun",
			Module:         "m",
			Node:           "X",
			NodeType:       KindComponent,
			SpecNodeID:     "x-node",
			SpecHash:       "new-hash",
			DepSpecNodeIDs: []string{"w-open"},
			Reason:         "Spec node modified (retarget): m/X",
		},
		{Type: ActionObsolete, BeadID: "spexmachina-old", Module: "m", Node: "B", NodeType: KindComponent, Reason: "removed"},
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
// every in-batch dep to nothing rather than failing loudly
// (test_changeset_builder.md, "Canonical schema and field order").
func TestBuild_RefWireKeyNames(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["y-open"] = Pairing{TaskID: "spexmachina-y", BeadStatus: "open"}
	actions := []Action{
		sampleComponentCreate("a1", "m", "A", nil),
		sampleComponentCreate("b1", "m", "B", []string{"a1", "y-open"}),
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "Q",
			NodeType:   KindComponent,
			SpecNodeID: "q1",
			SpecHash:   "h-q1",
			OldBeadID:  "spexmachina-abc",
			Reason:     "Spec node modified (new): m/Q",
		},
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
	if !strings.Contains(got, `{"ref":"bead","bead_id":"spexmachina-y"}`) {
		t.Errorf("existing-task dep: want literal {\"ref\":\"bead\",\"bead_id\":...} on the wire, got %s", got)
	}

	q := findOp(t, cs.Ops, "q1")
	raw2, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got2 := string(raw2)
	if !strings.Contains(got2, `{"ref":"bead","bead_id":"spexmachina-abc","type":"blocks"}`) {
		t.Errorf("lineage dep: want type naming the edge on the wire, got %s", got2)
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
	if c1.Parent == nil || c1.Parent.Kind != RefBead || c1.Parent.BeadID != "spexmachina-epic" {
		t.Errorf("c1 parent: want ref:bead spexmachina-epic, got %+v", c1.Parent)
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
	if c1.Parent == nil || c1.Parent.Kind != RefBead || c1.Parent.BeadID != "spexmachina-legacy-epic" {
		t.Errorf("c1 parent: want ref:bead spexmachina-legacy-epic, got %+v", c1.Parent)
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

// --- Existing-task dep resolves to ref:bead ---

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
	if len(x.Deps) != 1 || x.Deps[0] != (Ref{Kind: RefBead, BeadID: "spexmachina-y"}) {
		t.Fatalf("X.deps: want [ref:bead spexmachina-y], got %+v", x.Deps)
	}
}

// --- Closed-task dep is dropped ---

func TestBuild_ClosedTaskDepIsDropped(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y", BeadStatus: "closed"}
	actions := []Action{sampleComponentCreate("X", "m", "X", []string{"Y"})}

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	x := findOp(t, cs.Ops, "X")
	if len(x.Deps) != 0 {
		t.Errorf("X.deps: want empty (closed dep satisfied), got %+v", x.Deps)
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
			BeadID:         "spexmachina-hun",
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
			BeadID:         "spexmachina-hun",
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
	if op.Target == nil || op.Target.Kind != RefBead || op.Target.BeadID != "spexmachina-hun" {
		t.Errorf("target: want ref:bead spexmachina-hun, got %+v", op.Target)
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
	if op.Deps[1] != (Ref{Kind: RefBead, BeadID: "spexmachina-w"}) {
		t.Errorf("deps[1]: want ref:bead spexmachina-w, got %+v", op.Deps[1])
	}
}

func TestBuild_TwoRetargetsCarryDistinctLabels(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{Type: ActionRetarget, BeadID: "spexmachina-a", NodeType: KindComponent, SpecNodeID: "a-node", SpecHash: "ha", Reason: "r"},
		{Type: ActionRetarget, BeadID: "spexmachina-b", NodeType: KindComponent, SpecNodeID: "b-node", SpecHash: "hb", Reason: "r"},
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
		{Type: ActionRetarget, BeadID: "spexmachina-a", NodeType: KindComponent, SpecNodeID: "a-node", SpecHash: "ha", Reason: "r"},
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
	if len(cs.Absorbed) != 1 || cs.Absorbed[0].Node != "n1" || cs.Absorbed[0].Reason != "cosmetic rewording" {
		t.Errorf("absorbed: want the entry verbatim, got %+v", cs.Absorbed)
	}
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

// --- Obsolete + create lineage ---

func TestBuild_ObsoleteAndCreateLineage(t *testing.T) {
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
			OldBeadID:  "spexmachina-abc",
			Reason:     "Spec node modified (new): m/Q",
		},
		{
			Type:       ActionObsolete,
			BeadID:     "spexmachina-abc",
			Module:     "m",
			Node:       "Q",
			NodeType:   KindComponent,
			SpecNodeID: "Q",
			ChangeType: "modified",
			Reason:     "Spec node modified: m/Q",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	q := findOp(t, cs.Ops, "Q")
	var foundLineage bool
	for _, d := range q.Deps {
		if d == (Ref{Kind: RefBead, BeadID: "spexmachina-abc", EdgeType: "blocks"}) {
			foundLineage = true
		}
	}
	if !foundLineage {
		t.Errorf("Q.deps: want ref:bead spexmachina-abc type:blocks, got %+v", q.Deps)
	}

	closeOp := findClose(t, cs.Ops, "spexmachina-abc")
	if len(closeOp.Labels) != 0 {
		t.Errorf("close labels: want none (retired spex:obsolete/commit:<HEAD> markers), got %v", closeOp.Labels)
	}
	if closeOp.Reason != "Spec node modified: m/Q" {
		t.Errorf("close reason: got %q", closeOp.Reason)
	}

	wantLabel := "spex:deadbeef:" + q.OpID
	if q.Idempotency == nil || q.Idempotency.Label != wantLabel {
		t.Errorf("Q.idempotency.label: want %q (own op_id), got %+v", wantLabel, q.Idempotency)
	}
}

func TestBuild_ObsoleteAndCreateLineageLabelHoldsWithEmptyFold(t *testing.T) {
	env := newBuilderEnv()
	actions := []Action{
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "Q",
			NodeType:   KindComponent,
			SpecNodeID: "Q",
			SpecHash:   "h-Q",
			OldBeadID:  "spexmachina-abc",
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

func TestBuild_OpenPairingRetargetsWithNoLineage(t *testing.T) {
	// Rerun of TestBuild_ObsoleteAndCreateLineage's fixture with the
	// pairing open: a single retarget op, no blocks dep anywhere.
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		{
			Type:           ActionRetarget,
			BeadID:         "spexmachina-abc",
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
	for _, op := range cs.Ops {
		for _, d := range op.Deps {
			if d.EdgeType == "blocks" {
				t.Errorf("retarget path must mint no blocks lineage dep, got %+v", op.Deps)
			}
		}
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
			OldBeadID:  "spexmachina-old",
			Reason:     "Code cleanup: m/X",
		},
		{
			Type:       ActionObsolete,
			BeadID:     "spexmachina-old",
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			ChangeType: "removed",
			Reason:     "Spec node removed: m/X",
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
	closeOp := findClose(t, cs.Ops, "spexmachina-old")
	wantLabel := "spex:deadbeef:" + closeOp.OpID
	if op.Idempotency == nil || op.Idempotency.Label != wantLabel {
		t.Errorf("idempotency.label: want %q (same-batch close op's eid), got %+v", wantLabel, op.Idempotency)
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
			OldBeadID:  "spexmachina-old",
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
	var foundLineage bool
	for _, d := range op.Deps {
		if d == (Ref{Kind: RefBead, BeadID: "spexmachina-old", EdgeType: "blocks"}) {
			foundLineage = true
		}
	}
	if !foundLineage {
		t.Errorf("deps: want ref:bead spexmachina-old type:blocks, got %+v", op.Deps)
	}
	if op.Priority != FallbackPriority {
		t.Errorf("priority: want fallback %d, got %d", FallbackPriority, op.Priority)
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
			BeadID:         "spexmachina-r",
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
	if x.Deps[1] != (Ref{Kind: RefBead, BeadID: "spexmachina-z"}) {
		t.Errorf("x1 dep[1]: want ref:bead spexmachina-z, got %+v", x.Deps[1])
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
	if d.Deps[1] != (Ref{Kind: RefBead, BeadID: "spexmachina-q"}) {
		t.Errorf("dep[1]: want pure ref:bead spexmachina-q, got %+v", d.Deps[1])
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

// TestBuild_CrossComponent_SpecNodeKindPerProfileDeclaredType covers the
// default-profile half of test_changeset_builder.md's "spec_node_kind per
// declared type comes from the profile" scenario: ChangesetBuilder copies
// Action.NodeType into spec_node_kind verbatim and validates nothing
// against the vocabulary table (arch_changeset_builder.md, "spec_node_kind
// vocabulary": "the closure is upstream, not here"), so under the default
// profile every create carries exactly the kind ActionClassifier assigned
// it.
//
// The scenario's extended-profile half — a create whose NodeType names a
// profile-declared type outside today's four-entry vocabulary (e.g.
// "endpoint") — cannot be exercised through Build(): TopologicalSorter's
// tier table is a fixed four-entry map with no profile awareness, and its
// own arch (arch_topological_sorter.md, arch_resolver.md) documents that it
// refuses to tier a create whose kind it does not recognise — confirmed by
// probing Build() with such an action, which errors "belongs to no tier"
// rather than composing an op. See drifts/drift-spexmachina-h4gv.21.json.
func TestBuild_CrossComponent_SpecNodeKindPerProfileDeclaredType(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		sampleComponentCreate("c1", "m", "C", nil),
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "F",
			NodeType:   KindDataFlow,
			SpecNodeID: "f1",
			SpecHash:   "h-f1",
			Reason:     "New spec node: m/F",
		},
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "T",
			NodeType:   KindTestSection,
			SpecNodeID: "t1",
			SpecHash:   "h-t1",
			Reason:     "New spec node: m/T",
		},
	}
	cs, err := env.build(actions, "p", "deadbeefcafe")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	c := findOp(t, cs.Ops, "c1")
	if c.SpecNodeKind != KindComponent {
		t.Errorf("c1 spec_node_kind: want %q, got %q", KindComponent, c.SpecNodeKind)
	}
	f := findOp(t, cs.Ops, "f1")
	if f.SpecNodeKind != KindDataFlow {
		t.Errorf("f1 spec_node_kind: want %q, got %q", KindDataFlow, f.SpecNodeKind)
	}
	ts := findOp(t, cs.Ops, "t1")
	if ts.SpecNodeKind != KindTestSection {
		t.Errorf("t1 spec_node_kind: want %q, got %q", KindTestSection, ts.SpecNodeKind)
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
		{Type: ActionObsolete, BeadID: "spexmachina-1", Module: "m", Node: "A", NodeType: KindComponent, Reason: "removed"},
		{Type: ActionObsolete, BeadID: "spexmachina-2", Module: "m", Node: "B", NodeType: KindComponent, Reason: "removed"},
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

func TestBuild_InBatchDepWinsOverClosedFoldEntry(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	env.fold.fakeFold["Y"] = Pairing{TaskID: "spexmachina-y-old", BeadStatus: "closed"}
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
		t.Errorf("X.deps: want [ref:op %s] (in-batch wins over closed fold entry), got %+v", y.OpID, x.Deps)
	}
}

func TestBuild_OpIDsUnpaddedUnderTen(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := make([]Action, 0, 8)
	for i := 0; i < 8; i++ {
		id := string(rune('A' + i))
		actions = append(actions, sampleComponentCreate(id, "m", id, nil))
	}
	actions = append(actions, Action{Type: ActionObsolete, BeadID: "spexmachina-old", Module: "m", Node: "Z", NodeType: KindComponent, Reason: "removed"})

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 9 {
		t.Fatalf("want 9 ops, got %d", len(cs.Ops))
	}
	if cs.Ops[0].OpID != "op-1" || cs.Ops[8].OpID != "op-9" {
		t.Errorf("want unpadded ids op-1..op-9, got %s .. %s", cs.Ops[0].OpID, cs.Ops[8].OpID)
	}
}

func TestBuild_OpIDsPaddedAtTenthOp(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := make([]Action, 0, 9)
	for i := 0; i < 9; i++ {
		id := string(rune('A' + i))
		actions = append(actions, sampleComponentCreate(id, "m", id, nil))
	}
	actions = append(actions, Action{Type: ActionObsolete, BeadID: "spexmachina-old", Module: "m", Node: "Z", NodeType: KindComponent, Reason: "removed"})

	cs, err := env.build(actions, "p", "h")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 10 {
		t.Fatalf("want 10 ops, got %d", len(cs.Ops))
	}
	if cs.Ops[0].OpID != "op-01" || cs.Ops[9].OpID != "op-10" {
		t.Errorf("10 total ops: want zero-padded ids op-01..op-10, got %s .. %s", cs.Ops[0].OpID, cs.Ops[9].OpID)
	}
}

// --- Op id numbering across a batch mixing every kind ---

// TestBuild_OpIDNumberingAcrossMixedBatch mixes every op kind in one batch —
// five conventional creates, one cleanup create for a removed node, one
// retarget, and three closes, one of them the removal close the cleanup
// answers — and asserts op ids run from op-01 in creates -> retargets ->
// closes order with no gap and no reuse. The batch is sized to ten ops
// total, and only reaches ten when the retarget is counted alongside the
// creates and closes: six creates plus three closes alone is nine, which a
// pad rule blind to retargets (test_changeset_builder.md, "Op id numbering
// across a batch mixing every kind") would leave unpadded at op-1..op-9;
// counting the retarget crosses the batch to ten and forces op-01..op-10.
// It also pins that the cleanup's idempotency.label reads the removal
// close's actual op_id out of the emitted document: the label is derived
// before the close ops are numbered, so it rests on a prediction of where
// the closes will start, and the retarget block sitting between the
// creates and the closes is exactly what that prediction has to account
// for.
func TestBuild_OpIDNumberingAcrossMixedBatch(t *testing.T) {
	env := newBuilderEnv()
	env.fold.fakeFold["p"] = Pairing{TaskID: "spexmachina-epic"}
	actions := []Action{
		sampleComponentCreate("c1", "m", "C1", nil),
		sampleComponentCreate("c2", "m", "C2", nil),
		sampleComponentCreate("c3", "m", "C3", nil),
		sampleComponentCreate("c4", "m", "C4", nil),
		sampleComponentCreate("c5", "m", "C5", nil),
		{
			Type:       ActionCreate,
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			SpecNodeID: "cleanup-node",
			OldBeadID:  "spexmachina-old",
			Reason:     "Code cleanup: m/X",
		},
		{
			Type:       ActionRetarget,
			BeadID:     "spexmachina-hun",
			Module:     "m",
			Node:       "R",
			NodeType:   KindComponent,
			SpecNodeID: "r-node",
			SpecHash:   "new-hash",
			Reason:     "Spec node modified (retarget): m/R",
		},
		{
			Type:       ActionObsolete,
			BeadID:     "spexmachina-old",
			Module:     "m",
			Node:       "X",
			NodeType:   KindComponent,
			ChangeType: "removed",
			Reason:     "Spec node removed: m/X",
		},
		{
			Type:       ActionObsolete,
			BeadID:     "spexmachina-other",
			Module:     "m",
			Node:       "Y",
			NodeType:   KindComponent,
			ChangeType: "removed",
			Reason:     "Spec node removed: m/Y",
		},
		{
			Type:       ActionObsolete,
			BeadID:     "spexmachina-third",
			Module:     "m",
			Node:       "Z",
			NodeType:   KindComponent,
			ChangeType: "removed",
			Reason:     "Spec node removed: m/Z",
		},
	}
	cs, err := env.build(actions, "p", "deadbeef")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cs.Ops) != 10 {
		t.Fatalf("want 10 ops, got %d: %+v", len(cs.Ops), cs.Ops)
	}

	seen := make(map[string]bool, len(cs.Ops))
	for i, op := range cs.Ops {
		want := fmt.Sprintf("op-%02d", i+1)
		if op.OpID != want {
			t.Errorf("op[%d]: want id %s, got %s", i, want, op.OpID)
		}
		if seen[op.OpID] {
			t.Errorf("op id %s reused", op.OpID)
		}
		seen[op.OpID] = true
	}

	for i := 0; i < 6; i++ {
		if cs.Ops[i].Type != OpCreate {
			t.Errorf("op[%d]: want create (creates come first), got %s", i, cs.Ops[i].Type)
		}
	}
	if cs.Ops[6].Type != OpRetarget {
		t.Errorf("op[6]: want retarget (after creates, before closes), got %s", cs.Ops[6].Type)
	}
	for i := 7; i < 10; i++ {
		if cs.Ops[i].Type != OpClose {
			t.Errorf("op[%d]: want close (closes come last), got %s", i, cs.Ops[i].Type)
		}
	}

	cleanup := findOp(t, cs.Ops, "cleanup-node")
	removalClose := findClose(t, cs.Ops, "spexmachina-old")
	wantLabel := "spex:deadbeef:" + removalClose.OpID
	if cleanup.Idempotency == nil || cleanup.Idempotency.Label != wantLabel {
		t.Errorf("cleanup idempotency.label: want %s (the removal close's actual op_id), got %+v", wantLabel, cleanup.Idempotency)
	}
}
