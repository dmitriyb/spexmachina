package merkle

import (
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

func TestREQ5_Classify_ImplOnly(t *testing.T) {
	changes := []Change{
		{Key: "module/1/impl_section/1", Type: Modified, NodeType: "impl_section", Module: "mod1"},
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
		{Key: "module/1/impl_section/1", Type: Modified, NodeType: "impl_section", Module: "mod1", OldHash: "aaa", NewHash: "bbb"},
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
		{Key: "module/1/impl_section/1", Type: Modified, NodeType: "impl_section", Module: "mod1"},
		{Key: "module/2/data_flow/1", Type: Added, NodeType: "data_flow", Module: "mod2"},
	}
	names := map[string]string{"mod1": "Alpha", "mod2": "Beta"}

	classified := Classify(changes, names)

	if len(classified) != 4 {
		t.Fatalf("expected 4 classified changes, got %d", len(classified))
	}

	expected := []struct {
		impact ImpactLevel
		module string
	}{
		{Structural, ""},
		{ArchImpl, "Alpha"},
		{ImplOnly, "Alpha"},
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

	// Modify a component file and an impl file
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Modified arch\n")
	writeFile(t, filepath.Join(specDir, "alpha"), "impl_comp1.md", "# Modified impl\n")
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
	impl1Key := schema.IdentityHash("alpha", "impl_section", "Impl1")

	// Should have at least one arch_impl and one impl_only
	var hasArch, hasImpl bool
	for _, c := range classified {
		if c.Impact == ArchImpl && c.Key == comp1Key {
			hasArch = true
			if c.Module != "Alpha" {
				t.Errorf("expected module Alpha for component change, got %q", c.Module)
			}
		}
		if c.Impact == ImplOnly && c.Key == impl1Key {
			hasImpl = true
		}
	}
	if !hasArch {
		t.Errorf("expected arch_impl change for %s", comp1Key)
	}
	if !hasImpl {
		t.Errorf("expected impl_only change for %s", impl1Key)
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
