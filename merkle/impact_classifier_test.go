package merkle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// Placeholder identity hashes for the Classify scenarios in
// test_diff_classification.md, computed the same way setupSpecDir computes
// its fixture hashes. These scenarios pass synthetic []Change literals
// straight into Classify and build no tree — that's why FLOW1_HASH (a
// data_flow) is reachable here even though setupSpecDir declares no
// data_flow.
var (
	classifyAlphaHash = schema.IdentityHash("module", "Alpha")
	classifyBetaHash  = schema.IdentityHash("module", "Beta")
	classifyTest1Hash = schema.IdentityHash("alpha", "test_section", "Test1")
	classifyFlow1Hash = schema.IdentityHash("alpha", "data_flow", "Flow1")
	classifyComp1Hash = schema.IdentityHash("alpha", "component", "Comp1")
	classifyComp2Hash = schema.IdentityHash("beta", "component", "Comp2")
	classifyReq2Hash  = schema.IdentityHash("alpha", "requirement", "Alpha req 2")
	classifyProjReq1  = schema.IdentityHash("project", "requirement", "Proj req 1")
)

// TestREQ5_S7_ImplOnly covers test_diff_classification.md S7: a test_section
// change classifies impl_only, and its module hash resolves to a name.
func TestREQ5_S7_ImplOnly(t *testing.T) {
	changes := []Change{
		{Key: classifyTest1Hash, Type: Modified, NodeType: "test_section", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != ImplOnly {
		t.Errorf("expected impl_only, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

// TestREQ5_S8_DataFlowIsContract covers S8: data_flow is a contract-level
// surface, not impl_only.
func TestREQ5_S8_DataFlowIsContract(t *testing.T) {
	changes := []Change{
		{Key: classifyFlow1Hash, Type: Modified, NodeType: "data_flow", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if classified[0].Impact != Contract {
		t.Errorf("expected contract for data_flow, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

// TestREQ5_S9_ArchImpl covers S9: a component change classifies arch_impl.
func TestREQ5_S9_ArchImpl(t *testing.T) {
	changes := []Change{
		{Key: classifyComp1Hash, Type: Modified, NodeType: "component", Module: classifyBetaHash},
	}
	names := map[string]string{classifyBetaHash: "Beta"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != ArchImpl {
		t.Errorf("expected arch_impl, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Beta" {
		t.Errorf("expected module Beta, got %q", classified[0].Module)
	}
}

// TestREQ5_S10_StructuralModuleMeta covers S10: a modified module envelope
// (module.json) classifies structural.
func TestREQ5_S10_StructuralModuleMeta(t *testing.T) {
	changes := []Change{
		{Key: "meta/" + classifyAlphaHash, Type: Modified, NodeType: "meta", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if classified[0].Impact != Structural {
		t.Errorf("expected structural for module meta, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

// TestREQ5_S11_StructuralProjectMeta covers S11: a modified project
// envelope (project.json) classifies structural with an empty module.
func TestREQ5_S11_StructuralProjectMeta(t *testing.T) {
	changes := []Change{
		{Key: "meta/project", Type: Modified, NodeType: "meta", Module: ""},
	}

	classified := Classify(changes, nil, schema.DefaultProfile())

	if classified[0].Impact != Structural {
		t.Errorf("expected structural for project meta, got %s", classified[0].Impact)
	}
	if classified[0].Module != "" {
		t.Errorf("expected empty module for project meta, got %q", classified[0].Module)
	}
}

// TestREQ5_S12_MixedImpactsNoAggregation covers S12: each change in a module
// keeps its own level — the classifier does not aggregate per module.
func TestREQ5_S12_MixedImpactsNoAggregation(t *testing.T) {
	changes := []Change{
		{Key: classifyTest1Hash, Type: Modified, NodeType: "test_section", Module: classifyAlphaHash},
		{Key: classifyComp2Hash, Type: Modified, NodeType: "component", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if len(classified) != 2 {
		t.Fatalf("expected 2 classified changes, got %d", len(classified))
	}
	for _, c := range classified {
		if c.Module != "Alpha" {
			t.Errorf("expected module Alpha, got %q", c.Module)
		}
	}
	if classified[0].Impact != ImplOnly {
		t.Errorf("expected impl_only for test_section change, got %s", classified[0].Impact)
	}
	if classified[1].Impact != ArchImpl {
		t.Errorf("expected arch_impl for component change, got %s", classified[1].Impact)
	}
}

// TestREQ5_S13_StructuralAlongsideLowerImpacts covers S13: a per-module
// aggregate, if a caller computes one locally, is the max of the per-change
// levels — the classifier itself still reports one level per change.
func TestREQ5_S13_StructuralAlongsideLowerImpacts(t *testing.T) {
	changes := []Change{
		{Key: classifyTest1Hash, Type: Modified, NodeType: "test_section", Module: classifyAlphaHash},
		{Key: classifyComp2Hash, Type: Modified, NodeType: "component", Module: classifyAlphaHash},
		{Key: "meta/" + classifyAlphaHash, Type: Modified, NodeType: "meta", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	want := []ImpactLevel{ImplOnly, ArchImpl, Structural}
	if len(classified) != len(want) {
		t.Fatalf("expected %d classified changes, got %d", len(want), len(classified))
	}
	max := Unknown
	for i, c := range classified {
		if c.Impact != want[i] {
			t.Errorf("[%d] impact: want %s, got %s", i, want[i], c.Impact)
		}
		if c.Impact > max {
			max = c.Impact
		}
	}
	if max != Structural {
		t.Errorf("locally computed per-module max: want structural, got %s", max)
	}
}

// TestREQ5_S14_DiffThenClassifyEndToEnd covers S14, the one Classify
// scenario that builds real trees: DiffEngine produces raw changes carrying
// node metadata, and ImpactClassifier annotates them from that metadata —
// the exact data path flow_diff_classification.md describes. All three S14
// Then clauses are exercised: alpha's test_section change (impl_only),
// beta's component change (arch_impl), and a new module gamma's envelope
// change (structural).
func TestREQ5_S14_DiffThenClassifyEndToEnd(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// alpha's test_section leaf changes, beta's component leaf changes.
	writeFile(t, filepath.Join(specDir, "alpha"), "test_comp1.md", "# Modified tests\n")
	writeFile(t, filepath.Join(specDir, "beta"), "arch_beta.md", "# Modified beta arch\n")

	// A new module gamma appears, adding its meta/<GAMMA_HASH> envelope leaf.
	gammaModID := schema.IdentityHash("module", "Gamma")
	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": "` + fixtureProjReq1ID + `", "type": "functional", "title": "Do stuff", "description": "The system must do stuff.", "priority": 1},
			{"id": "` + fixtureProjReq2ID + `", "type": "non_functional", "title": "Be fast", "priority": 2}
		],
		"modules": [
			{"id": "` + classifyAlphaHash + `", "name": "Alpha", "path": "alpha"},
			{"id": "` + classifyBetaHash + `", "name": "Beta", "path": "beta"},
			{"id": "` + gammaModID + `", "name": "Gamma", "path": "gamma"}
		]
	}`
	writeFile(t, specDir, "project.json", proj)
	gammaDir := filepath.Join(specDir, "gamma")
	must(t, os.MkdirAll(gammaDir, 0755))
	writeFile(t, gammaDir, "module.json", `{"name": "gamma"}`)

	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)
	classified := Classify(changes, ModuleNames(current), schema.DefaultProfile())

	test1Key := schema.IdentityHash("alpha", "test_section", "Test1")
	betaCompKey := schema.IdentityHash("beta", "component", "BetaComp")
	gammaMetaKey := "meta/" + gammaModID

	var sawImplOnly, sawArchImpl, sawGammaStructural bool
	for _, c := range classified {
		if c.Key == test1Key {
			sawImplOnly = c.Impact == ImplOnly
		}
		if c.Key == betaCompKey {
			sawArchImpl = c.Impact == ArchImpl
		}
		if c.Key == gammaMetaKey {
			sawGammaStructural = c.Impact == Structural
		}
	}
	if !sawImplOnly {
		t.Errorf("expected impl_only change for %s", test1Key)
	}
	if !sawArchImpl {
		t.Errorf("expected arch_impl change for %s", betaCompKey)
	}
	if !sawGammaStructural {
		t.Errorf("expected structural change for gamma envelope %s", gammaMetaKey)
	}
}

// TestREQ5_S15_ImpactLevelsFollowProfile covers S15: the mapping from
// declared node type to impact level is read from whatever profile is
// handed in — a resolved profile that maps a project-declared "endpoint"
// type to contract classifies an endpoint change contract, while the four
// levels themselves and the fixed meta rule stay put.
func TestREQ5_S15_ImpactLevelsFollowProfile(t *testing.T) {
	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name: "endpoint", PluralKey: "endpoints", Scope: "module", RequiresContent: true,
	})
	profile.ImpactLevels["endpoint"] = "contract"

	changes := []Change{
		{Key: schema.IdentityHash("api", "endpoint", "GET /v1/widgets"), Type: Modified, NodeType: "endpoint", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, profile)

	if classified[0].Impact != Contract {
		t.Errorf("expected contract for profile-declared endpoint type, got %s", classified[0].Impact)
	}
}

// TestREQ5_S16_UndeclaredNodeTypeIsUnknown covers S16: a node_type the
// resolved profile does not declare (and that is not "meta") classifies
// unknown rather than failing. This is reachable because a removed leaf
// takes its node type from the snapshot side verbatim.
func TestREQ5_S16_UndeclaredNodeTypeIsUnknown(t *testing.T) {
	changes := []Change{
		{Key: classifyFlow1Hash, Type: Removed, NodeType: "endpoint", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != Unknown {
		t.Errorf("expected unknown for undeclared node type, got %s", classified[0].Impact)
	}
	if classified[0].Impact.String() != "unknown" {
		t.Errorf("expected Impact.String() == \"unknown\", got %q", classified[0].Impact.String())
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

// TestREQ5_R1_RequirementIsStructural covers R1: a module requirement
// change classifies structural.
func TestREQ5_R1_RequirementIsStructural(t *testing.T) {
	changes := []Change{
		{Key: classifyReq2Hash, Type: Modified, NodeType: "requirement", Module: classifyAlphaHash},
	}
	names := map[string]string{classifyAlphaHash: "Alpha"}

	classified := Classify(changes, names, schema.DefaultProfile())

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != Structural {
		t.Errorf("expected structural for requirement, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

// TestREQ5_R2_ProjectRequirementIsStructural covers R2: a project-level
// requirement change classifies structural with an empty module.
func TestREQ5_R2_ProjectRequirementIsStructural(t *testing.T) {
	changes := []Change{
		{Key: classifyProjReq1, Type: Modified, NodeType: "requirement", Module: ""},
	}

	classified := Classify(changes, nil, schema.DefaultProfile())

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != Structural {
		t.Errorf("expected structural for project requirement, got %s", classified[0].Impact)
	}
	if classified[0].Module != "" {
		t.Errorf("expected empty module for project requirement, got %q", classified[0].Module)
	}
}

// TestREQ5_Classify_EmptyChanges covers E1: Classify on an empty or nil
// slice returns an empty slice and never panics or injects synthetic
// entries. Named per test_diff_classification.md line 184, which cites this
// exact test name for E1's Classify(nil, ...) arm.
func TestREQ5_Classify_EmptyChanges(t *testing.T) {
	classified := Classify([]Change{}, nil, schema.DefaultProfile())
	if len(classified) != 0 {
		t.Fatalf("expected 0 classified changes for empty input, got %d", len(classified))
	}

	classified = Classify(nil, nil, schema.DefaultProfile())
	if len(classified) != 0 {
		t.Fatalf("expected 0 classified changes for nil input, got %d", len(classified))
	}
}

func TestREQ5_ImpactLevel_String(t *testing.T) {
	tests := []struct {
		level ImpactLevel
		want  string
	}{
		{ImplOnly, "impl_only"},
		{Contract, "contract"},
		{ArchImpl, "arch_impl"},
		{Structural, "structural"},
		{Unknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("ImpactLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

// TestREQ5_Classify_PreservesChangeFields is a unit test (not a named
// scenario): every field the DiffEngine attached survives Classify
// untouched, aside from Module's hash-to-name substitution.
func TestREQ5_Classify_PreservesChangeFields(t *testing.T) {
	changes := []Change{
		{Key: classifyTest1Hash, Type: Modified, NodeType: "test_section", Module: classifyAlphaHash, OldHash: "aaa", NewHash: "bbb"},
	}

	classified := Classify(changes, nil, schema.DefaultProfile())

	c := classified[0]
	if c.Key != changes[0].Key {
		t.Errorf("key mismatch: %s vs %s", c.Key, changes[0].Key)
	}
	if c.Type != Modified {
		t.Errorf("type mismatch: %v", c.Type)
	}
	if c.NodeType != "test_section" {
		t.Errorf("node_type mismatch: %v", c.NodeType)
	}
	if c.OldHash != "aaa" || c.NewHash != "bbb" {
		t.Errorf("hash mismatch: old=%s new=%s", c.OldHash, c.NewHash)
	}
}

// TestREQ5_Classify_NilModuleNames_FallsBackToHash is a unit test for the
// arch doc's stated behavior: "a hash the name map does not cover is passed
// through as the hash".
func TestREQ5_Classify_NilModuleNames_FallsBackToHash(t *testing.T) {
	changes := []Change{
		{Key: classifyComp1Hash, Type: Modified, NodeType: "component", Module: classifyAlphaHash},
	}

	classified := Classify(changes, nil, schema.DefaultProfile())

	if classified[0].Module != classifyAlphaHash {
		t.Errorf("expected module hash fallback %q, got %q", classifyAlphaHash, classified[0].Module)
	}
}
