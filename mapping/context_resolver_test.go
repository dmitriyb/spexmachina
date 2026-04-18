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

// TestFR6_ResolveContext_MatchingSections covers S1 and S2: a component
// referenced by multiple impl_sections, a single test_section, and one of
// two data_flows. Lookup is by direct identity-hash match on Describes/Uses.
func TestFR6_ResolveContext_MatchingSections(t *testing.T) {
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "impact")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "impact",
		Components: []schema.Component{
			{ID: "aabbccddeeff", Name: "ActionClassifier", Content: "arch_action_classifier.md"},
			{ID: "ffeeddccbbaa", Name: "ReportGenerator", Content: "arch_report_generator.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: "111111111111", Name: "Classification rules", Content: "impl_classification.md", Describes: []string{"aabbccddeeff"}},
			{ID: "222222222222", Name: "Report format", Content: "impl_report_format.md", Describes: []string{"ffeeddccbbaa"}},
			{ID: "333333333333", Name: "Shared helpers", Content: "impl_shared.md", Describes: []string{"aabbccddeeff", "ffeeddccbbaa"}},
		},
		TestSections: []schema.TestSection{
			{ID: "444444444444", Name: "Classifier tests", Content: "test_classifier.md", Describes: []string{"aabbccddeeff"}},
			{ID: "555555555555", Name: "Report tests", Content: "test_report.md", Describes: []string{"ffeeddccbbaa"}},
		},
		DataFlows: []schema.DataFlow{
			{ID: "666666666666", Name: "Impact flow", Content: "flow_impact.md", Uses: []string{"aabbccddeeff", "ffeeddccbbaa"}},
			{ID: "777777777777", Name: "Other flow", Content: "flow_other.md", Uses: []string{"ffeeddccbbaa"}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          10,
		SpecNodeID:  "aabbccddeeff",
		Module:      "impact",
		Component:   "ActionClassifier",
		ContentFile: "spec/impact/arch_action_classifier.md",
	}

	result, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

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

	wantTest := []string{
		filepath.Join(specDir, "impact", "test_classifier.md"),
	}
	if len(result.TestFiles) != len(wantTest) {
		t.Fatalf("TestFiles count = %d, want %d", len(result.TestFiles), len(wantTest))
	}
	if result.TestFiles[0] != wantTest[0] {
		t.Errorf("TestFiles[0] = %q, want %q", result.TestFiles[0], wantTest[0])
	}

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

// TestFR6_ResolveContext_S4_SecondComponent covers scenario S4: a record
// targeting a second component in the same fixture resolves to that
// component's sections, confirming lookup is parameterised on the record,
// not baked into the resolver.
func TestFR6_ResolveContext_S4_SecondComponent(t *testing.T) {
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "impact")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "impact",
		Components: []schema.Component{
			{ID: "aabbccddeeff", Name: "ActionClassifier", Content: "arch_action_classifier.md"},
			{ID: "ffeeddccbbaa", Name: "ReportGenerator", Content: "arch_report_generator.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: "111111111111", Name: "Classification rules", Content: "impl_classification.md", Describes: []string{"aabbccddeeff"}},
			{ID: "222222222222", Name: "Report format", Content: "impl_report_format.md", Describes: []string{"ffeeddccbbaa"}},
			{ID: "333333333333", Name: "Shared helpers", Content: "impl_shared.md", Describes: []string{"aabbccddeeff", "ffeeddccbbaa"}},
		},
		TestSections: []schema.TestSection{
			{ID: "444444444444", Name: "Classifier tests", Content: "test_classifier.md", Describes: []string{"aabbccddeeff"}},
			{ID: "555555555555", Name: "Report tests", Content: "test_report.md", Describes: []string{"ffeeddccbbaa"}},
		},
		DataFlows: []schema.DataFlow{
			{ID: "666666666666", Name: "Impact flow", Content: "flow_impact.md", Uses: []string{"aabbccddeeff", "ffeeddccbbaa"}},
			{ID: "777777777777", Name: "Other flow", Content: "flow_other.md", Uses: []string{"ffeeddccbbaa"}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          11,
		SpecNodeID:  "ffeeddccbbaa",
		Module:      "impact",
		Component:   "ReportGenerator",
		ContentFile: "spec/impact/arch_report_generator.md",
	}

	result, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	if result.ArchFile != "spec/impact/arch_report_generator.md" {
		t.Errorf("ArchFile = %q, want ReportGenerator arch", result.ArchFile)
	}

	wantImpl := []string{
		filepath.Join(specDir, "impact", "impl_report_format.md"),
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

	if len(result.TestFiles) != 1 || result.TestFiles[0] != filepath.Join(specDir, "impact", "test_report.md") {
		t.Errorf("TestFiles = %v, want [test_report.md]", result.TestFiles)
	}

	wantFlow := []string{
		filepath.Join(specDir, "impact", "flow_impact.md"),
		filepath.Join(specDir, "impact", "flow_other.md"),
	}
	if len(result.FlowFiles) != len(wantFlow) {
		t.Fatalf("FlowFiles count = %d, want %d", len(result.FlowFiles), len(wantFlow))
	}
	for i, got := range result.FlowFiles {
		if got != wantFlow[i] {
			t.Errorf("FlowFiles[%d] = %q, want %q", i, got, wantFlow[i])
		}
	}
}

// TestFR6_ResolveContext_UnknownHash covers edge case E3: a record whose
// identity hash is valid but appears in no impl_section, test_section, or
// data_flow returns an empty result — this is not an error. The validator
// (not ResolveContext) is responsible for catching dangling references.
func TestFR6_ResolveContext_UnknownHash(t *testing.T) {
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "map")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "map",
		Components: []schema.Component{
			{ID: "aabbccddeeff", Name: "OnlyComponent", Content: "arch_only.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: "111111111111", Name: "Only impl", Content: "impl_only.md", Describes: []string{"aabbccddeeff"}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          99,
		SpecNodeID:  "deadbeefcafe",
		Module:      "map",
		Component:   "Ghost",
		ContentFile: "spec/map/arch_ghost.md",
	}

	result, err := ResolveContext(specDir, rec)
	if err != nil {
		t.Fatalf("ResolveContext should not error on unknown hash: %v", err)
	}
	if result.ArchFile != "spec/map/arch_ghost.md" {
		t.Errorf("ArchFile = %q, want passthrough from record", result.ArchFile)
	}
	if result.ModuleFile != filepath.Join(specDir, "map", "module.json") {
		t.Errorf("ModuleFile = %q, want module.json path", result.ModuleFile)
	}
	if len(result.ImplFiles) != 0 || len(result.TestFiles) != 0 || len(result.FlowFiles) != 0 {
		t.Errorf("all file lists should be empty for unknown hash; got impl=%v test=%v flow=%v",
			result.ImplFiles, result.TestFiles, result.FlowFiles)
	}
}

// TestFR6_ResolveContext_ModuleJsonNotFound covers edge case E1: missing
// module.json surfaces as a read error.
func TestFR6_ResolveContext_ModuleJsonNotFound(t *testing.T) {
	specDir := t.TempDir()
	rec := Record{
		SpecNodeID: "aabbccddeeff",
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

// TestFR6_ResolveContext_Deterministic asserts the pure-function contract:
// same inputs produce byte-identical outputs.
func TestFR6_ResolveContext_Deterministic(t *testing.T) {
	specDir := t.TempDir()
	modDir := filepath.Join(specDir, "schema")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := schema.ModuleSpec{
		Name: "schema",
		Components: []schema.Component{
			{ID: "aabbccddeeff", Name: "ProjectSchema", Content: "arch_project_schema.md"},
		},
		ImplSections: []schema.ImplSection{
			{ID: "111111111111", Name: "Schema format", Content: "impl_format.md", Describes: []string{"aabbccddeeff"}},
		},
		DataFlows: []schema.DataFlow{
			{ID: "222222222222", Name: "Load flow", Content: "flow_load.md", Uses: []string{"aabbccddeeff"}},
		},
	}
	writeModuleJSON(t, modDir, ms)

	rec := Record{
		ID:          1,
		SpecNodeID:  "aabbccddeeff",
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

	j1, _ := json.Marshal(r1)
	j2, _ := json.Marshal(r2)
	if string(j1) != string(j2) {
		t.Errorf("not deterministic:\n  first:  %s\n  second: %s", j1, j2)
	}
}
