package plan

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// classifierFixture builds a small four-module spec graph exercising every
// edge shape ActionClassifier walks:
//
//   - plan/CompX uses plan/CompY (direct, same module)
//   - plan/CompY uses plan/CompW (direct) — proves uses is not transitive
//   - plan requires_module merkle, merkle requires_module schema — proves
//     requires_module is transitive across two hops
//   - cyclicA and cyclicB require each other — proves cycle termination
//   - plan/FlowF uses plan/CompX — the data_flow add-on
//   - plan/TSOne describes only CompX (fold-back); plan/TSMany describes
//     CompX and CompY (still owes its own task)
type classifierFixture struct {
	PlanMod, MerkleMod, SchemaMod, CyclicAMod, CyclicBMod string
	CompX, CompY, CompW, CompZ, CompS, CompA, CompB       string
	FlowF                                                 string
	TSOne, TSMany                                         string
	Graph                                                 SpecGraph
}

func newClassifierFixture() classifierFixture {
	f := classifierFixture{
		PlanMod:    schema.IdentityHash("module", "plan"),
		MerkleMod:  schema.IdentityHash("module", "merkle"),
		SchemaMod:  schema.IdentityHash("module", "schema"),
		CyclicAMod: schema.IdentityHash("module", "cyclicA"),
		CyclicBMod: schema.IdentityHash("module", "cyclicB"),
		CompX:      schema.IdentityHash("plan", "component", "CompX"),
		CompY:      schema.IdentityHash("plan", "component", "CompY"),
		CompW:      schema.IdentityHash("plan", "component", "CompW"),
		CompZ:      schema.IdentityHash("merkle", "component", "CompZ"),
		CompS:      schema.IdentityHash("schema", "component", "CompS"),
		CompA:      schema.IdentityHash("cyclicA", "component", "CompA"),
		CompB:      schema.IdentityHash("cyclicB", "component", "CompB"),
		FlowF:      schema.IdentityHash("plan", "data_flow", "FlowF"),
		TSOne:      schema.IdentityHash("plan", "test_section", "TSOne"),
		TSMany:     schema.IdentityHash("plan", "test_section", "TSMany"),
	}

	proj := schema.Project{
		Modules: []schema.Module{
			{ID: f.PlanMod, Name: "plan", RequiresModule: []string{f.MerkleMod}},
			{ID: f.MerkleMod, Name: "merkle", RequiresModule: []string{f.SchemaMod}},
			{ID: f.SchemaMod, Name: "schema"},
			{ID: f.CyclicAMod, Name: "cyclicA", RequiresModule: []string{f.CyclicBMod}},
			{ID: f.CyclicBMod, Name: "cyclicB", RequiresModule: []string{f.CyclicAMod}},
		},
	}
	specs := map[string]schema.ModuleSpec{
		f.PlanMod: {
			Name: "plan",
			Components: []schema.Component{
				{ID: f.CompX, Name: "CompX", Uses: []string{f.CompY}},
				{ID: f.CompY, Name: "CompY", Uses: []string{f.CompW}},
				{ID: f.CompW, Name: "CompW"},
			},
			DataFlows: []schema.DataFlow{
				{ID: f.FlowF, Name: "FlowF", Uses: []string{f.CompX}},
			},
			TestSections: []schema.TestSection{
				{ID: f.TSOne, Name: "TSOne", Describes: []string{f.CompX}},
				{ID: f.TSMany, Name: "TSMany", Describes: []string{f.CompX, f.CompY}},
			},
		},
		f.MerkleMod: {
			Name:       "merkle",
			Components: []schema.Component{{ID: f.CompZ, Name: "CompZ"}},
		},
		f.SchemaMod: {
			Name:       "schema",
			Components: []schema.Component{{ID: f.CompS, Name: "CompS"}},
		},
		f.CyclicAMod: {
			Name:       "cyclicA",
			Components: []schema.Component{{ID: f.CompA, Name: "CompA"}},
		},
		f.CyclicBMod: {
			Name:       "cyclicB",
			Components: []schema.Component{{ID: f.CompB, Name: "CompB"}},
		},
	}
	f.Graph = NewSpecGraph(proj, specs)
	return f
}

func change(key, module, nodeType string, ct merkle.ChangeType, oldHash, newHash string) merkle.ClassifiedChange {
	return merkle.ClassifiedChange{
		Change: merkle.Change{Key: key, Type: ct, NodeType: nodeType, OldHash: oldHash, NewHash: newHash},
		Module: module,
	}
}

func sortedStrings(ss ...string) []string {
	out := append([]string(nil), ss...)
	sort.Strings(out)
	return out
}

// --- Node-type gate (unmatched path only) ---

func TestGateAdmits_NodeTypeTable(t *testing.T) {
	f := newClassifierFixture()
	tests := []struct {
		name     string
		nodeType string
		key      string
		want     bool
	}{
		{"component", "component", f.CompX, true},
		{"data_flow", "data_flow", f.FlowF, true},
		{"test_section_two_or_more", "test_section", f.TSMany, true},
		{"test_section_one", "test_section", f.TSOne, false},
		{"api_never", "api", "some-api-id", false},
		{"meta_never", "meta", "meta-id", false},
		{"requirement_never", "requirement", "req-id", false},
		{"module_dead_row_still_admitted", "module", "module-id", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := change(tt.key, "plan", tt.nodeType, merkle.Added, "", "hash1")
			if got := gateAdmits(c, f.Graph); got != tt.want {
				t.Errorf("gateAdmits(%s): got %v want %v", tt.nodeType, got, tt.want)
			}
		})
	}
}

func TestGateAdmits_TestSection_UnestablishedCouplingIsAdmitted(t *testing.T) {
	f := newClassifierFixture()
	if !gateAdmits(change("ts-1", "no-such-module", "test_section", merkle.Added, "", "h"), f.Graph) {
		t.Errorf("want admitted when the module cannot be resolved")
	}
	if !gateAdmits(change("ts-2", "plan", "test_section", merkle.Added, "", "h"), f.Graph) {
		t.Errorf("want admitted when the section cannot be resolved in its module")
	}
}

// --- Unmatched path: create actions and their reasons ---

func TestClassifyUnmatched_AddedProducesCreateWithNewSpecNodeReason(t *testing.T) {
	f := newClassifierFixture()
	u := Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "newhash")}
	actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %+v", actions)
	}
	a := actions[0]
	if a.Type != ActionCreate || a.Module != "plan" || a.Node != "CompX" || a.NodeType != "component" ||
		a.SpecNodeID != f.CompX || a.SpecHash != "newhash" || a.BeadID != "" {
		t.Fatalf("unexpected action: %+v", a)
	}
	if a.Reason != "New spec node: plan/CompX" {
		t.Errorf("reason: got %q", a.Reason)
	}
	want := sortedStrings(f.CompY, f.CompZ, f.CompS)
	if !reflect.DeepEqual(a.DepSpecNodeIDs, want) {
		t.Errorf("deps: got %v want %v", a.DepSpecNodeIDs, want)
	}
}

func TestClassifyUnmatched_ModifiedProducesModifiedNewReason(t *testing.T) {
	f := newClassifierFixture()
	u := Unmatched{Change: change(f.CompW, "plan", "component", merkle.Modified, "old", "new")}
	actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1, got %+v", actions)
	}
	if actions[0].Reason != "Spec node modified (new): plan/CompW" {
		t.Errorf("reason: got %q", actions[0].Reason)
	}
}

func TestClassifyUnmatched_GateDropsNonAdmittedNodeTypes(t *testing.T) {
	f := newClassifierFixture()
	cases := []merkle.ClassifiedChange{
		change("api-1", "plan", "api", merkle.Added, "", "h"),
		change("meta-1", "plan", "meta", merkle.Modified, "a", "b"),
		change("req-1", "plan", "requirement", merkle.Modified, "a", "b"),
		change(f.TSOne, "plan", "test_section", merkle.Modified, "a", "b"),
	}
	var u []Unmatched
	for _, c := range cases {
		u = append(u, Unmatched{Change: c})
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("want 0 actions (all gated out), got %+v", actions)
	}
}

// TestClassifyUnmatched_ApiYieldsNoActions pins the api invariant (arch_action_classifier.md,
// "That invariant lives only in the absence of an `api` entry in the bead-producing set, so it
// is pinned by a dedicated test rather than by the shape of the code") over both change types
// the gate is what makes the difference for: added and modified.
func TestClassifyUnmatched_ApiYieldsNoActions(t *testing.T) {
	f := newClassifierFixture()
	for _, ct := range []merkle.ChangeType{merkle.Added, merkle.Modified} {
		t.Run(ct.String(), func(t *testing.T) {
			u := Unmatched{Change: change("api-1", "plan", "api", ct, "old", "new")}
			actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(actions) != 0 {
				t.Fatalf("want 0 actions for an api change, got %+v", actions)
			}
		})
	}
}

func TestClassifyUnmatched_DataFlowAndMultiComponentTestSectionAdmitted(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.FlowF, "plan", "data_flow", merkle.Added, "", "fh")},
		{Change: change(f.TSMany, "plan", "test_section", merkle.Added, "", "th")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("want 2, got %+v", actions)
	}
}

// TestClassifyUnmatched_RemovedYieldsNoAction pins S4's second bullet:
// "Unmatched removed change -> no action (nothing to obsolete)" — also
// arch_action_classifier.md:51's "removed | no | no action" row.
// MatchNodes never actually routes a removed change into the unmatched
// list (TestE3_RemovedChangeNoRecord pins that upstream invariant), but
// ClassifyActions must not depend on that alone: fed one directly, it must
// still refuse to synthesize a create.
func TestClassifyUnmatched_RemovedYieldsNoAction(t *testing.T) {
	f := newClassifierFixture()
	u := Unmatched{Change: change(f.CompX, "plan", "component", merkle.Removed, "old", "")}
	actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("want 0 actions for an unmatched removed change, got %+v", actions)
	}
}

// --- Matched path: the state transition table's status split (S1) ---

func TestClassifyMatched_StatusSplit(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")

	t.Run("closed_obsoletes_and_creates_successor", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-001", BeadStatus: "closed"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 2 {
			t.Fatalf("want 2 actions, got %+v", actions)
		}
		var create, obsolete *Action
		for i := range actions {
			switch actions[i].Type {
			case ActionCreate:
				create = &actions[i]
			case ActionObsolete:
				obsolete = &actions[i]
			}
		}
		if obsolete == nil || create == nil {
			t.Fatalf("want one create and one obsolete: %+v", actions)
		}
		if obsolete.BeadID != "spex-001" || obsolete.ChangeType != "modified" {
			t.Errorf("obsolete: %+v", *obsolete)
		}
		if create.OldBeadID != "spex-001" || create.SpecHash != "new" || create.BeadID != "" {
			t.Errorf("create: %+v", *create)
		}
		if create.Reason != "Spec node modified (new): plan/CompX" {
			t.Errorf("create reason: got %q", create.Reason)
		}
		if obsolete.Reason != "Spec node modified: plan/CompX" {
			t.Errorf("obsolete reason: got %q", obsolete.Reason)
		}
	})

	t.Run("open_retargets_only", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: "open", After: "old"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 1 || actions[0].Type != ActionRetarget {
			t.Fatalf("want 1 retarget, got %+v", actions)
		}
		a := actions[0]
		if a.BeadID != "spex-003" || a.SpecHash != "new" {
			t.Errorf("retarget: %+v", a)
		}
		if a.Reason != "Spec node modified (retarget): plan/CompX" {
			t.Errorf("retarget reason: got %q", a.Reason)
		}
	})

	t.Run("in_progress_refuses_the_run", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-007", BeadStatus: "in_progress"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err == nil {
			t.Fatalf("want refusal error")
		}
		if !strings.Contains(err.Error(), "spex-007") {
			t.Errorf("error must name the claimed task: %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("no action list is returned at all on refusal, got %+v", actions)
		}
	})

	t.Run("unknown_status_takes_closed_path", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-099"}}} // BeadStatus unset
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 2 {
			t.Fatalf("want obsolete+create for an unjoined status, got %+v", actions)
		}
	})
}

func TestClassifyMatched_RefusalIsTotal_NamesEveryClaimedTask(t *testing.T) {
	f := newClassifierFixture()
	m1 := Match{
		Change:  change(f.CompX, "plan", "component", merkle.Modified, "a", "b"),
		Records: []Pairing{{TaskID: "spex-007", BeadStatus: "in_progress"}},
	}
	m2 := Match{
		Change:  change(f.CompY, "plan", "component", merkle.Modified, "a", "b"),
		Records: []Pairing{{TaskID: "spex-008", BeadStatus: "in_progress"}},
	}
	m3 := Match{
		Change:  change(f.CompZ, "merkle", "component", merkle.Modified, "a", "b"),
		Records: []Pairing{{TaskID: "spex-009", BeadStatus: "closed"}},
	}
	actions, err := ClassifyActions([]Match{m1, m2, m3}, nil, nil, f.Graph)
	if err == nil {
		t.Fatalf("want error")
	}
	for _, want := range []string{"spex-007", "spex-008"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing claimed task %s: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "spex-009") {
		t.Errorf("error must not name a non-claimed task: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("no action list is returned at all on refusal — m3's cleanly classifiable entry must not leak, got %+v", actions)
	}
}

// --- Already-tracked cell (S2), checked before the status split ---

func TestClassifyMatched_AlreadyTrackedYieldsNoAction(t *testing.T) {
	f := newClassifierFixture()
	statuses := []string{"open", "in_progress", "closed", ""}
	for _, status := range statuses {
		t.Run("status_"+status, func(t *testing.T) {
			c := change(f.CompX, "plan", "component", merkle.Modified, "old", "same")
			m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: status, After: "same"}}}
			actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(actions) != 0 {
				t.Fatalf("want 0 actions, got %+v", actions)
			}
		})
	}
}

func TestClassifyMatched_DifferingAfterHashStillRetargets(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: "open", After: "different"}}}
	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionRetarget {
		t.Fatalf("want retarget when after != current hash, got %+v", actions)
	}
}

// --- Added-with-differing-hash behaves exactly as modified (S3) ---

func TestClassifyMatched_AddedWithExistingPairing_BehavesLikeModified(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Added, "", "new")

	openM := Match{Change: c, Records: []Pairing{{TaskID: "spex-x", BeadStatus: "open", After: "old"}}}
	if actions, err := ClassifyActions([]Match{openM}, nil, nil, f.Graph); err != nil || len(actions) != 1 || actions[0].Type != ActionRetarget {
		t.Errorf("open pairing: want single retarget, got actions=%+v err=%v", actions, err)
	}

	closedM := Match{Change: c, Records: []Pairing{{TaskID: "spex-y", BeadStatus: "closed"}}}
	if actions, err := ClassifyActions([]Match{closedM}, nil, nil, f.Graph); err != nil || len(actions) != 2 {
		t.Errorf("closed pairing: want obsolete+create, got actions=%+v err=%v", actions, err)
	}

	inProgressM := Match{Change: c, Records: []Pairing{{TaskID: "spex-z", BeadStatus: "in_progress"}}}
	if _, err := ClassifyActions([]Match{inProgressM}, nil, nil, f.Graph); err == nil {
		t.Errorf("in_progress pairing: want refusal")
	}
}

// --- test_section fold-back (E4): precedes the status split entirely ---

func TestFoldback_BeatsStatusSplit(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.TSOne, "plan", "test_section", merkle.Modified, "old", "new")
	for _, status := range []string{"open", "in_progress", "closed"} {
		t.Run(status, func(t *testing.T) {
			m := Match{Change: c, Records: []Pairing{{TaskID: "spex-ts", BeadStatus: status}}}
			actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(actions) != 1 || actions[0].Type != ActionObsolete {
				t.Fatalf("want exactly 1 obsolete action, got %+v", actions)
			}
		})
	}
}

func TestFoldback_PrecedenceIsDescribesLengthNotNodeType(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.TSMany, "plan", "test_section", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-tm", BeadStatus: "open"}}}
	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionRetarget {
		t.Fatalf("want retarget (describes >= 2 still owes a task), got %+v", actions)
	}
}

// --- Orphaned path (removed nodes) ---

func TestClassifyOrphaned_OpenYieldsObsoleteOnly(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-010", Module: "plan", Name: "Legacy", BeadStatus: "open"},
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionObsolete || actions[0].ChangeType != "removed" {
		t.Fatalf("want 1 obsolete action, got %+v", actions)
	}
	if actions[0].Reason != "Spec node removed: plan/Legacy" {
		t.Errorf("reason: got %q", actions[0].Reason)
	}
}

func TestClassifyOrphaned_InProgressYieldsObsoleteOnly(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-011", Module: "plan", Name: "Legacy", BeadStatus: "in_progress"},
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionObsolete {
		t.Fatalf("want 1 obsolete action (in_progress: nothing shipped), got %+v", actions)
	}
}

func TestClassifyOrphaned_ClosedYieldsObsoletePlusCleanupCreate(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-010", Module: "plan", Name: "Legacy", BeadStatus: "closed"},
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("want 2 actions, got %+v", actions)
	}
	var obsolete, cleanup *Action
	for i := range actions {
		switch actions[i].Type {
		case ActionObsolete:
			obsolete = &actions[i]
		case ActionCreate:
			cleanup = &actions[i]
		}
	}
	if obsolete == nil || cleanup == nil {
		t.Fatalf("want one obsolete and one create: %+v", actions)
	}
	if cleanup.OldBeadID != "spex-010" || cleanup.SpecHash != "" {
		t.Errorf("cleanup create: %+v", *cleanup)
	}
	if cleanup.Reason != "Code cleanup: plan/Legacy" {
		t.Errorf("cleanup reason: got %q", cleanup.Reason)
	}
	if len(cleanup.DepSpecNodeIDs) != 0 {
		t.Errorf("cleanup create must carry no deps, got %v", cleanup.DepSpecNodeIDs)
	}
}

// --- DepSpecNodeIDs collection (D1-D9) ---

func TestDeps_ComponentUsesDirectAndSelfFiltered(t *testing.T) {
	f := newClassifierFixture()
	deps := collectDeps("plan", f.CompX, f.Graph)
	// CompX uses CompY directly; plan requires_module merkle -> schema
	// transitively, contributing CompZ and CompS. CompW (CompY's own use)
	// must not appear: uses is not transitive (D4).
	want := sortedStrings(f.CompY, f.CompZ, f.CompS)
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("got %v want %v", deps, want)
	}
	for _, d := range deps {
		if d == f.CompX {
			t.Errorf("self-reference must be filtered: %v", deps)
		}
		if d == f.CompW {
			t.Errorf("uses must not be transitive: %v", deps)
		}
	}
}

// TestDeps_UsesAndRequiresModuleOverlapCollapsesToOne pins D5: "duplicates
// collapse to one entry." newClassifierFixture's CompX.Uses ([CompY], same
// module) never overlaps its requires_module-transitive contributions
// ([CompZ, CompS], from other modules), so no fixture path there produces a
// literal duplicate to collapse — this builds one where a component's uses
// edge names a component its own module's transitive requires_module walk
// also reaches.
func TestDeps_UsesAndRequiresModuleOverlapCollapsesToOne(t *testing.T) {
	planMod := schema.IdentityHash("module", "plan")
	merkleMod := schema.IdentityHash("module", "merkle")
	compX := schema.IdentityHash("plan", "component", "CompX")
	compZ := schema.IdentityHash("merkle", "component", "CompZ")

	proj := schema.Project{
		Modules: []schema.Module{
			{ID: planMod, Name: "plan", RequiresModule: []string{merkleMod}},
			{ID: merkleMod, Name: "merkle"},
		},
	}
	specs := map[string]schema.ModuleSpec{
		planMod: {
			Name:       "plan",
			Components: []schema.Component{{ID: compX, Name: "CompX", Uses: []string{compZ}}},
		},
		merkleMod: {
			Name:       "merkle",
			Components: []schema.Component{{ID: compZ, Name: "CompZ"}},
		},
	}
	graph := NewSpecGraph(proj, specs)

	deps := collectDeps("plan", compX, graph)
	if len(deps) != 1 || deps[0] != compZ {
		t.Fatalf("want CompZ named by both uses and the transitive requires_module walk collapsed to one entry, got %v", deps)
	}
}

func TestDeps_RequiresModuleTransitiveAcrossTwoHops(t *testing.T) {
	f := newClassifierFixture()
	deps := collectDeps("merkle", f.CompZ, f.Graph)
	want := sortedStrings(f.CompS)
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("got %v want %v", deps, want)
	}
}

func TestDeps_RequiresModuleCycleTerminates(t *testing.T) {
	f := newClassifierFixture()
	deps := collectDeps("cyclicA", f.CompA, f.Graph)
	want := sortedStrings(f.CompB)
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("cycle must terminate collecting each module once: got %v want %v", deps, want)
	}
}

func TestDeps_NoEdgesYieldsEmpty(t *testing.T) {
	f := newClassifierFixture()
	deps := collectDeps("schema", f.CompS, f.Graph)
	if len(deps) != 0 {
		t.Fatalf("want 0 deps, got %v", deps)
	}
}

func TestDeps_UnknownModuleYieldsEmpty(t *testing.T) {
	f := newClassifierFixture()
	deps := collectDeps("no-such-module", "whatever", f.Graph)
	if len(deps) != 0 {
		t.Fatalf("want 0 deps for an unresolvable module, got %v", deps)
	}
}

func TestDepsFor_NonComponentCollectsNothing(t *testing.T) {
	f := newClassifierFixture()
	flowChange := change(f.FlowF, "plan", "data_flow", merkle.Added, "", "h")
	if deps := depsFor(flowChange, f.Graph); len(deps) != 0 {
		t.Errorf("data_flow must collect no uses/requires_module deps, got %v", deps)
	}
	tsChange := change(f.TSMany, "plan", "test_section", merkle.Added, "", "h")
	if deps := depsFor(tsChange, f.Graph); len(deps) != 0 {
		t.Errorf("test_section must collect no uses/requires_module deps, got %v", deps)
	}
}

func TestObsoleteActions_NeverCarryDeps(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "a", "b")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-1", BeadStatus: "closed"}}}
	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, a := range actions {
		if a.Type == ActionObsolete && len(a.DepSpecNodeIDs) != 0 {
			t.Errorf("obsolete action must carry no deps: %+v", a)
		}
	}
}

func TestDeps_RetargetRecomputesFreshDeps(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-003", BeadStatus: "open", After: "old"}}}
	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 retarget, got %+v", actions)
	}
	want := sortedStrings(f.CompY, f.CompZ, f.CompS)
	if !reflect.DeepEqual(actions[0].DepSpecNodeIDs, want) {
		t.Fatalf("retarget deps: got %v want %v", actions[0].DepSpecNodeIDs, want)
	}
}

// --- Bead status is irrelevant to collection (D2) ---

func TestDeps_ClassifierIgnoresDependencyBeadStatus(t *testing.T) {
	f := newClassifierFixture()
	// CompX (unmatched, added) uses CompY directly. CompY itself is
	// matched with a closed pairing in the same run, so Y gets its own
	// obsolete+create pair — but that must not filter Y out of X's
	// DepSpecNodeIDs: filtering already-satisfied deps belongs to the
	// Resolver, not the classifier.
	unmatched := Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")}
	yMatch := Match{
		Change:  change(f.CompY, "plan", "component", merkle.Modified, "old", "new"),
		Records: []Pairing{{TaskID: "spex-y", BeadStatus: "closed"}},
	}
	actions, err := ClassifyActions([]Match{yMatch}, []Unmatched{unmatched}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var xAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == f.CompX {
			xAction = &actions[i]
		}
	}
	if xAction == nil {
		t.Fatalf("CompX action missing: %+v", actions)
	}
	found := false
	for _, d := range xAction.DepSpecNodeIDs {
		if d == f.CompY {
			found = true
		}
	}
	if !found {
		t.Errorf("D2: dependency collection must ignore Y's closed bead status, got deps=%v", xAction.DepSpecNodeIDs)
	}
}

// --- Data_flow add-on (D7, D8) ---

func TestDataFlowAddOn_ComponentGainsFlowDepWhenBothInBatch(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
		{Change: change(f.FlowF, "plan", "data_flow", merkle.Added, "", "flow-hash")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var compAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == f.CompX {
			compAction = &actions[i]
		}
	}
	if compAction == nil {
		t.Fatalf("component action missing: %+v", actions)
	}
	found := false
	for _, d := range compAction.DepSpecNodeIDs {
		if d == f.FlowF {
			found = true
		}
	}
	if !found {
		t.Errorf("component must gain a dep on the in-batch data_flow: %v", compAction.DepSpecNodeIDs)
	}
}

func TestDataFlowAddOn_NoDepWhenFlowOutsideBatch(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, d := range actions[0].DepSpecNodeIDs {
		if d == f.FlowF {
			t.Errorf("component must not gain a dep on an out-of-batch flow: %v", actions[0].DepSpecNodeIDs)
		}
	}
}

func TestDataFlowAddOn_ComponentsNotListedGainNothing(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.CompW, "plan", "component", merkle.Added, "", "cw-hash")}, // FlowF.Uses does not name CompW
		{Change: change(f.FlowF, "plan", "data_flow", merkle.Added, "", "flow-hash")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, a := range actions {
		if a.SpecNodeID != f.CompW {
			continue
		}
		for _, d := range a.DepSpecNodeIDs {
			if d == f.FlowF {
				t.Errorf("CompW is not named by FlowF's uses array, must not gain the dep: %v", a.DepSpecNodeIDs)
			}
		}
	}
}

func TestNonComponentCreates_CarryNoSpecGraphDeps(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.FlowF, "plan", "data_flow", merkle.Added, "", "fh")},
		{Change: change(f.TSMany, "plan", "test_section", merkle.Added, "", "th")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("want 2 actions, got %+v", actions)
	}
	for _, a := range actions {
		if len(a.DepSpecNodeIDs) != 0 {
			t.Errorf("node type %s must carry no deps from classification, got %v", a.NodeType, a.DepSpecNodeIDs)
		}
	}
}

// --- Names resolved from the graph; node types carried through (S8b) ---

// TestClassifyMatched_NamesResolvedFromGraph_AllNodeKinds pins S8b's first
// bullet over all three node kinds the matched path can carry — a
// component-only fixture would pass even if the resolver only ever handled
// components. Each subtest classifies a closed pairing (obsolete + create
// successor) so both actions' Node and NodeType are checked at once.
func TestClassifyMatched_NamesResolvedFromGraph_AllNodeKinds(t *testing.T) {
	f := newClassifierFixture()
	tests := []struct {
		name     string
		nodeID   string
		nodeType string
		wantName string
	}{
		{"component", f.CompX, "component", "CompX"},
		{"data_flow", f.FlowF, "data_flow", "FlowF"},
		{"test_section", f.TSMany, "test_section", "TSMany"}, // describes 2 components: no fold-back
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := change(tt.nodeID, "plan", tt.nodeType, merkle.Modified, "old", "new")
			m := Match{Change: c, Records: []Pairing{{TaskID: "spex-100", BeadStatus: "closed"}}}
			actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(actions) != 2 {
				t.Fatalf("want obsolete+create, got %+v", actions)
			}
			for _, a := range actions {
				if a.Node != tt.wantName {
					t.Errorf("%s action: Node = %q, want declared name %q, not the identity hash", a.Type, a.Node, tt.wantName)
				}
				if a.NodeType != tt.nodeType {
					t.Errorf("%s action: NodeType = %q, want %q", a.Type, a.NodeType, tt.nodeType)
				}
			}
		})
	}
}

// TestClassifyUnmatched_NameFallback_UnresolvedModule and
// TestClassifyUnmatched_NameFallback_UnresolvedHash pin S8b's fallback
// bullet: a change whose module the graph does not hold, and a change
// whose hash that module declares under no section of its type, both
// resolve the reason to "<module>/<hash>" rather than erroring, and
// classification continues (an action is still produced).

func TestClassifyUnmatched_NameFallback_UnresolvedModule(t *testing.T) {
	f := newClassifierFixture()
	u := Unmatched{Change: change("some-hash", "no-such-module", "component", merkle.Added, "", "h")}
	actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want classification to continue with 1 action, got %+v", actions)
	}
	if actions[0].Node != "some-hash" {
		t.Errorf("Node fallback: got %q, want the identity hash itself", actions[0].Node)
	}
	if want := "New spec node: no-such-module/some-hash"; actions[0].Reason != want {
		t.Errorf("reason: got %q, want %q", actions[0].Reason, want)
	}
}

func TestClassifyUnmatched_NameFallback_UnresolvedHashInKnownModule(t *testing.T) {
	f := newClassifierFixture()
	u := Unmatched{Change: change("unknown-hash", "plan", "component", merkle.Added, "", "h")}
	actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want classification to continue with 1 action, got %+v", actions)
	}
	if want := "New spec node: plan/unknown-hash"; actions[0].Reason != want {
		t.Errorf("reason: got %q, want %q", actions[0].Reason, want)
	}
}

// TestClassifyOrphaned_NameAndModuleFromJournalNotGraph pins S8b's last
// bullet: an orphaned pairing's module and name come from the journal
// pairing, never from the graph. The graph fixture holds a real node at
// f.CompX's identity hash named "CompX" in module "plan" — the orphaned
// pairing claims the same identity hash but a different module and name,
// and the classified action must still read the journal's values, proving
// the removed node's absence from the graph is not silently patched over
// by a lookup that happens to still resolve.
func TestClassifyOrphaned_NameAndModuleFromJournalNotGraph(t *testing.T) {
	f := newClassifierFixture()
	o := Orphaned{
		Record:   Pairing{SpecNodeID: f.CompX, TaskID: "spex-020", Module: "legacy-module", Name: "LegacyName", BeadStatus: "open"},
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %+v", actions)
	}
	if actions[0].Module != "legacy-module" || actions[0].Node != "LegacyName" {
		t.Errorf("want module/name from the journal pairing, got module=%q node=%q", actions[0].Module, actions[0].Node)
	}
}

// TestClassifyOrphaned_NodeTypeCarriedFromPairingRecord confirms the
// orphaned NodeType field (carried alongside the pairing because an
// identity hash does not embed a node type — test_classification.md's
// Setup section) reaches every resulting action unchanged, using a kind
// other than "component" so the assertion cannot pass on a coincidental
// default.
func TestClassifyOrphaned_NodeTypeCarriedFromPairingRecord(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-010", Module: "plan", Name: "Legacy", BeadStatus: "closed"},
		NodeType: "data_flow",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("want obsolete+cleanup create, got %+v", actions)
	}
	for _, a := range actions {
		if a.NodeType != "data_flow" {
			t.Errorf("%s action: NodeType = %q, want %q carried from the pairing record", a.Type, a.NodeType, "data_flow")
		}
	}
}

// --- Determinism (S9), empty inputs (E1), duplicate preservation (E2) ---

func TestClassifyActions_DeterministicAcrossShuffledInput(t *testing.T) {
	f := newClassifierFixture()
	matches := []Match{
		{
			Change:  change(f.CompX, "plan", "component", merkle.Modified, "a", "b"),
			Records: []Pairing{{TaskID: "spex-001", BeadStatus: "closed"}},
		},
		{
			Change:  change(f.CompZ, "merkle", "component", merkle.Modified, "a", "b"),
			Records: []Pairing{{TaskID: "spex-002", BeadStatus: "open", After: "a"}},
		},
	}
	unmatched := []Unmatched{
		{Change: change(f.CompW, "plan", "component", merkle.Added, "", "w")},
	}
	orphaned := []Orphaned{
		{Record: Pairing{SpecNodeID: "legacy", TaskID: "spex-010", Module: "plan", Name: "Legacy", BeadStatus: "closed"}, NodeType: "component"},
	}

	a1, err1 := ClassifyActions(matches, unmatched, orphaned, f.Graph)
	shuffled := []Match{matches[1], matches[0]}
	a2, err2 := ClassifyActions(shuffled, unmatched, orphaned, f.Graph)

	if err1 != nil || err2 != nil {
		t.Fatalf("errs: %v %v", err1, err2)
	}
	if !reflect.DeepEqual(a1, a2) {
		t.Fatalf("non-deterministic across shuffled match order:\nA=%+v\nB=%+v", a1, a2)
	}
	if len(a1) == 0 {
		t.Fatalf("expected a non-empty action list to compare")
	}
}

func TestClassifyActions_EmptyInputsYieldEmptyList(t *testing.T) {
	actions, err := ClassifyActions(nil, nil, nil, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("want empty action list, got %+v", actions)
	}
}

func TestClassifyActions_DuplicateEntriesAreNotDeduplicated(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.CompX, "plan", "component", merkle.Added, "", "h")
	matches := []Match{{Change: c, Records: []Pairing{{TaskID: "spex-001", BeadStatus: "open", After: "different"}}}}
	unmatched := []Unmatched{{Change: c}}

	actions, err := ClassifyActions(matches, unmatched, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("want 2 actions (one from the match, one from the unmatched duplicate), got %+v", actions)
	}
}
