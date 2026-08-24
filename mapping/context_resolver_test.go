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
// abc-123 (added at git_head beef0001) and recording removed node Widget
// (999999999999) — the fixture spec/map/test_context_resolver.md
// describes. Builder has an added event (e2) but no task_created receipt
// naming it, for S1c. Extra lines are appended to the journal after the
// base fixture, for scenarios that extend a node's history (S1b).
func buildContextFixture(t *testing.T, extra ...string) (specDir, journalPath string) {
	t.Helper()
	specDir = t.TempDir()

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

	lines := []string{
		changeLine("added", "e1", "aabbccddeeff", "Parser", "component", "alpha", "", "h1", "beef0001", ""),
		taskCreatedLine("e1", "", "abc-123"),
		changeLine("added", "e2", "ffeeddccbbaa", "Builder", "component", "alpha", "", "h2", "", ""),
		changeLine("added", "e3", "999999999999", "Widget", "component", "alpha", "", "h3", "beef0001", ""),
		taskCreatedLine("e3", "", "abc-777"),
		changeLine("removed", "e4", "999999999999", "Widget", "component", "alpha", "h3", "", "cafe1234", "2026-08-01-task-journal"),
	}
	lines = append(lines, extra...)
	journalPath = writeJournal(t, specDir, lines)

	return specDir, journalPath
}

// --- S1: live component by identity hash ---

func TestREQ_40a3d3155131_S1_ResolveLiveComponentByHash(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
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

	if result.Eid != "e1" {
		t.Errorf("Eid = %q, want e1", result.Eid)
	}
	if result.Event != "added" {
		t.Errorf("Event = %q, want added", result.Event)
	}
	if result.AfterHead != "beef0001" {
		t.Errorf("AfterHead = %q, want beef0001", result.AfterHead)
	}
	if result.BeforeHead != "" {
		t.Errorf("BeforeHead = %q, want empty — an add has no predecessor", result.BeforeHead)
	}
}

// --- S1b: bracket follows the lineage's latest event ---

func TestREQ_40a3d3155131_S1b_BracketFollowsLatestEvent(t *testing.T) {
	specDir, journalPath := buildContextFixture(t,
		changeLine("modified", "e5", "aabbccddeeff", "Parser", "component", "alpha", "h1", "h5", "cafe9999", ""),
		taskCreatedLine("e5", "", "abc-124"),
	)

	result, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	wantArch := filepath.Join(specDir, "alpha", "arch_parser.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q (file set unchanged from S1)", result.ArchFile, wantArch)
	}

	if result.Eid != "e5" {
		t.Errorf("Eid = %q, want e5", result.Eid)
	}
	if result.Event != "modified" {
		t.Errorf("Event = %q, want modified", result.Event)
	}
	if result.AfterHead != "cafe9999" {
		t.Errorf("AfterHead = %q, want cafe9999", result.AfterHead)
	}
	if result.BeforeHead != "beef0001" {
		t.Errorf("BeforeHead = %q, want beef0001 (the preceding event's git_head)", result.BeforeHead)
	}
}

// --- S1d: retargeted task widens the bracket ---

func TestREQ_76fe608c3a40_S1d_RetargetedTaskWidensBracket(t *testing.T) {
	specDir, journalPath := buildContextFixture(t,
		changeLine("modified", "e5", "aabbccddeeff", "Parser", "component", "alpha", "h1", "h5", "cafe9999", ""),
		taskRetargetedLine("e5", "abc-123"),
		changeLine("modified", "e8", "aabbccddeeff", "Parser", "component", "alpha", "h5", "h8", "feed0002", ""),
		taskRetargetedLine("e8", "abc-123"),
	)

	for _, key := range []string{"aabbccddeeff", "abc-123"} {
		t.Run(key, func(t *testing.T) {
			result, err := ResolveContext(specDir, journalPath, key)
			if err != nil {
				t.Fatalf("ResolveContext(%q): %v", key, err)
			}

			wantArch := filepath.Join(specDir, "alpha", "arch_parser.md")
			if result.ArchFile != wantArch {
				t.Errorf("ArchFile = %q, want %q (file set unchanged from S1)", result.ArchFile, wantArch)
			}

			if result.Eid != "e8" {
				t.Errorf("Eid = %q, want e8 (the latest retargeted event)", result.Eid)
			}
			if result.Event != "modified" {
				t.Errorf("Event = %q, want modified", result.Event)
			}
			if result.AfterHead != "feed0002" {
				t.Errorf("AfterHead = %q, want feed0002 (the latest retargeted event's git_head)", result.AfterHead)
			}
			if result.BeforeHead != "" {
				t.Errorf("BeforeHead = %q, want empty — the task's original task_created was born from an added, which has no predecessor", result.BeforeHead)
			}
		})
	}
}

// --- S1d resurrected: a retargeted task born on a resurrection add still
// has a null before_head, not the prior removal's git_head ---

func TestREQ_76fe608c3a40_S1d_RetargetedResurrectedAdd_BeforeHeadNull(t *testing.T) {
	specDir, journalPath := buildContextFixture(t,
		changeLine("added", "e1g", "112233445566", "Ghost", "component", "alpha", "", "g1", "beefg001", ""),
		changeLine("removed", "e5g", "112233445566", "Ghost", "component", "alpha", "g1", "", "cafe1111", "some-proposal"),
		changeLine("added", "e6g", "112233445566", "Ghost", "component", "alpha", "", "g6", "cafe2222", ""),
		taskCreatedLine("e6g", "", "abc-999"),
		changeLine("modified", "e7g", "112233445566", "Ghost", "component", "alpha", "g6", "g7", "cafe3333", ""),
		taskRetargetedLine("e7g", "abc-999"),
	)

	result, err := ResolveContext(specDir, journalPath, "112233445566")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result (re-added), got Removed=true: %+v", result)
	}

	if result.Eid != "e7g" {
		t.Errorf("Eid = %q, want e7g (the latest retargeted event)", result.Eid)
	}
	if result.Event != "modified" {
		t.Errorf("Event = %q, want modified", result.Event)
	}
	if result.AfterHead != "cafe3333" {
		t.Errorf("AfterHead = %q, want cafe3333", result.AfterHead)
	}
	if result.BeforeHead != "" {
		t.Errorf("BeforeHead = %q, want empty — abc-999 was born on the resurrection add (e6g), which has no predecessor regardless of its position in the lineage", result.BeforeHead)
	}
}

// --- S1c: live node with no task-bearing event serves a null bracket ---

func TestREQ_40a3d3155131_S1c_NullBracketForNoTaskBearingEvent(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "ffeeddccbbaa")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result, got Removed=true: %+v", result)
	}

	wantArch := filepath.Join(specDir, "alpha", "arch_builder.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q — the file set never depends on the journal", result.ArchFile, wantArch)
	}

	if result.Eid != "" || result.Event != "" || result.BeforeHead != "" || result.AfterHead != "" {
		t.Errorf("want null bracket for a node with no task-bearing event, got %+v", result)
	}
}

// --- a resurrected node's before_head is null for the add, not the prior removal ---

func TestREQ_40a3d3155131_ResurrectedAdd_BeforeHeadNull(t *testing.T) {
	specDir, journalPath := buildContextFixture(t,
		changeLine("added", "e6", "112233445566", "Ghost", "component", "alpha", "", "g6", "cafeaaa1", ""),
		taskCreatedLine("e6", "", "task-001"),
		changeLine("removed", "e7", "112233445566", "Ghost", "component", "alpha", "g6", "", "cafeaaa2", "some-proposal"),
		changeLine("added", "e8", "112233445566", "Ghost", "component", "alpha", "", "g8", "cafeaaa3", ""),
		taskCreatedLine("e8", "", "task-002"),
	)

	result, err := ResolveContext(specDir, journalPath, "112233445566")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result (re-added), got Removed=true: %+v", result)
	}

	if result.Eid != "e8" {
		t.Errorf("Eid = %q, want e8 (the resurrection event)", result.Eid)
	}
	if result.Event != "added" {
		t.Errorf("Event = %q, want added", result.Event)
	}
	if result.AfterHead != "cafeaaa3" {
		t.Errorf("AfterHead = %q, want cafeaaa3", result.AfterHead)
	}
	if result.BeforeHead != "" {
		t.Errorf("BeforeHead = %q, want empty — the latest event is an added, so it has no predecessor regardless of position in the lineage", result.BeforeHead)
	}
}

// --- S2: resolve by task id reaches the same node ---

func TestREQ_40a3d3155131_S2_ResolveByTaskIDMatchesHash(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	byHash, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
	if err != nil {
		t.Fatalf("ResolveContext by hash: %v", err)
	}
	byTask, err := ResolveContext(specDir, journalPath, "abc-123")
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
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
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
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "112233445566")
	if err != nil {
		t.Fatalf("ResolveContext should not error for a component with no data_flows: %v", err)
	}
	if len(result.FlowFiles) != 0 {
		t.Errorf("FlowFiles = %v, want empty", result.FlowFiles)
	}
}

// --- S5: removed node resolves from the journal ---

func TestREQ_40a3d3155131_S5_RemovedNodeResolvesFromJournal(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "999999999999")
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
	if result.Eid != "e4" {
		t.Errorf("Eid = %q, want e4 (the removal event's own eid)", result.Eid)
	}
	if result.Event != "removed" {
		t.Errorf("Event = %q, want removed", result.Event)
	}
	if result.AfterHead != "cafe1234" {
		t.Errorf("AfterHead = %q, want cafe1234", result.AfterHead)
	}
	if result.BeforeHead != "beef0001" {
		t.Errorf("BeforeHead = %q, want beef0001 (the prior change's git_head)", result.BeforeHead)
	}
}

// --- S6: live data_flow by identity hash ---

func TestREQ_40a3d3155131_S6_ResolveLiveDataFlowByHash(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "444444444444")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result, got Removed=true: %+v", result)
	}

	wantArch := filepath.Join(specDir, "alpha", "flow_pipeline.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q", result.ArchFile, wantArch)
	}
	if len(result.TestFiles) != 0 || len(result.FlowFiles) != 0 {
		t.Errorf("TestFiles/FlowFiles should be empty for a data_flow node, got test=%v flow=%v", result.TestFiles, result.FlowFiles)
	}
	wantModule := filepath.Join(specDir, "alpha", "module.json")
	if result.ModuleFile != wantModule {
		t.Errorf("ModuleFile = %q, want %q", result.ModuleFile, wantModule)
	}
}

// --- S7: live test_section by identity hash ---

func TestREQ_40a3d3155131_S7_ResolveLiveTestSectionByHash(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "333333333333")
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if result.Removed {
		t.Fatalf("want live result, got Removed=true: %+v", result)
	}

	wantArch := filepath.Join(specDir, "alpha", "test_components.md")
	if result.ArchFile != wantArch {
		t.Errorf("ArchFile = %q, want %q", result.ArchFile, wantArch)
	}
	if len(result.TestFiles) != 0 || len(result.FlowFiles) != 0 {
		t.Errorf("TestFiles/FlowFiles should be empty for a test_section node, got test=%v flow=%v", result.TestFiles, result.FlowFiles)
	}
	wantModule := filepath.Join(specDir, "alpha", "module.json")
	if result.ModuleFile != wantModule {
		t.Errorf("ModuleFile = %q, want %q", result.ModuleFile, wantModule)
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

	journalPath := writeJournal(t, specDir, []string{
		changeLine("added", "e1", "aabbccddeeff", "Parser", "component", "missing", "", "h1", "", ""),
	})

	_, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
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
	specDir, journalPath := buildContextFixture(t)

	_, err := ResolveContext(specDir, journalPath, "deadbeefdead")
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
	specDir, journalPath := buildContextFixture(t)

	result, err := ResolveContext(specDir, journalPath, "112233445566")
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

// --- E4: malformed journal propagates as a parse error for a task-id lookup ---

func TestREQ_40a3d3155131_E4_MalformedJournalPropagatesForTaskIDLookup(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)
	writeJournal(t, specDir, []string{`{"event":"added","eid":"e1"invalid}`})

	_, err := ResolveContext(specDir, journalPath, "abc-123")
	if err == nil {
		t.Fatal("want error for malformed journal, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want a parse error, not ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "journal line 1") {
		t.Errorf("error = %q, want naming the malformed journal line", err.Error())
	}
	if !strings.Contains(err.Error(), "context:") {
		t.Errorf("error = %q, want the context: prefix", err.Error())
	}
}

// --- E5: malformed journal propagates as a parse error for a removed-node lookup ---

func TestREQ_40a3d3155131_E5_MalformedJournalPropagatesForRemovedLookup(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)
	writeJournal(t, specDir, []string{`{"event":"added","eid":"e1"invalid}`})

	_, err := ResolveContext(specDir, journalPath, "deadbeefdead")
	if err == nil {
		t.Fatal("want error for malformed journal, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want a parse error, not ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "journal line 1") {
		t.Errorf("error = %q, want naming the malformed journal line", err.Error())
	}
	if !strings.Contains(err.Error(), "context:") {
		t.Errorf("error = %q, want the context: prefix", err.Error())
	}
}

// --- Pure-function determinism ---

func TestREQ_40a3d3155131_Deterministic(t *testing.T) {
	specDir, journalPath := buildContextFixture(t)

	r1, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	r2, err := ResolveContext(specDir, journalPath, "aabbccddeeff")
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
