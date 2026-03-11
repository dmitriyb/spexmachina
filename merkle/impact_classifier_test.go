package merkle

import (
	"path/filepath"
	"testing"
)

func TestREQ5_Classify_ImplOnly(t *testing.T) {
	changes := []Change{
		{Path: "module/1/impl_section/1", Type: Modified},
	}
	names := map[int]string{1: "Alpha"}

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

func TestREQ5_Classify_DataFlowIsImplOnly(t *testing.T) {
	changes := []Change{
		{Path: "module/1/data_flow/1", Type: Added},
	}

	classified := Classify(changes, nil)

	if classified[0].Impact != ImplOnly {
		t.Errorf("expected impl_only for data_flow, got %s", classified[0].Impact)
	}
}

func TestREQ5_Classify_ArchImpl(t *testing.T) {
	changes := []Change{
		{Path: "module/1/component/1", Type: Modified},
	}
	names := map[int]string{1: "Alpha"}

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
		{Path: "module/1/meta", Type: Modified},
	}
	names := map[int]string{1: "Alpha"}

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
		{Path: "project/meta", Type: Modified},
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
		{Path: "module/1/impl_section/1", Type: Modified, OldHash: "aaa", NewHash: "bbb"},
	}

	classified := Classify(changes, nil)

	c := classified[0]
	if c.Path != changes[0].Path {
		t.Errorf("path mismatch: %s vs %s", c.Path, changes[0].Path)
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
		{Path: "project/meta", Type: Modified},
		{Path: "module/1/component/1", Type: Modified},
		{Path: "module/1/impl_section/1", Type: Modified},
		{Path: "module/2/data_flow/1", Type: Added},
	}
	names := map[int]string{1: "Alpha", 2: "Beta"}

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
		{ImplOnly, "Beta"},
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

	// Should have at least one arch_impl and one impl_only
	var hasArch, hasImpl bool
	for _, c := range classified {
		if c.Impact == ArchImpl && c.Path == "module/1/component/1" {
			hasArch = true
			if c.Module != "Alpha" {
				t.Errorf("expected module Alpha for component change, got %q", c.Module)
			}
		}
		if c.Impact == ImplOnly && c.Path == "module/1/impl_section/1" {
			hasImpl = true
		}
	}
	if !hasArch {
		t.Error("expected arch_impl change for module/1/component/1")
	}
	if !hasImpl {
		t.Error("expected impl_only change for module/1/impl_section/1")
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

func TestREQ5_Classify_NilModuleNames_FallsBackToID(t *testing.T) {
	changes := []Change{
		{Path: "module/3/component/1", Type: Modified},
	}

	classified := Classify(changes, nil)

	if classified[0].Module != "3" {
		t.Errorf("expected module ID fallback '3', got %q", classified[0].Module)
	}
}
