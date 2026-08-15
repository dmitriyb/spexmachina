package plan

import (
	"strings"
	"testing"

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
	want := Ref{Kind: RefBead, BeadID: "spexmachina-abc"}
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
	want := Ref{Kind: RefBead, BeadID: "spexmachina-abc"}
	if len(refs) != 1 || refs[0] != want {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
}

func TestResolveDeps_FoldClosedIsDropped(t *testing.T) {
	batch := map[string]string{}
	fold := fakeFold{"dep1": {TaskID: "spexmachina-abc", BeadStatus: "closed"}}

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
		{Kind: RefBead, BeadID: "spexmachina-b"},
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
		"w": {TaskID: "spexmachina-w", BeadStatus: "closed"},
	}

	refs, err := ResolveDeps([]string{"y", "z", "w"}, batch, fold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Ref{
		{Kind: RefOp, OpID: "op-2"},
		{Kind: RefBead, BeadID: "spexmachina-z"},
	}
	if len(refs) != len(want) {
		t.Fatalf("got %+v, want %+v (w's closed pairing should drop)", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("index %d: got %+v, want %+v", i, refs[i], want[i])
		}
	}
}

// TestResolveDeps_RetargetUnresolvableDepIsError pins S6's last bullet: "the
// retarget path gets no laxer resolution than the create path." ResolveDeps
// is the identical function call for a retarget's freshly recomputed deps as
// for a create's (see Builder.retargetOp), so a dep neither in-batch nor
// tracked by the fold is a plan error regardless of which action shape
// produced the DepSpecNodeIDs list.
func TestResolveDeps_RetargetUnresolvableDepIsError(t *testing.T) {
	_, err := ResolveDeps([]string{"ghost"}, map[string]string{}, fakeFold{})
	if err == nil {
		t.Fatal("want error for a retarget dep that is neither in-batch nor tracked by the fold")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error should name the spec_node_id: %v", err)
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
	want := Ref{Kind: RefBead, BeadID: "spexmachina-epic"}
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
