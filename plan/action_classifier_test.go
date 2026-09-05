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

// TestGateAdmits_S5b_DefaultProfileMatchesHardcodedTable pins S5b's first
// arm: attaching schema.DefaultProfile() explicitly must not change a
// single verdict from the zero-value (no profile attached) graph, which
// falls back to the same default — the admitted set is read from the
// profile, but the default profile reproduces the old compiled-in set
// byte-for-byte. Each case carries its own expected verdict (matching
// TestGateAdmits_NodeTypeTable) so a change that quietly widened the
// admitted set would fail here even though zero-value and explicit-default
// still agreed with each other.
func TestGateAdmits_S5b_DefaultProfileMatchesHardcodedTable(t *testing.T) {
	f := newClassifierFixture()
	withDefault := f.Graph.WithProfile(schema.DefaultProfile())

	cases := []struct {
		change merkle.ClassifiedChange
		want   bool
	}{
		{change(f.CompX, "plan", "component", merkle.Added, "", "h"), true},
		{change(f.FlowF, "plan", "data_flow", merkle.Added, "", "h"), true},
		{change(f.TSMany, "plan", "test_section", merkle.Added, "", "h"), true},
		{change(f.TSOne, "plan", "test_section", merkle.Added, "", "h"), false},
		{change("api-1", "plan", "api", merkle.Added, "", "h"), false},
		{change("meta-1", "plan", "meta", merkle.Added, "", "h"), false},
		{change("req-1", "plan", "requirement", merkle.Added, "", "h"), false},
	}
	for _, tc := range cases {
		t.Run(tc.change.NodeType, func(t *testing.T) {
			zeroValue := gateAdmits(tc.change, f.Graph)
			explicit := gateAdmits(tc.change, withDefault)
			if zeroValue != tc.want {
				t.Errorf("gateAdmits(%s) no-profile: got %v want %v", tc.change.NodeType, zeroValue, tc.want)
			}
			if explicit != tc.want {
				t.Errorf("gateAdmits(%s) explicit default: got %v want %v", tc.change.NodeType, explicit, tc.want)
			}
		})
	}
}

// TestGateAdmits_S5b_ProfileDeclaredTypeBranchesOnDeclaration pins S5b's
// second arm: a profile naming a type the default never declared —
// "endpoint" — is admitted when that profile lists it in PlanRelevant, and
// the very same change is dropped under the default profile. The
// classifier never branches on the type name "endpoint" itself, only on
// whether the resolved profile declares it.
func TestGateAdmits_S5b_ProfileDeclaredTypeBranchesOnDeclaration(t *testing.T) {
	f := newClassifierFixture()
	endpointChange := change("endpoint-1", "plan", "endpoint", merkle.Added, "", "eh")

	withEndpoint := f.Graph.WithProfile(&schema.Profile{
		PlanRelevant: []string{"component", "data_flow", "test_section", "endpoint"},
	})

	t.Run("default_profile_drops_endpoint", func(t *testing.T) {
		u := Unmatched{Change: endpointChange}
		actions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 0 {
			t.Fatalf("want 0 actions under the default profile, got %+v", actions)
		}
	})

	t.Run("declaring_profile_admits_endpoint", func(t *testing.T) {
		u := Unmatched{Change: endpointChange}
		actions, err := ClassifyActions(nil, []Unmatched{u}, nil, withEndpoint)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("want 1 create action, got %+v", actions)
		}
		if actions[0].Type != ActionCreate || actions[0].NodeType != "endpoint" {
			t.Fatalf("want create action carrying node type %q, got %+v", "endpoint", actions[0])
		}
	})

	t.Run("s5_verdicts_unaffected_by_endpoint_declaration", func(t *testing.T) {
		// The custom profile still declares component/data_flow/test_section,
		// so S5's admitted set holds alongside the new endpoint entry.
		if !gateAdmits(change(f.CompX, "plan", "component", merkle.Added, "", "h"), withEndpoint) {
			t.Errorf("want component still admitted")
		}
		if gateAdmits(change("api-1", "plan", "api", merkle.Added, "", "h"), withEndpoint) {
			t.Errorf("want api still dropped — the custom profile never declared it plan-relevant")
		}
	})
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
		a.SpecNodeID != f.CompX || a.SpecHash != "newhash" || a.TaskID != "" {
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

	// S1: SCHK_HASH — the pairing's task is absent from the artifact: one
	// plain create, no close against the predecessor, no old task id on the
	// action, no lineage of any kind.
	t.Run("absent_from_artifact_yields_plain_create", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-001"}}} // BeadStatus unset: absent
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("want 1 action, got %+v", actions)
		}
		a := actions[0]
		if a.Type != ActionCreate || a.TaskID != "" || a.SpecHash != "new" {
			t.Fatalf("want a plain create with no prior task id, got %+v", a)
		}
		if a.Reason != "Spec node modified (new): plan/CompX" {
			t.Errorf("reason: got %q", a.Reason)
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
		if a.TaskID != "spex-003" || a.SpecHash != "new" {
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

	// The artifact carries no fourth status: any live-status value other than
	// "open" or "in_progress" — a tracker status the task-state schema does
	// not admit, surfaced only via the legacy --beads path — is read the
	// same as "absent from the artifact", never as some other case.
	t.Run("non_open_non_in_progress_status_also_yields_plain_create", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-099", BeadStatus: "closed"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(actions) != 1 || actions[0].Type != ActionCreate {
			t.Fatalf("want a single plain create for a non-open/in_progress status, got %+v", actions)
		}
	})
}

// TestClassifyMatched_S1b_AbsentCreateMatchesFreshCreate pins S1b: the create
// a matched-but-absent node yields (SCHK_HASH in S1) is the same create a
// never-tracked node yields, field for field, except the reason string. Both
// sides use the same identity hash and new content hash so every other field
// — Module, Node, NodeType, SpecNodeID, SpecHash, TaskID and
// DepSpecNodeIDs — lines up exactly; only Reason is blanked before the
// comparison.
func TestClassifyMatched_S1b_AbsentCreateMatchesFreshCreate(t *testing.T) {
	f := newClassifierFixture()

	modified := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	m := Match{Change: modified, Records: []Pairing{{TaskID: "spex-001"}}} // BeadStatus unset: absent
	matchedActions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("matched: err: %v", err)
	}
	if len(matchedActions) != 1 {
		t.Fatalf("matched: want 1 action, got %+v", matchedActions)
	}

	added := change(f.CompX, "plan", "component", merkle.Added, "", "new")
	u := Unmatched{Change: added}
	unmatchedActions, err := ClassifyActions(nil, []Unmatched{u}, nil, f.Graph)
	if err != nil {
		t.Fatalf("unmatched: err: %v", err)
	}
	if len(unmatchedActions) != 1 {
		t.Fatalf("unmatched: want 1 action, got %+v", unmatchedActions)
	}

	matchedAction, unmatchedAction := matchedActions[0], unmatchedActions[0]

	if matchedAction.Reason != "Spec node modified (new): plan/CompX" {
		t.Errorf("matched reason: got %q", matchedAction.Reason)
	}
	if unmatchedAction.Reason != "New spec node: plan/CompX" {
		t.Errorf("unmatched reason: got %q", unmatchedAction.Reason)
	}

	matchedAction.Reason, unmatchedAction.Reason = "", ""
	if !reflect.DeepEqual(matchedAction, unmatchedAction) {
		t.Errorf("matched-absent create must equal a fresh create in every field but Reason:\nmatched=%+v\nunmatched=%+v", matchedAction, unmatchedAction)
	}
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

	absentM := Match{Change: c, Records: []Pairing{{TaskID: "spex-y"}}} // BeadStatus unset: absent
	if actions, err := ClassifyActions([]Match{absentM}, nil, nil, f.Graph); err != nil || len(actions) != 1 || actions[0].Type != ActionCreate {
		t.Errorf("absent pairing: want a single plain create, got actions=%+v err=%v", actions, err)
	}

	inProgressM := Match{Change: c, Records: []Pairing{{TaskID: "spex-z", BeadStatus: "in_progress"}}}
	if _, err := ClassifyActions([]Match{inProgressM}, nil, nil, f.Graph); err == nil {
		t.Errorf("in_progress pairing: want refusal")
	}
}

// --- test_section fold-back (E4): precedes the status split entirely,
// and reads the split the way a removal does: open -> close, in_progress ->
// refuse, absent -> nothing owed ---

func TestFoldback_BeatsStatusSplit(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.TSOne, "plan", "test_section", merkle.Modified, "old", "new")

	t.Run("open", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-ts", BeadStatus: "open"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 || actions[0].Type != ActionObsolete {
			t.Fatalf("want exactly 1 close action, got %+v", actions)
		}
		if actions[0].Reason != "Spec node modified: plan/TSOne" {
			t.Errorf("reason: got %q", actions[0].Reason)
		}
	})

	t.Run("in_progress", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-ts", BeadStatus: "in_progress"}}}
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err == nil {
			t.Fatalf("want refusal error, got actions=%+v", actions)
		}
		if !strings.Contains(err.Error(), "spex-ts") {
			t.Errorf("error must name the claimed task: %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("no action list is returned at all on refusal, got %+v", actions)
		}
	})

	t.Run("absent", func(t *testing.T) {
		m := Match{Change: c, Records: []Pairing{{TaskID: "spex-ts"}}} // BeadStatus unset: absent
		actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 0 {
			t.Fatalf("want no action at all — the section's task is finished and owes nothing further, got %+v", actions)
		}
	})
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

// TestClassifyOrphaned_InProgressRefusesTheRun pins S4: a claimed task
// under a removed node is the largest move a target can make, so the run
// refuses naming it rather than silently closing — no action list at all.
func TestClassifyOrphaned_InProgressRefusesTheRun(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-011", Module: "plan", Name: "Legacy", BeadStatus: "in_progress"},
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err == nil {
		t.Fatalf("want refusal error, got actions=%+v", actions)
	}
	if !strings.Contains(err.Error(), "spex-011") {
		t.Errorf("error must name the claimed task: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("no action list is returned at all on refusal, got %+v", actions)
	}
}

// TestClassifyOrphaned_AbsentYieldsCleanupCreateOnly pins S4's absent case:
// the task shipped, so a cleanup create is minted to delete the code, with
// no close beside it — nothing is live to close — and no old task id or
// dependency on the finished task.
func TestClassifyOrphaned_AbsentYieldsCleanupCreateOnly(t *testing.T) {
	o := Orphaned{
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-010", Module: "plan", Name: "Legacy"}, // BeadStatus unset: absent
		NodeType: "component",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want exactly 1 cleanup create action, no close, got %+v", actions)
	}
	cleanup := actions[0]
	if cleanup.Type != ActionCreate || cleanup.TaskID != "" || cleanup.SpecHash != "" {
		t.Errorf("cleanup create: %+v", cleanup)
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

func TestDepsFor_DataFlowCollectsNothing(t *testing.T) {
	f := newClassifierFixture()
	flowChange := change(f.FlowF, "plan", "data_flow", merkle.Added, "", "h")
	if deps := depsFor(flowChange, f.Graph); len(deps) != 0 {
		t.Errorf("data_flow must collect no uses/requires_module deps, got %v", deps)
	}
}

// --- Test_section describes edges (D10) ---

func TestDeps_TestSectionDescribesCollectsEachDescribedComponent(t *testing.T) {
	f := newClassifierFixture()
	tsChange := change(f.TSMany, "plan", "test_section", merkle.Added, "", "h")
	deps := depsFor(tsChange, f.Graph)
	want := sortedStrings(f.CompX, f.CompY)
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("D10: got %v want %v", deps, want)
	}
}

func TestDeps_TestSectionDescribes_HoldsForRetargetToo(t *testing.T) {
	f := newClassifierFixture()
	c := change(f.TSMany, "plan", "test_section", merkle.Modified, "old", "new")
	m := Match{Change: c, Records: []Pairing{{TaskID: "spex-ts", BeadStatus: "open", After: "old"}}}
	actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != ActionRetarget {
		t.Fatalf("want 1 retarget, got %+v", actions)
	}
	want := sortedStrings(f.CompX, f.CompY)
	if !reflect.DeepEqual(actions[0].DepSpecNodeIDs, want) {
		t.Fatalf("D10 on retarget: got %v want %v", actions[0].DepSpecNodeIDs, want)
	}
}

func TestDeps_TestSectionDescribesCollectionIgnoresBeadStatus(t *testing.T) {
	// D2's division of labour holds for describes too: a described
	// component's own bead status must not filter it out of collection —
	// that is the Resolver's job three steps later.
	f := newClassifierFixture()
	yMatch := Match{
		Change:  change(f.CompY, "plan", "component", merkle.Modified, "old", "new"),
		Records: []Pairing{{TaskID: "spex-y", BeadStatus: "closed"}},
	}
	unmatched := Unmatched{Change: change(f.TSMany, "plan", "test_section", merkle.Added, "", "th")}
	actions, err := ClassifyActions([]Match{yMatch}, []Unmatched{unmatched}, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var tsAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == f.TSMany {
			tsAction = &actions[i]
		}
	}
	if tsAction == nil {
		t.Fatalf("TSMany action missing: %+v", actions)
	}
	want := sortedStrings(f.CompX, f.CompY)
	if !reflect.DeepEqual(tsAction.DepSpecNodeIDs, want) {
		t.Errorf("got %v want %v — CompY's closed status must not filter collection", tsAction.DepSpecNodeIDs, want)
	}
}

func TestDeps_TestSectionUnresolvableYieldsEmpty(t *testing.T) {
	f := newClassifierFixture()
	tests := []struct {
		name   string
		change merkle.ClassifiedChange
	}{
		{"unknown module", change("whatever", "no-such-module", "test_section", merkle.Added, "", "h")},
		{"unknown section in known module", change("no-such-hash", "plan", "test_section", merkle.Added, "", "h")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if deps := depsFor(tt.change, f.Graph); len(deps) != 0 {
				t.Errorf("want 0 deps, got %v", deps)
			}
		})
	}
}

func TestObsoleteActions_NeverCarryDeps(t *testing.T) {
	f := newClassifierFixture()
	// TSOne folds back with an open pairing, yielding a close — the only
	// shape this test needs. CompX rides along, open, so its retarget's
	// real DepSpecNodeIDs (CompY directly, plus CompZ and CompS
	// transitively via requires_module) are in the same list the close is
	// checked against, proving the close stays deps-free rather than
	// merely never having had any to carry.
	foldback := change(f.TSOne, "plan", "test_section", merkle.Modified, "old", "new")
	retarget := change(f.CompX, "plan", "component", merkle.Modified, "old", "new")
	matches := []Match{
		{Change: foldback, Records: []Pairing{{TaskID: "spex-ts", BeadStatus: "open"}}},
		{Change: retarget, Records: []Pairing{{TaskID: "spex-cx", BeadStatus: "open", After: "old"}}},
	}
	actions, err := ClassifyActions(matches, nil, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	found := false
	for _, a := range actions {
		if a.Type == ActionObsolete {
			found = true
			if len(a.DepSpecNodeIDs) != 0 {
				t.Errorf("obsolete action must carry no deps: %+v", a)
			}
		}
	}
	if !found {
		t.Fatalf("want an obsolete action in the list, got %+v", actions)
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
	// matched with a pairing absent from the artifact in the same run, so
	// Y yields one plain create — but that must not filter Y out of X's
	// DepSpecNodeIDs: filtering already-satisfied deps belongs to the
	// Resolver, not the classifier.
	unmatched := Unmatched{Change: change(f.CompX, "plan", "component", merkle.Added, "", "cx-hash")}
	yMatch := Match{
		Change:  change(f.CompY, "plan", "component", merkle.Modified, "old", "new"),
		Records: []Pairing{{TaskID: "spex-y"}}, // BeadStatus unset: absent
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
		t.Errorf("D2: dependency collection must ignore Y's absent bead status, got deps=%v", xAction.DepSpecNodeIDs)
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

// TestDataFlowCreate_CarriesNoSpecGraphDeps pins D8's data_flow half: a
// data_flow create walks neither uses nor requires_module and collects no
// describes (it has none) — its ordering inside the batch is driven by the
// add-on applied to the components on the other side, not by deps of its
// own.
func TestDataFlowCreate_CarriesNoSpecGraphDeps(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.FlowF, "plan", "data_flow", merkle.Added, "", "fh")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %+v", actions)
	}
	if len(actions[0].DepSpecNodeIDs) != 0 {
		t.Errorf("data_flow must carry no deps from classification, got %v", actions[0].DepSpecNodeIDs)
	}
}

// TestTestSectionCreate_WalksDescribesButNotUsesOrRequiresModule pins D8's
// test_section half: a test_section create is not dep-free — it collects
// its describes array (D10) — but it walks no uses and no requires_module,
// so a test_section id that happened to collide with a component's uses
// edge (it never does here; the fixture's TSMany has no uses of its own)
// still yields exactly the describes set.
func TestTestSectionCreate_WalksDescribesButNotUsesOrRequiresModule(t *testing.T) {
	f := newClassifierFixture()
	u := []Unmatched{
		{Change: change(f.TSMany, "plan", "test_section", merkle.Added, "", "th")},
	}
	actions, err := ClassifyActions(nil, u, nil, f.Graph)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %+v", actions)
	}
	want := sortedStrings(f.CompX, f.CompY)
	if !reflect.DeepEqual(actions[0].DepSpecNodeIDs, want) {
		t.Fatalf("got %v want %v", actions[0].DepSpecNodeIDs, want)
	}
}

// --- Names resolved from the graph; node types carried through (S8b) ---

// TestClassifyMatched_NamesResolvedFromGraph_AllNodeKinds pins S8b's first
// bullet over all three node kinds the matched path can carry — a
// component-only fixture would pass even if the resolver only ever handled
// components. Each subtest classifies a pairing whose task is absent from
// the artifact (a plain create) so the resulting action's Node and NodeType
// are checked directly.
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
			m := Match{Change: c, Records: []Pairing{{TaskID: "spex-100"}}} // BeadStatus unset: absent
			actions, err := ClassifyActions([]Match{m}, nil, nil, f.Graph)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(actions) != 1 {
				t.Fatalf("want 1 plain create, got %+v", actions)
			}
			a := actions[0]
			if a.Node != tt.wantName {
				t.Errorf("%s action: Node = %q, want declared name %q, not the identity hash", a.Type, a.Node, tt.wantName)
			}
			if a.NodeType != tt.nodeType {
				t.Errorf("%s action: NodeType = %q, want %q", a.Type, a.NodeType, tt.nodeType)
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
		Record:   Pairing{SpecNodeID: "legacy1", TaskID: "spex-010", Module: "plan", Name: "Legacy"}, // BeadStatus unset: absent
		NodeType: "data_flow",
	}
	actions, err := ClassifyActions(nil, nil, []Orphaned{o}, SpecGraph{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 cleanup create, got %+v", actions)
	}
	if actions[0].NodeType != "data_flow" {
		t.Errorf("%s action: NodeType = %q, want %q carried from the pairing record", actions[0].Type, actions[0].NodeType, "data_flow")
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
		{Record: Pairing{SpecNodeID: "legacy", TaskID: "spex-010", Module: "plan", Name: "Legacy", BeadStatus: "open"}, NodeType: "component"},
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
