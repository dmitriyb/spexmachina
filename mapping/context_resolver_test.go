package mapping

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// writeModuleJSON writes a schema.ModuleSpec as module.json into the given directory.
func writeModuleJSON(t *testing.T, dir string, ms schema.ModuleSpec) {
	t.Helper()
	data, err := json.MarshalIndent(ms, "", "  ")
	if err != nil {
		t.Fatalf("marshal module.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.json"), data, 0644); err != nil {
		t.Fatalf("write module.json: %v", err)
	}
}

func TestFR6_ResolveContext_FullResolution(t *testing.T) {
	// Setup: create a spec directory with a module.json containing impl_sections,
	// test_sections, and data_flows that describe/use component 4.
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "map")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "map",
		Components: []schema.Component{
			{ID: 1, Name: "MappingStore", Content: "arch_mapping_store.md"},
			{ID: 4, Name: "ContextResolver", Content: "arch_context_resolver.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: 1, Name: "Mapping format", Content: "impl_mapping_format.md", Describes: []int{1}},
			{ID: 2, Name: "CRUD ops", Content: "impl_crud_operations.md", Describes: []int{1}},
		},
		TestSections: []schema.TestSection{
			{ID: 1, Name: "Store tests", Content: "test_mapping_store.md", Describes: []int{1}},
		},
		DataFlows: []schema.DataFlow{
			{ID: 1, Name: "Bead mapping flow", Content: "flow_bead_mapping.md", Uses: []int{1, 3}},
			{ID: 2, Name: "Preflight flow", Content: "flow_preflight.md", Uses: []int{1, 2}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          50,
		SpecNodeID:  "map/component/4",
		BeadID:      "spexmachina-2pu",
		Module:      "map",
		Component:   "ContextResolver",
		ContentFile: "spec/map/arch_context_resolver.md",
	}

	result, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	// ArchFile comes from the record's ContentFile.
	if result.ArchFile != "spec/map/arch_context_resolver.md" {
		t.Errorf("ArchFile = %q, want %q", result.ArchFile, "spec/map/arch_context_resolver.md")
	}

	// ModuleFile = specDir/module/module.json
	wantModFile := filepath.Join(specDir, "map", "module.json")
	if result.ModuleFile != wantModFile {
		t.Errorf("ModuleFile = %q, want %q", result.ModuleFile, wantModFile)
	}

	// Component 4 is not described by any impl_sections → empty.
	if len(result.ImplFiles) != 0 {
		t.Errorf("ImplFiles = %v, want empty (component 4 not described)", result.ImplFiles)
	}

	// Component 4 is not described by any test_sections → empty.
	if len(result.TestFiles) != 0 {
		t.Errorf("TestFiles = %v, want empty (component 4 not described)", result.TestFiles)
	}

	// Component 4 is not used by any data_flows → empty.
	if len(result.FlowFiles) != 0 {
		t.Errorf("FlowFiles = %v, want empty (component 4 not used)", result.FlowFiles)
	}

	// Record should be passed through.
	if result.Record.ID != 50 {
		t.Errorf("Record.ID = %d, want 50", result.Record.ID)
	}
}

func TestFR6_ResolveContext_MatchingSections(t *testing.T) {
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "impact")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "impact",
		Components: []schema.Component{
			{ID: 1, Name: "ActionClassifier", Content: "arch_action_classifier.md"},
			{ID: 2, Name: "ReportGenerator", Content: "arch_report_generator.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: 1, Name: "Classification rules", Content: "impl_classification.md", Describes: []int{1}},
			{ID: 2, Name: "Report format", Content: "impl_report_format.md", Describes: []int{2}},
			{ID: 3, Name: "Shared helpers", Content: "impl_shared.md", Describes: []int{1, 2}},
		},
		TestSections: []schema.TestSection{
			{ID: 1, Name: "Classifier tests", Content: "test_classifier.md", Describes: []int{1}},
			{ID: 2, Name: "Report tests", Content: "test_report.md", Describes: []int{2}},
		},
		DataFlows: []schema.DataFlow{
			{ID: 1, Name: "Impact flow", Content: "flow_impact.md", Uses: []int{1, 2}},
			{ID: 2, Name: "Other flow", Content: "flow_other.md", Uses: []int{2}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          10,
		SpecNodeID:  "impact/component/1",
		Module:      "impact",
		Component:   "ActionClassifier",
		ContentFile: "spec/impact/arch_action_classifier.md",
	}

	result, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	// impl_sections 1 and 3 describe component 1.
	wantImpl := []string{
		filepath.Join(specDir, "impact", "impl_classification.md"),
		filepath.Join(specDir, "impact", "impl_shared.md"),
	}
	if len(result.ImplFiles) != len(wantImpl) {
		t.Fatalf("ImplFiles count = %d, want %d", len(result.ImplFiles), len(wantImpl))
	}
	for i, got := range result.ImplFiles {
		if got != wantImpl[i] {
			t.Errorf("ImplFiles[%d] = %q, want %q", i, got, wantImpl[i])
		}
	}

	// test_section 1 describes component 1.
	wantTest := []string{
		filepath.Join(specDir, "impact", "test_classifier.md"),
	}
	if len(result.TestFiles) != len(wantTest) {
		t.Fatalf("TestFiles count = %d, want %d", len(result.TestFiles), len(wantTest))
	}
	if result.TestFiles[0] != wantTest[0] {
		t.Errorf("TestFiles[0] = %q, want %q", result.TestFiles[0], wantTest[0])
	}

	// data_flow 1 uses component 1.
	wantFlow := []string{
		filepath.Join(specDir, "impact", "flow_impact.md"),
	}
	if len(result.FlowFiles) != len(wantFlow) {
		t.Fatalf("FlowFiles count = %d, want %d", len(result.FlowFiles), len(wantFlow))
	}
	if result.FlowFiles[0] != wantFlow[0] {
		t.Errorf("FlowFiles[0] = %q, want %q", result.FlowFiles[0], wantFlow[0])
	}
}

func TestFR6_ResolveContext_InvalidSpecNodeID(t *testing.T) {
	specDir := t.TempDir()

	tests := []struct {
		name       string
		specNodeID string
		wantErr    string
	}{
		{"too few parts", "map/component", "invalid spec_node_id"},
		{"not a component", "map/requirement/1", "not a component node"},
		{"non-numeric ID", "map/component/abc", "invalid spec_node_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := Record{SpecNodeID: tt.specNodeID, Module: "map"}
			_, err := ResolveContext(specDir, rec)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFR6_ResolveContext_ModuleJsonNotFound(t *testing.T) {
	specDir := t.TempDir()
	// No module.json created — should fail.
	rec := Record{
		SpecNodeID: "missing/component/1",
		Module:     "missing",
	}

	_, err := ResolveContext(specDir, rec)
	if err == nil {
		t.Fatalf("expected error for missing module.json, got nil")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error = %q, want containing 'read'", err.Error())
	}
}

func TestFR6_ResolveContext_Deterministic(t *testing.T) {
	// Same inputs produce same outputs — pure function.
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "schema")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "schema",
		Components: []schema.Component{
			{ID: 1, Name: "ProjectSchema", Content: "arch_project_schema.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: 1, Name: "Schema format", Content: "impl_format.md", Describes: []int{1}},
		},
		DataFlows: []schema.DataFlow{
			{ID: 1, Name: "Load flow", Content: "flow_load.md", Uses: []int{1}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          1,
		SpecNodeID:  "schema/component/1",
		Module:      "schema",
		Component:   "ProjectSchema",
		ContentFile: "spec/schema/arch_project_schema.md",
	}

	r1, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	r2, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Compare JSON serialization for deep equality.
	j1, _ := json.Marshal(r1)
	j2, _ := json.Marshal(r2)
	if string(j1) != string(j2) {
		t.Errorf("not deterministic:\n  first:  %s\n  second: %s", j1, j2)
	}
}
