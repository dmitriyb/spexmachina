package plan

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// fakeFold is a map-backed FoldLookup stand-in for tests
// (spec/plan/arch_resolver.md, "Interface": "tests substitute a stand-in").
type fakeFold map[string]Pairing

func (f fakeFold) Lookup(key string) (Pairing, bool) {
	p, ok := f[key]
	return p, ok
}

func intp(n int) *int { return &n }

// --- ResolveDeps: the two ref shapes + drop + error ---

func TestResolveDeps_InBatchWinsOverFold(t *testing.T) {
	batch := map[string]string{"dep1": "op-2"}
	fold := fakeFold{"dep1": {TaskID: "spexmachina-old", BeadStatus: "open"}}

	refs, err := ResolveDeps([]string{"dep1"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Ref{{Kind: RefOp, OpID: "op-2"}}
	if len(refs) != 1 || refs[0] != want[0] {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
}

func TestResolveDeps_FoldOpenYieldsRefBead(t *testing.T) {
	batch := map[string]string{}
	fold := fakeFold{"dep1": {TaskID: "spexmachina-abc", BeadStatus: "open"}}

	refs, err := ResolveDeps([]string{"dep1"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Ref{Kind: RefTask, TaskID: "spexmachina-abc"}
	if len(refs) != 1 || refs[0] != want {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
}

func TestResolveDeps_FoldInProgressYieldsRefBead(t *testing.T) {
	batch := map[string]string{}
	fold := fakeFold{"dep1": {TaskID: "spexmachina-abc", BeadStatus: "in_progress"}}

	refs, err := ResolveDeps([]string{"dep1"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Ref{Kind: RefTask, TaskID: "spexmachina-abc"}
	if len(refs) != 1 || refs[0] != want {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
}

// TestResolveDeps_FoldAbsentIsDropped pins the real v4 shape
// arch_resolver.md's "The Two Ref Shapes" describes: there is no "closed"
// status to read, only absence from the task-state artifact — a fold
// pairing whose BeadStatus is unset (plan/node_matcher.go's Pairing doc:
// "A pairing for which no bead was supplied arrives with BeadStatus unset")
// is dropped.
func TestResolveDeps_FoldAbsentIsDropped(t *testing.T) {
	batch := map[string]string{}
	fold := fakeFold{"dep1": {TaskID: "spexmachina-abc"}}

	refs, err := ResolveDeps([]string{"dep1"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("want dropped dep to produce no ref, got %+v", refs)
	}
}

func TestResolveDeps_UnresolvableIsError(t *testing.T) {
	_, err := ResolveDeps([]string{"missing"}, map[string]string{}, fakeFold{})
	if err == nil {
		t.Fatal("want error for a dep neither in-batch nor in the fold")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error should name the spec_node_id: %v", err)
	}
}

func TestResolveDeps_PreservesInputOrder(t *testing.T) {
	batch := map[string]string{"a": "op-3", "c": "op-1"}
	fold := fakeFold{"b": {TaskID: "spexmachina-b", BeadStatus: "open"}}

	refs, err := ResolveDeps([]string{"c", "b", "a"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Ref{
		{Kind: RefOp, OpID: "op-1"},
		{Kind: RefTask, TaskID: "spexmachina-b"},
		{Kind: RefOp, OpID: "op-3"},
	}
	if len(refs) != len(want) {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("index %d: got %+v, want %+v", i, refs[i], want[i])
		}
	}
}

func TestResolveDeps_EmptyYieldsEmpty(t *testing.T) {
	refs, err := ResolveDeps(nil, map[string]string{}, fakeFold{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("want no refs, got %+v", refs)
	}
}

// --- ResolveDeps: retarget path takes the same classification (S6) ---

func TestResolveDeps_RetargetSameClassificationAsCreate(t *testing.T) {
	batch := map[string]string{"y": "op-2"}
	fold := fakeFold{
		"z": {TaskID: "spexmachina-z", BeadStatus: "open"},
		"w": {TaskID: "spexmachina-w"},
	}

	refs, err := ResolveDeps([]string{"y", "z", "w"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Ref{
		{Kind: RefOp, OpID: "op-2"},
		{Kind: RefTask, TaskID: "spexmachina-z"},
	}
	if len(refs) != len(want) {
		t.Fatalf("got %+v, want %+v (w's absent pairing should drop)", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("index %d: got %+v, want %+v", i, refs[i], want[i])
		}
	}
}

// --- Retarget-path pairing with ActionClassifier (S6) ---
//
// arch_resolver.md's "Test surface" carves this specific pairing out of
// test_changeset_builder's Builder.Build()-level coverage: "the
// retarget-path pairing with ActionClassifier lives in test_classification".
// These feed ActionClassifier's real, graph-derived DepSpecNodeIDs straight
// into ResolveDeps — not hand-written dep lists — so the two components'
// contract is exercised together, not just each one's shape in isolation.

// TestRetargetDeps_ClassifierOutputResolvesViaResolver pins S6's first two
// bullets: a retarget action's DepSpecNodeIDs, as ActionClassifier actually
// computes them from CompX's real uses/requires_module edges, resolve
// through Resolver with the same in-batch-wins-over-fold precedence and the
// same drop-if-absent rule a create's deps use.
func TestRetargetDeps_ClassifierOutputResolvesViaResolver(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: "open", After: "old"}}}

	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionRetarget {
		t.Fatalf("want 1 retarget action, got %+v", actions)
	}
	retarget := actions[0]

	// CompX's real DepSpecNodeIDs (collectDeps): CompY (direct uses), plus
	// CompZ and CompS (transitive requires_module, plan -> merkle -> schema).
	// Y is an in-batch create, Z is open in the fold, S is absent from the
	// fold and must drop.
	batch := map[string]string{f.CompY: "op-y"}
	fold := fakeFold{
		f.CompZ: {TaskID: "spexmachina-z", BeadStatus: "open"},
		f.CompS: {TaskID: "spexmachina-s"},
	}

	refs, err := ResolveDeps(retarget.DepSpecNodeIDs, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var want []Ref
	for _, id := range retarget.DepSpecNodeIDs {
		switch id {
		case f.CompY:
			want = append(want, Ref{Kind: RefOp, OpID: "op-y"})
		case f.CompZ:
			want = append(want, Ref{Kind: RefTask, TaskID: "spexmachina-z"})
		case f.CompS:
			// absent from the fold: dropped, no ref.
		default:
			t.Fatalf("unexpected dep in retarget.DepSpecNodeIDs: %s", id)
		}
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
}

// TestRetargetDeps_ClassifierOutputUnresolvedIsError pins S6's third
// bullet: the retarget path gets no laxer resolution than the create path —
// a dep that is neither in-batch nor in the fold is a plan error naming the
// spec_node_id, even when the dep list came from the classifier rather than
// a hand-written fixture.
func TestRetargetDeps_ClassifierOutputUnresolvedIsError(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: "open", After: "old"}}}

	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	retarget := actions[0]

	_, err = ResolveDeps(retarget.DepSpecNodeIDs, map[string]string{}, fakeFold{})
	if err == nil {
		t.Fatal("want a plan error when a retarget's dep is neither in-batch nor in the fold")
	}
	named := false
	for _, id := range retarget.DepSpecNodeIDs {
		if strings.Contains(err.Error(), id) {
			named = true
		}
	}
	if !named {
		t.Errorf("error should name the unresolvable spec_node_id, got: %v", err)
	}
}

// --- Test_section describes deps: the four fates, fed from the real
// classifier output (S10) ---
//
// S10 makes no Resolver change — describes-collected deps are ordinary
// spec_node_ids and ResolveDeps already classifies them by the same
// in-batch/fold/error precedence a component's uses deps take. These tests
// pin that a test_section's real, classifier-computed DepSpecNodeIDs (its
// describes array, per D10) resolve correctly through Resolver, one arm per
// fate, mirroring how S6's tests pair ActionClassifier's retarget output
// with Resolver rather than hand-writing dep lists.

// resolveTSManyCreate classifies TSMany plus whichever of CompX/CompY the
// caller also lists as unmatched creates, and returns TSMany's create
// action's DepSpecNodeIDs (sorted [CompX, CompY] per the fixture's
// TSMany.Describes).
func resolveTSManyCreate(t *testing.T, f classifierFixture, extra ...Unmatched) Action {
	t.Helper()
	u := append([]Unmatched{{Change: change(f.TSMany, "plan", "test_section", merkle.Added, "", "ts-hash")}}, extra...)
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, a := range actions {
		if a.SpecNodeID == f.TSMany {
			return a
		}
	}
	t.Fatalf("TSMany create action missing: %+v", actions)
	return Action{}
}

func TestResolveDeps_TestSectionDescribes_AllInBatchCreate(t *testing.T) {
	f := newClassifierFixture()
	tsAction := resolveTSManyCreate(t, f,
		Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
		Unmatched{Change: change(f.CompY, "plan", "component", merkle.Added, "", "cy-hash")},
	)

	batch := map[string]string{f.CompX: "op-x", f.CompY: "op-y"}
	refs, err := ResolveDeps(tsAction.DepSpecNodeIDs, batch, fakeFold{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var want []Ref
	for _, id := range tsAction.DepSpecNodeIDs {
		switch id {
		case f.CompX:
			want = append(want, Ref{Kind: RefOp, OpID: "op-x"})
		case f.CompY:
			want = append(want, Ref{Kind: RefOp, OpID: "op-y"})
		default:
			t.Fatalf("unexpected dep: %s", id)
		}
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("got %+v, want %+v — the test task is not actionable until its components exist", refs, want)
	}
}

func TestResolveDeps_TestSectionDescribes_OpenFoldPairingYieldsRefBead(t *testing.T) {
	f := newClassifierFixture()
	tsAction := resolveTSManyCreate(t, f,
		Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
	)

	batch := map[string]string{f.CompX: "op-x"}
	fold := fakeFold{f.CompY: {TaskID: "spexmachina-y", BeadStatus: "open"}}
	refs, err := ResolveDeps(tsAction.DepSpecNodeIDs, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var want []Ref
	for _, id := range tsAction.DepSpecNodeIDs {
		switch id {
		case f.CompX:
			want = append(want, Ref{Kind: RefOp, OpID: "op-x"})
		case f.CompY:
			want = append(want, Ref{Kind: RefTask, TaskID: "spexmachina-y"})
		default:
			t.Fatalf("unexpected dep: %s", id)
		}
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("got %+v, want %+v — the test task waits for the in-flight component work", refs, want)
	}
}

func TestResolveDeps_TestSectionDescribes_AbsentPairingIsDropped(t *testing.T) {
	f := newClassifierFixture()
	tsAction := resolveTSManyCreate(t, f,
		Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
	)

	batch := map[string]string{f.CompX: "op-x"}
	fold := fakeFold{f.CompY: {TaskID: "spexmachina-y"}}
	refs, err := ResolveDeps(tsAction.DepSpecNodeIDs, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []Ref{{Kind: RefOp, OpID: "op-x"}}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("got %+v, want %+v — a test against existing code stays immediately actionable, no ref for the dropped dep", refs, want)
	}
}

func TestResolveDeps_TestSectionDescribes_NoPairingIsError(t *testing.T) {
	f := newClassifierFixture()
	tsAction := resolveTSManyCreate(t, f,
		Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
	)

	batch := map[string]string{f.CompX: "op-x"}
	_, err := ResolveDeps(tsAction.DepSpecNodeIDs, batch, fakeFold{})
	if err == nil {
		t.Fatal("want a plan error when a described component has never been tracked by any journal event")
	}
	if !strings.Contains(err.Error(), f.CompY) {
		t.Fatalf("error should name CompY's spec_node_id, got: %v", err)
	}
}

// TestRetargetDeps_TestSectionDescribes_ReMintedSuccessorGainsRefOp pins
// S10's closing line: a test section retargeted in a batch that re-mints
// one of its described components gains a ref:op dep on the successor,
// applied add-only per S6.
func TestRetargetDeps_TestSectionDescribes_ReMintedSuccessorGainsRefOp(t *testing.T) {
	f := newClassifierFixture()
	matches := []Match{
		{
			Change:  change(f.TSMany, "plan", "test_section", merkle.Modified, "old-ts", "new-ts"),
			Records: []Pairing{{TaskID: "spex-ts", BeadStatus: "open", After: "old-ts"}},
		},
		{
			Change:  change(f.CompY, "plan", "component", merkle.Modified, "old-y", "new-y"),
			Records: []Pairing{{TaskID: "spex-y-old"}},
		},
	}
	actions, err := ClassifyActions(matches, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var tsAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == f.TSMany && actions[i].Type == ActionRetarget {
			tsAction = &actions[i]
		}
	}
	if tsAction == nil {
		t.Fatalf("TSMany retarget action missing: %+v", actions)
	}

	batch := map[string]string{f.CompY: "op-y-successor"}
	fold := fakeFold{f.CompX: {TaskID: "spexmachina-x", BeadStatus: "open"}}
	refs, err := ResolveDeps(tsAction.DepSpecNodeIDs, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var want []Ref
	for _, id := range tsAction.DepSpecNodeIDs {
		switch id {
		case f.CompX:
			want = append(want, Ref{Kind: RefTask, TaskID: "spexmachina-x"})
		case f.CompY:
			want = append(want, Ref{Kind: RefOp, OpID: "op-y-successor"})
		default:
			t.Fatalf("unexpected dep: %s", id)
		}
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("got %+v, want %+v — the retargeted section gains a ref:op dep on the re-minted successor", refs, want)
	}
}

// --- ResolveEpicAction: fold vs. registration precedence ---

func TestResolveEpicAction_FoldPairingSkipsCreate(t *testing.T) {
	fold := fakeFold{"proposal-a": {TaskID: "spexmachina-epic"}}

	action, err := ResolveEpicAction("proposal-a", fold, Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatalf("want no new epic action when fold already pairs one, got %+v", action)
	}
}

func TestResolveEpicAction_FoldWinsOverRegistration(t *testing.T) {
	fold := fakeFold{"proposal-a": {TaskID: "spexmachina-epic"}}
	reg := Registration{EID: "eid-registered"}

	action, err := ResolveEpicAction("proposal-a", fold, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatalf("an existing fold pairing must win over an in-batch registration, got %+v", action)
	}
}

func TestResolveEpicAction_NoFoldWithRegistration_ReturnsNewEpicAction(t *testing.T) {
	reg := Registration{EID: "eid-registered"}

	action, err := ResolveEpicAction("proposal-a", fakeFold{}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action == nil {
		t.Fatal("want a new epic action")
	}
	if action.Type != ActionCreate {
		t.Errorf("Type = %q, want %q", action.Type, ActionCreate)
	}
	if action.NodeType != KindProposalEpic {
		t.Errorf("NodeType = %q, want %q", action.NodeType, KindProposalEpic)
	}
	if action.SpecNodeID != "proposal-a" {
		t.Errorf("SpecNodeID = %q, want the proposal ref itself", action.SpecNodeID)
	}
	if !strings.Contains(action.Reason, "proposal-a") {
		t.Errorf("Reason = %q, want it to name the proposal", action.Reason)
	}
}

func TestResolveEpicAction_NoFoldNoRegistration_IsError(t *testing.T) {
	_, err := ResolveEpicAction("proposal-a", fakeFold{}, Registration{})
	if err == nil {
		t.Fatal("want a plan error when neither the fold nor the registration answers")
	}
	if !strings.Contains(err.Error(), "proposal-a") {
		t.Fatalf("error should name the proposal slug: %v", err)
	}
}

// --- ResolveParent ---

func TestResolveParent_ExistingEpicYieldsRefBead(t *testing.T) {
	fold := fakeFold{"proposal-a": {TaskID: "spexmachina-epic"}}

	ref, err := ResolveParent("proposal-a", fold, Registration{}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Ref{Kind: RefTask, TaskID: "spexmachina-epic"}
	if ref != want {
		t.Fatalf("got %+v, want %+v", ref, want)
	}
}

func TestResolveParent_NewEpicYieldsRefOpFromBatch(t *testing.T) {
	reg := Registration{EID: "eid-registered"}
	batch := map[string]string{"proposal-a": "op-1"}

	ref, err := ResolveParent("proposal-a", fakeFold{}, reg, batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Ref{Kind: RefOp, OpID: "op-1"}
	if ref != want {
		t.Fatalf("got %+v, want %+v", ref, want)
	}
}

func TestResolveParent_NoRegistrationNoFold_IsError(t *testing.T) {
	_, err := ResolveParent("proposal-a", fakeFold{}, Registration{}, map[string]string{})
	if err == nil {
		t.Fatal("want a plan error naming the slug")
	}
	if !strings.Contains(err.Error(), "proposal-a") {
		t.Fatalf("error should name the proposal slug: %v", err)
	}
}

func TestResolveParent_NewEpicMissingFromBatch_IsError(t *testing.T) {
	reg := Registration{EID: "eid-registered"}

	_, err := ResolveParent("proposal-a", fakeFold{}, reg, map[string]string{})
	if err == nil {
		t.Fatal("want an error when the epic's op is missing from the sorted batch")
	}
}

// --- ResolvePriority: the implements -> preq_id -> priority chain ---

// priorityFixture builds a small project + module carrying every priority-
// chain shape: a full chain, a chain with no preq_id on the module
// requirement, a preq_id naming no project requirement, and a project
// requirement with no priority set.
type priorityFixture struct {
	ModID                                                     string
	ReqX, ReqY, ReqNoPreq, ReqDanglingPreq, ReqNoProjPriority string
	PreqA, PreqB, PreqNoPriority                              string
	CompFull, CompSingle, CompNoImplements                    string
	CompBadModReq, CompDanglingPreq, CompNoProjPriority       string
	Graph                                                     SpecGraph
}

func newPriorityFixture() priorityFixture {
	f := priorityFixture{
		ModID:              schema.IdentityHash("module", "plan"),
		ReqX:               schema.IdentityHash("plan", "requirement", "ReqX"),
		ReqY:               schema.IdentityHash("plan", "requirement", "ReqY"),
		ReqNoPreq:          schema.IdentityHash("plan", "requirement", "ReqNoPreq"),
		ReqDanglingPreq:    schema.IdentityHash("plan", "requirement", "ReqDanglingPreq"),
		ReqNoProjPriority:  schema.IdentityHash("plan", "requirement", "ReqNoProjPriority"),
		PreqA:              schema.IdentityHash("project", "requirement", "PreqA"),
		PreqB:              schema.IdentityHash("project", "requirement", "PreqB"),
		PreqNoPriority:     schema.IdentityHash("project", "requirement", "PreqNoPriority"),
		CompFull:           schema.IdentityHash("plan", "component", "CompFull"),
		CompSingle:         schema.IdentityHash("plan", "component", "CompSingle"),
		CompNoImplements:   schema.IdentityHash("plan", "component", "CompNoImplements"),
		CompBadModReq:      schema.IdentityHash("plan", "component", "CompBadModReq"),
		CompDanglingPreq:   schema.IdentityHash("plan", "component", "CompDanglingPreq"),
		CompNoProjPriority: schema.IdentityHash("plan", "component", "CompNoProjPriority"),
	}

	proj := schema.Project{
		Modules: []schema.Module{{ID: f.ModID, Name: "plan"}},
		Requirements: []schema.Requirement{
			{ID: f.PreqA, Priority: intp(1)},
			{ID: f.PreqB, Priority: intp(2)},
			{ID: f.PreqNoPriority},
		},
	}
	specs := map[string]schema.ModuleSpec{
		f.ModID: {
			Name: "plan",
			Requirements: []schema.ModuleRequirement{
				{ID: f.ReqX, PreqID: f.PreqA},
				{ID: f.ReqY, PreqID: f.PreqB},
				{ID: f.ReqNoPreq},
				{ID: f.ReqDanglingPreq, PreqID: "no-such-preq"},
				{ID: f.ReqNoProjPriority, PreqID: f.PreqNoPriority},
			},
			Components: []schema.Component{
				{ID: f.CompFull, Name: "CompFull", Implements: []string{f.ReqX, f.ReqY}},
				{ID: f.CompSingle, Name: "CompSingle", Implements: []string{f.ReqY}},
				{ID: f.CompNoImplements, Name: "CompNoImplements"},
				{ID: f.CompBadModReq, Name: "CompBadModReq", Implements: []string{f.ReqNoPreq}},
				{ID: f.CompDanglingPreq, Name: "CompDanglingPreq", Implements: []string{f.ReqDanglingPreq}},
				{ID: f.CompNoProjPriority, Name: "CompNoProjPriority", Implements: []string{f.ReqNoProjPriority}},
			},
		},
	}
	f.Graph = NewSpecGraph(proj, specs)
	return f
}

func TestResolvePriority_MinAcrossImplementsSet(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompFull, f.Graph)
	if got != 1 {
		t.Errorf("got %d, want min(1,2) = 1", got)
	}
}

func TestResolvePriority_SingleImplements(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompSingle, f.Graph)
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestResolvePriority_NoImplements_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompNoImplements, f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_ModuleRequirementMissingPreqID_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompBadModReq, f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_PreqIDNamesMissingProjectRequirement_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompDanglingPreq, f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_ProjectRequirementNoPriority_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", f.CompNoProjPriority, f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_UnknownModule_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("no-such-module", f.CompFull, f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_UnknownComponent_Fallback(t *testing.T) {
	f := newPriorityFixture()
	got := ResolvePriority("plan", "no-such-component", f.Graph)
	if got != FallbackPriority {
		t.Errorf("got %d, want fallback %d", got, FallbackPriority)
	}
}

func TestResolvePriority_DeterministicRegardlessOfImplementsOrder(t *testing.T) {
	f := newPriorityFixture()

	proj := schema.Project{
		Modules: []schema.Module{{ID: f.ModID, Name: "plan"}},
		Requirements: []schema.Requirement{
			{ID: f.PreqA, Priority: intp(1)},
			{ID: f.PreqB, Priority: intp(2)},
		},
	}
	specs := map[string]schema.ModuleSpec{
		f.ModID: {
			Name: "plan",
			Requirements: []schema.ModuleRequirement{
				{ID: f.ReqX, PreqID: f.PreqA},
				{ID: f.ReqY, PreqID: f.PreqB},
			},
			Components: []schema.Component{
				{ID: f.CompFull, Name: "CompFull", Implements: []string{f.ReqY, f.ReqX}},
			},
		},
	}
	shuffled := NewSpecGraph(proj, specs)

	got := ResolvePriority("plan", f.CompFull, shuffled)
	if got != 1 {
		t.Errorf("got %d, want 1 regardless of implements order", got)
	}
}
