package merkle

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

func TestREQ5_Classify_ImplOnly(t *testing.T) {
	changes := []Change{
		{Key: "module/1/test_section/1", Type: Modified, NodeType: "test_section", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

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

func TestREQ5_Classify_DataFlowIsContract(t *testing.T) {
	changes := []Change{
		{Key: "module/1/data_flow/1", Type: Added, NodeType: "data_flow", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

	if classified[0].Impact != Contract {
		t.Errorf("expected contract for data_flow, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

func TestREQ5_Classify_APIIsContract(t *testing.T) {
	changes := []Change{
		{Key: "module/1/api/1", Type: Added, NodeType: "api", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

	if classified[0].Impact != Contract {
		t.Errorf("expected contract for api, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

func TestREQ5_Classify_TestSectionIsImplOnly(t *testing.T) {
	changes := []Change{
		{Key: "module/1/test_section/1", Type: Modified, NodeType: "test_section", Module: "mod1"},
	}

	classified := Classify(changes, nil)

	if classified[0].Impact != ImplOnly {
		t.Errorf("expected impl_only for test_section, got %s", classified[0].Impact)
	}
}

func TestREQ5_Classify_ArchImpl(t *testing.T) {
	changes := []Change{
		{Key: "module/1/component/1", Type: Modified, NodeType: "component", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != ArchImpl {
		t.Errorf("expected arch_impl, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

func TestREQ5_Classify_StructuralModuleMeta(t *testing.T) {
	changes := []Change{
		{Key: "module/1/meta", Type: Modified, NodeType: "meta", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

	if classified[0].Impact != Structural {
		t.Errorf("expected structural for module meta, got %s", classified[0].Impact)
	}
	if classified[0].Module != "Alpha" {
		t.Errorf("expected module Alpha, got %q", classified[0].Module)
	}
}

func TestREQ5_Classify_StructuralProjectMeta(t *testing.T) {
	changes := []Change{
		{Key: "project/meta", Type: Modified, NodeType: "meta", Module: ""},
	}

	classified := Classify(changes, nil)

	if classified[0].Impact != Structural {
		t.Errorf("expected structural for project/meta, got %s", classified[0].Impact)
	}
	if classified[0].Module != "" {
		t.Errorf("expected empty module for project/meta, got %q", classified[0].Module)
	}
}

func TestREQ5_Classify_PreservesChangeFields(t *testing.T) {
	changes := []Change{
		{Key: "module/1/test_section/1", Type: Modified, NodeType: "test_section", Module: "mod1", OldHash: "aaa", NewHash: "bbb"},
	}

	classified := Classify(changes, nil)

	c := classified[0]
	if c.Key != changes[0].Key {
		t.Errorf("path mismatch: %s vs %s", c.Key, changes[0].Key)
	}
	if c.Type != Modified {
		t.Errorf("type mismatch: %v", c.Type)
	}
	if c.OldHash != "aaa" || c.NewHash != "bbb" {
		t.Errorf("hash mismatch: old=%s new=%s", c.OldHash, c.NewHash)
	}
}

func TestREQ5_Classify_MultipleChanges(t *testing.T) {
	changes := []Change{
		{Key: "project/meta", Type: Modified, NodeType: "meta", Module: ""},
		{Key: "module/1/component/1", Type: Modified, NodeType: "component", Module: "mod1"},
		{Key: "module/2/data_flow/1", Type: Added, NodeType: "data_flow", Module: "mod2"},
	}
	names := map[string]string{"mod1": "Alpha", "mod2": "Beta"}

	classified := Classify(changes, names)

	if len(classified) != 3 {
		t.Fatalf("expected 3 classified changes, got %d", len(classified))
	}

	expected := []struct {
		impact ImpactLevel
		module string
	}{
		{Structural, ""},
		{ArchImpl, "Alpha"},
		{Contract, "Beta"},
	}

	for i, want := range expected {
		if classified[i].Impact != want.impact {
			t.Errorf("[%d] impact: want %s, got %s", i, want.impact, classified[i].Impact)
		}
		if classified[i].Module != want.module {
			t.Errorf("[%d] module: want %q, got %q", i, want.module, classified[i].Module)
		}
	}
}

func TestREQ5_Classify_Integration_WithDiff(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Modify a component file and a test file
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Modified arch\n")
	writeFile(t, filepath.Join(specDir, "alpha"), "test_comp1.md", "# Modified tests\n")
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)
	moduleNames := ModuleNames(current)
	classified := Classify(changes, moduleNames)

	if len(classified) == 0 {
		t.Fatal("expected classified changes, got none")
	}

	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")
	test1Key := schema.IdentityHash("alpha", "test_section", "Test1")

	// Should have at least one arch_impl and one impl_only
	var hasArch, hasImpl bool
	for _, c := range classified {
		if c.Impact == ArchImpl && c.Key == comp1Key {
			hasArch = true
			if c.Module != "Alpha" {
				t.Errorf("expected module Alpha for component change, got %q", c.Module)
			}
		}
		if c.Impact == ImplOnly && c.Key == test1Key {
			hasImpl = true
		}
	}
	if !hasArch {
		t.Errorf("expected arch_impl change for %s", comp1Key)
	}
	if !hasImpl {
		t.Errorf("expected impl_only change for %s", test1Key)
	}
}

func TestREQ5_Classify_EmptyChanges(t *testing.T) {
	classified := Classify(nil, nil)
	if len(classified) != 0 {
		t.Fatalf("expected 0 classified changes for nil input, got %d", len(classified))
	}

	classified = Classify([]Change{}, nil)
	if len(classified) != 0 {
		t.Fatalf("expected 0 classified changes for empty input, got %d", len(classified))
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
		{ImpactLevel(0), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("ImpactLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestREQ5_Classify_RequirementIsStructural(t *testing.T) {
	changes := []Change{
		{Key: "module/1/requirement/2", Type: Modified, NodeType: "requirement", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names)

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

func TestREQ5_Classify_ProjectRequirementIsStructural(t *testing.T) {
	changes := []Change{
		{Key: "project/requirement/1", Type: Modified, NodeType: "requirement", Module: ""},
	}

	classified := Classify(changes, nil)

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

func TestREQ5_Classify_NilModuleNames_FallsBackToID(t *testing.T) {
	changes := []Change{
		{Key: "module/3/component/1", Type: Modified, NodeType: "component", Module: "mod3"},
	}

	classified := Classify(changes, nil)

	if classified[0].Module != "mod3" {
		t.Errorf("expected module hash fallback 'mod3', got %q", classified[0].Module)
	}
}

// TestREQ5_S15_DefaultProfile_MatchesOmittedProfile covers S15's default-profile
// half: calling Classify with the default profile passed explicitly must
// produce byte-for-byte the same results as omitting the argument, since the
// default profile assigns today's types to today's levels.
func TestREQ5_S15_DefaultProfile_MatchesOmittedProfile(t *testing.T) {
	changes := []Change{
		{Key: "TEST1_HASH", Type: Modified, NodeType: "test_section", Module: "mod1"},
		{Key: "FLOW1_HASH", Type: Modified, NodeType: "data_flow", Module: "mod1"},
		{Key: "API1_HASH", Type: Modified, NodeType: "api", Module: "mod1"},
		{Key: "COMP1_HASH", Type: Modified, NodeType: "component", Module: "mod1"},
		{Key: "meta/mod1", Type: Modified, NodeType: "meta", Module: "mod1"},
		{Key: "REQ1_HASH", Type: Modified, NodeType: "requirement", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	withoutProfile := Classify(changes, names)
	withDefaultProfile := Classify(changes, names, schema.DefaultProfile())

	if len(withoutProfile) != len(withDefaultProfile) {
		t.Fatalf("length mismatch: %d vs %d", len(withoutProfile), len(withDefaultProfile))
	}
	for i := range withoutProfile {
		if withoutProfile[i] != withDefaultProfile[i] {
			t.Errorf("[%d] mismatch: omitted=%+v explicit=%+v", i, withoutProfile[i], withDefaultProfile[i])
		}
	}

	want := []ImpactLevel{ImplOnly, Contract, Contract, ArchImpl, Structural, Structural}
	for i, w := range want {
		if withoutProfile[i].Impact != w {
			t.Errorf("[%d] impact: want %s, got %s", i, w, withoutProfile[i].Impact)
		}
	}
}

// TestREQ5_S15_CustomProfile_DeclaredTypeMapsToDeclaredLevel covers S15's
// custom-profile half: a spec/profile.json declaring a novel "endpoint" type
// mapped to the contract level classifies a change carrying that node type
// as contract — the mapping is read from the resolved profile's
// ImpactLevels declaration, never from a hard-coded set of type names.
func TestREQ5_S15_CustomProfile_DeclaredTypeMapsToDeclaredLevel(t *testing.T) {
	dir := t.TempDir()

	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
	})
	profile.ImpactLevels["endpoint"] = "contract"
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	writeFile(t, dir, "profile.json", string(profileJSON))

	resolved, err := schema.ResolveProfile(dir)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	changes := []Change{
		{Key: "ENDPOINT1_HASH", Type: Added, NodeType: "endpoint", Module: "mod1"},
	}
	names := map[string]string{"mod1": "Alpha"}

	classified := Classify(changes, names, resolved)

	if len(classified) != 1 {
		t.Fatalf("expected 1 classified change, got %d", len(classified))
	}
	if classified[0].Impact != Contract {
		t.Errorf("expected contract for profile-declared endpoint type, got %s", classified[0].Impact)
	}

	// Under the default profile (no such declaration), the same node type
	// carries no level at all.
	defaultClassified := Classify(changes, names)
	if defaultClassified[0].Impact != ImpactLevel(0) {
		t.Errorf("expected unknown impact under default profile, got %s", defaultClassified[0].Impact)
	}
}

// TestREQ5_S15_MetaFixedRule_UnderCustomProfile covers S15's fixed-rule
// clause: meta's structural classification is the frame's rule, not a
// profile declaration, so it holds even under a custom profile that never
// mentions "meta" in its ImpactLevels map (the default profile doesn't
// either).
func TestREQ5_S15_MetaFixedRule_UnderCustomProfile(t *testing.T) {
	profile := schema.DefaultProfile()
	if _, declared := profile.ImpactLevels["meta"]; declared {
		t.Fatalf("test assumption violated: profile declares an impact level for meta")
	}

	changes := []Change{
		{Key: "meta/mod1", Type: Modified, NodeType: "meta", Module: "mod1"},
	}
	classified := Classify(changes, nil, profile)

	if classified[0].Impact != Structural {
		t.Errorf("expected structural for meta under custom profile, got %s", classified[0].Impact)
	}
}

// TestREQ5_Classify_UnknownNodeType covers the "Rules" clause in
// arch_impact_classifier.md: a node_type the resolved profile does not
// declare (and that is not meta) gets no level at all, reported as unknown.
func TestREQ5_Classify_UnknownNodeType(t *testing.T) {
	changes := []Change{
		{Key: "MYSTERY_HASH", Type: Modified, NodeType: "mystery", Module: "mod1"},
	}

	classified := Classify(changes, nil)

	if classified[0].Impact != ImpactLevel(0) {
		t.Errorf("expected unknown impact for undeclared node type, got %s", classified[0].Impact)
	}
	if classified[0].Impact.String() != "unknown" {
		t.Errorf("expected String() 'unknown', got %q", classified[0].Impact.String())
	}
}
