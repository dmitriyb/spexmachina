package mapping

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// --- fixture helpers ---

// writeProjectJSON writes a schema.Project as project.json into specDir.
func writeProjectJSON(t *testing.T, specDir string, proj schema.Project) {
	t.Helper()
	data, err := json.MarshalIndent(proj, "", "  ")
	if err != nil {
		t.Fatalf("marshal project.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "project.json"), data, 0644); err != nil {
		t.Fatalf("write project.json: %v", err)
	}
}

// writeModuleJSON writes a schema.ModuleSpec as module.json into the given directory.
func writeModuleJSON(t *testing.T, dir string, ms schema.ModuleSpec) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(ms, "", "  ")
	if err != nil {
		t.Fatalf("marshal module.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.json"), data, 0644); err != nil {
		t.Fatalf("write module.json: %v", err)
	}
}

// buildContextFixture lays out the alpha module (Parser, Builder, Ghost),
// its test_sections and data_flow, and a journal pairing Parser with task
// abc-123 and recording removed node Widget (999999999999) — the fixture
// spec/map/test_context_resolver.md describes.
func buildContextFixture(t *testing.T) string {
	t.Helper()
	specDir := t.TempDir()

	writeProjectJSON(t, specDir, schema.Project{
		Name: "fixture",
		Modules: []schema.Module{
			{ID: "aaaa11112222", Name: "alpha", Path: "alpha"},
		},
	})

	writeModuleJSON(t, filepath.Join(specDir, "alpha"), schema.ModuleSpec{
		Name: "alpha",
		Components: []schema.Component{
			{ID: "aabbccddeeff", Name: "Parser", Content: "arch_parser.md"},
			{ID: "ffeeddccbbaa", Name: "Builder", Content: "arch_builder.md"},
			{ID: "112233445566", Name: "Ghost", Content: "arch_ghost.md"},
		},
		TestSections: []schema.TestSection{
			{ID: "333333333333", Name: "Component tests", Content: "test_components.md", Describes: []string{"aabbccddeeff", "ffeeddccbbaa"}},
			{ID: "555555555555", Name: "Parser-only tests", Content: "test_parser.md", Describes: []string{"aabbccddeeff"}},
		},
		DataFlows: []schema.DataFlow{
			{ID: "444444444444", Name: "Pipeline flow", Content: "flow_pipeline.md", Uses: []string{"aabbccddeeff", "ffeeddccbbaa"}},
		},
	})

	writeJournal(t, specDir, []string{
		changeLine("added", "e1", "aabbccddeeff", "Parser", "component", "alpha", "", "h1", "", ""),
		taskCreatedLine("e1", "", "abc-123"),
		changeLine("added", "e2", "ffeeddccbbaa", "Builder", "component", "alpha", "", "h2", "", ""),
		changeLine("added", "e3", "999999999999", "Widget", "component", "alpha", "", "h3", "babe0000", ""),
		taskCreatedLine("e3", "", "abc-777"),
		changeLine("removed", "e4", "999999999999", "Widget", "component", "alpha", "h3", "", "cafe1234", "2026-08-01-task-journal"),
	})

	return specDir
}

// --- S1: live component by identity hash ---

func TestREQ_40a3d3155131_S1_ResolveLiveComponentByHash(t *testing.T) {
	specDir := buildContextFixture(t)

	result, err := ResolveContext(specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result, got Removed=true: %+v", result)
	}

	wantArch := filepath.Join(specDir, "alpha", "arch_parser.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q", result.ArchFile, wantArch)
	}

	wantTest := filepath.Join(specDir, "alpha", "test_components.md")
	if !slicesContains(result.TestFiles, wantTest) {
		t.Errorf("TestFiles = %v, want to contain %q", result.TestFiles, wantTest)
	}

	wantFlow := filepath.Join(specDir, "alpha", "flow_pipeline.md")
	if !slicesContains(result.FlowFiles, wantFlow) {
		t.Errorf("FlowFiles = %v, want to contain %q", result.FlowFiles, wantFlow)
	}

	wantModule := filepath.Join(specDir, "alpha", "module.json")
	if result.ModuleFile != wantModule {
		t.Errorf("ModuleFile = %q, want %q", result.ModuleFile, wantModule)
	}
}

// --- S2: resolve by task id reaches the same node ---

func TestREQ_40a3d3155131_S2_ResolveByTaskIDMatchesHash(t *testing.T) {
	specDir := buildContextFixture(t)

	byHash, err := ResolveContext(specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("ResolveContext by hash: %v", err)
	}
	byTask, err := ResolveContext(specDir, "abc-123")
	if err != nil {
		t.Fatalf("ResolveContext by task id: %v", err)
	}

	byHashJSON, _ := json.Marshal(byHash)
	byTaskJSON, _ := json.Marshal(byTask)
	if string(byHashJSON) != string(byTaskJSON) {
		t.Errorf("task-id resolution diverged from hash resolution:\n  hash: %s\n  task: %s", byHashJSON, byTaskJSON)
	}
}

// --- S3: component referenced by multiple test_sections ---

func TestREQ_40a3d3155131_S3_MultipleTestSectionsInDeclarationOrder(t *testing.T) {
	specDir := buildContextFixture(t)

	result, err := ResolveContext(specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	want := []string{
		filepath.Join(specDir, "alpha", "test_components.md"),
		filepath.Join(specDir, "alpha", "test_parser.md"),
	}
	if len(result.TestFiles) != len(want) {
		t.Fatalf("TestFiles = %v, want %v", result.TestFiles, want)
	}
	for i, w := range want {
		if result.TestFiles[i] != w {
			t.Errorf("TestFiles[%d] = %q, want %q", i, result.TestFiles[i], w)
		}
	}
}

// --- S4: component with no data_flows ---

func TestREQ_40a3d3155131_S4_ComponentWithNoDataFlows(t *testing.T) {
	specDir := buildContextFixture(t)

	result, err := ResolveContext(specDir, "112233445566")
	if err != nil {
		t.Fatalf("ResolveContext should not error for a component with no data_flows: %v", err)
	}
	if len(result.FlowFiles) != 0 {
		t.Errorf("FlowFiles = %v, want empty", result.FlowFiles)
	}
}

// --- S5: removed node resolves from the journal ---

func TestREQ_40a3d3155131_S5_RemovedNodeResolvesFromJournal(t *testing.T) {
	specDir := buildContextFixture(t)

	result, err := ResolveContext(specDir, "999999999999")
	if err != nil {
		t.Fatalf("ResolveContext should not error for a removed-but-remembered node: %v", err)
	}
	if !result.Removed {
		t.Fatalf("want Removed=true, got %+v", result)
	}
	if result.ArchFile != "" || result.ModuleFile != "" || len(result.TestFiles) != 0 || len(result.FlowFiles) != 0 {
		t.Errorf("removed result should carry no live spec paths, got %+v", result)
	}
	if result.Name != "Widget" {
		t.Errorf("Name = %q, want Widget", result.Name)
	}
	if result.NodeType != "component" {
		t.Errorf("NodeType = %q, want component", result.NodeType)
	}
	if result.Module != "alpha" {
		t.Errorf("Module = %q, want alpha", result.Module)
	}
	if result.Proposal != "2026-08-01-task-journal" {
		t.Errorf("Proposal = %q, want 2026-08-01-task-journal", result.Proposal)
	}
	if result.TaskID != "abc-777" {
		t.Errorf("TaskID = %q, want abc-777 (the last task before removal)", result.TaskID)
	}
	if result.AfterHead != "cafe1234" {
		t.Errorf("AfterHead = %q, want cafe1234", result.AfterHead)
	}
	if result.BeforeHead != "babe0000" {
		t.Errorf("BeforeHead = %q, want babe0000 (the prior change's git_head)", result.BeforeHead)
	}
}

// --- E1: missing module.json ---

func TestREQ_40a3d3155131_E1_MissingModuleJSON(t *testing.T) {
	specDir := t.TempDir()

	writeProjectJSON(t, specDir, schema.Project{
		Name: "fixture",
		Modules: []schema.Module{
			{ID: "aaaa11112222", Name: "missing", Path: "missing"},
		},
	})
	// No module.json written under "missing" — the directory itself is
	// never created.

	writeJournal(t, specDir, []string{
		changeLine("added", "e1", "aabbccddeeff", "Parser", "component", "missing", "", "h1", "", ""),
	})

	_, err := ResolveContext(specDir, "aabbccddeeff")
	if err == nil {
		t.Fatal("want error for missing module.json, got nil")
	}
	wantPath := filepath.Join(specDir, "missing", "module.json")
	if !strings.Contains(err.Error(), "read") || !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error = %q, want containing 'read' and %q", err.Error(), wantPath)
	}
}

// --- E2: key known to neither spec nor journal ---

func TestREQ_40a3d3155131_E2_UnknownEverywhere(t *testing.T) {
	specDir := buildContextFixture(t)

	_, err := ResolveContext(specDir, "deadbeefdead")
	if err == nil {
		t.Fatal("want not-found error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want wrapping ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "deadbeefdead") {
		t.Errorf("error = %q, want naming the key", err.Error())
	}
}

// --- E3: component hash not found in any section ---

func TestREQ_40a3d3155131_E3_ComponentInNoSection(t *testing.T) {
	specDir := buildContextFixture(t)

	result, err := ResolveContext(specDir, "112233445566")
	if err != nil {
		t.Fatalf("ResolveContext should not error for a component described/used by nothing: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result, got Removed=true: %+v", result)
	}
	if len(result.TestFiles) != 0 || len(result.FlowFiles) != 0 {
		t.Errorf("TestFiles/FlowFiles should be empty, got test=%v flow=%v", result.TestFiles, result.FlowFiles)
	}
	wantArch := filepath.Join(specDir, "alpha", "arch_ghost.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q", result.ArchFile, wantArch)
	}
	wantModule := filepath.Join(specDir, "alpha", "module.json")
	if result.ModuleFile != wantModule {
		t.Errorf("ModuleFile = %q, want %q", result.ModuleFile, wantModule)
	}
}

// --- Pure-function determinism ---

func TestREQ_40a3d3155131_Deterministic(t *testing.T) {
	specDir := buildContextFixture(t)

	r1, err := ResolveContext(specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	r2, err := ResolveContext(specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	j1, _ := json.Marshal(r1)
	j2, _ := json.Marshal(r2)
	if string(j1) != string(j2) {
		t.Errorf("not deterministic:\n  first:  %s\n  second: %s", j1, j2)
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
