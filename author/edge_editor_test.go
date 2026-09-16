package author

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// addSecondModule registers a second module named modName in f's
// project.json and gives it one component named compName — the "beta"
// module test_node_editing.md's N12 needs to exercise a cross-module
// target, built directly (not through NodeEditor, which is a different
// bead's own scope) the same way TestAdd_RefusesNonEmptyModuleSkeleton
// (node_editor_test.go) hand-builds a second module.
func addSecondModule(t *testing.T, f obligationFixture, modName, compName string) (moduleID, compID string) {
	t.Helper()

	projPath := filepath.Join(f.dir, "project.json")
	data, err := os.ReadFile(projPath)
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatal(err)
	}
	moduleID = schema.IdentityHash("module", modName)
	proj.Modules = append(proj.Modules, schema.Module{ID: moduleID, Name: modName, Path: modName})
	writeJSON(t, projPath, proj)

	modDir := filepath.Join(f.dir, modName)
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}
	compID = schema.IdentityHash(modName, "component", compName)
	mod := schema.ModuleSpec{
		Name: modName,
		Components: []schema.Component{
			{ID: compID, Name: compName, Content: "arch_" + strings.ToLower(compName) + ".md"},
		},
	}
	writeJSON(t, filepath.Join(modDir, "module.json"), mod)
	writeFile(t, filepath.Join(modDir, "arch_"+strings.ToLower(compName)+".md"), "# "+compName+"\n")

	return moduleID, compID
}

// TestN9_EdgeCheckedAgainstProfileBeforeWrite covers test_node_editing.md's
// N9: spex edge add is refused, before any write, both when the source
// type does not declare the named field as a reference kind (describes is
// declared on test_section, not component) and when it does but the field
// does not permit the target's type (uses targets only component, not
// requirement).
func TestN9_EdgeCheckedAgainstProfileBeforeWrite(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("field not declared on source type", func(t *testing.T) {
		report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "describes", TargetID: f.comp1ID})
		if err != nil {
			t.Fatalf("AddEdge: unexpected error: %v", err)
		}
		if report != nil {
			t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
		}
		if len(refusals) != 1 {
			t.Fatalf("AddEdge: want exactly one refusal, got %+v", refusals)
		}
		if !strings.Contains(refusals[0].Message, "describes") {
			t.Errorf("message should name the field, got: %s", refusals[0].Message)
		}
		for _, want := range []string{"implements", "uses"} {
			if !strings.Contains(refusals[0].Fix, want) {
				t.Errorf("fix should list declared reference field %q, got: %s", want, refusals[0].Fix)
			}
		}
	})

	t.Run("field does not permit target type", func(t *testing.T) {
		report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "uses", TargetID: f.r1ID})
		if err != nil {
			t.Fatalf("AddEdge: unexpected error: %v", err)
		}
		if report != nil {
			t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
		}
		if len(refusals) != 1 {
			t.Fatalf("AddEdge: want exactly one refusal, got %+v", refusals)
		}
		if !strings.Contains(refusals[0].Fix, "component") {
			t.Errorf("fix should name component as the only target type uses permits, got: %s", refusals[0].Fix)
		}
	})

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}
}

// TestN10_CycleRefusedBeforeWrite covers test_node_editing.md's N10:
// Comp2 already uses Comp1; adding uses from Comp1 to Comp2 would close a
// cycle, refused with the validator's own dag message and fix, nothing
// written — the one check EdgeEditor defers entirely to Report.
func TestN10_CycleRefusedBeforeWrite(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp1ID, Field: "uses", TargetID: f.comp2ID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("AddEdge: want exactly one refusal, got %+v", refusals)
	}
	if refusals[0].Check != "dag" {
		t.Errorf("Check = %q, want %q (the validator's own dag check)", refusals[0].Check, "dag")
	}
	if !strings.Contains(refusals[0].Message, "cycle") {
		t.Errorf("message should name the cycle, got: %s", refusals[0].Message)
	}
	if !strings.Contains(refusals[0].Fix, "remove one edge from the cycle") {
		t.Errorf("fix should name the cycle-breaking action, got: %s", refusals[0].Fix)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity oracle: the same entry written by hand fails spex validate
	// with the same dag message.
	hand := buildRenameFixture(t)
	var mod schema.ModuleSpec
	modPath := filepath.Join(hand.dir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatal(err)
	}
	for i := range mod.Components {
		if mod.Components[i].ID == hand.comp1ID {
			mod.Components[i].Uses = append(mod.Components[i].Uses, hand.comp2ID)
		}
	}
	writeJSON(t, modPath, mod)
	handErrs := validator.CheckDAGFS(os.DirFS(hand.dir))
	if len(handErrs) != 1 {
		t.Fatalf("want exactly one hand-edit dag error, got %+v", handErrs)
	}
	if handErrs[0].Message != refusals[0].Message {
		t.Errorf("command message %q != hand-edit validator message %q", refusals[0].Message, handErrs[0].Message)
	}
}

// TestN11_ExistingEdgeIsNoOp_AndRemovalRestoresTree covers
// test_node_editing.md's N11: Comp2 already uses Comp1 (the fixture's own
// shape); adding it again is a no-op that leaves the tree byte-identical,
// and removing it afterwards drops the entry and reports the meta
// obligations on Comp1 and Comp2.
func TestN11_ExistingEdgeIsNoOp_AndRemovalRestoresTree(t *testing.T) {
	f := buildRenameFixture(t)
	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	addReport, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "uses", TargetID: f.comp1ID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("AddEdge: unexpected refusals: %+v", refusals)
	}
	if addReport == nil {
		t.Fatal("AddEdge: want a report for a no-op, got nil")
	}
	if len(addReport.Written) != 0 {
		t.Errorf("AddEdge: want no files written for an existing entry, got %+v", addReport.Written)
	}
	afterAdd, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterAdd) {
		t.Error("AddEdge: a no-op must leave the tree byte-identical")
	}

	removeReport, refusals, err := RemoveEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "uses", TargetID: f.comp1ID})
	if err != nil {
		t.Fatalf("RemoveEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("RemoveEdge: unexpected refusals: %+v", refusals)
	}
	if removeReport == nil || len(removeReport.Written) == 0 {
		t.Fatalf("RemoveEdge: want a write, got %+v", removeReport)
	}

	var mod schema.ModuleSpec
	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	for _, c := range mod.Components {
		if c.ID == f.comp2ID && len(c.Uses) != 0 {
			t.Errorf("Comp2.Uses should be empty after removal, got %v", c.Uses)
		}
	}

	keys := obligationKeys(t, removeReport.Obligations)
	for _, name := range []string{"Comp1", "Comp2"} {
		if !containsSubstring(keys, name) {
			t.Errorf("want an obligation naming %s, got %+v", name, keys)
		}
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	changes := merkle.Diff(afterTree, beforeTree)
	var metaChanges, otherChanges []merkle.Change
	for _, c := range changes {
		if c.NodeType == "meta" {
			metaChanges = append(metaChanges, c)
		} else {
			otherChanges = append(otherChanges, c)
		}
	}
	if len(metaChanges) != 1 {
		t.Errorf("want exactly one meta change (alpha), got %+v", metaChanges)
	}
	if len(otherChanges) != 0 {
		t.Errorf("want no other changes, got %+v", otherChanges)
	}
}

// TestN12_CrossModuleTargetRefusedOnModuleLocalField covers
// test_node_editing.md's N12: provided_by is module-local, so pointing
// alpha's api at a component from module beta is refused with the
// validator's own module-local message, the fix naming alpha's own
// components as the permissible targets.
func TestN12_CrossModuleTargetRefusedOnModuleLocalField(t *testing.T) {
	f := buildRenameFixture(t)
	_, otherID := addSecondModule(t, f, "beta", "Other")

	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.apiID, Field: "provided_by", TargetID: otherID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("AddEdge: want exactly one refusal, got %+v", refusals)
	}

	wantMessage := "provided_by references non-existent component " + otherID + " (provided_by is module-local)"
	if refusals[0].Message != wantMessage {
		t.Errorf("Message = %q, want the validator's own message %q", refusals[0].Message, wantMessage)
	}
	if refusals[0].Check != "id" {
		t.Errorf("Check = %q, want %q (the validator's own check)", refusals[0].Check, "id")
	}
	for _, want := range []string{"Comp1", "Comp2"} {
		if !strings.Contains(refusals[0].Fix, want) {
			t.Errorf("fix should name alpha's own component %q as a permissible target, got: %s", want, refusals[0].Fix)
		}
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity: the validator itself raises the identical message over the
	// same entry written by hand.
	var mod schema.ModuleSpec
	if err := json.Unmarshal(after, &mod); err != nil {
		t.Fatal(err)
	}
	for i := range mod.APIs {
		if mod.APIs[i].ID == f.apiID {
			mod.APIs[i].ProvidedBy = append(mod.APIs[i].ProvidedBy, otherID)
		}
	}
	writeJSON(t, filepath.Join(f.dir, "alpha", "module.json"), mod)
	handErrs := validator.CheckIDsFS(os.DirFS(f.dir))
	found := false
	for _, e := range handErrs {
		if e.Check == "id" && e.Message == wantMessage {
			found = true
		}
	}
	if !found {
		t.Errorf("want the hand-edited tree to fail spex validate with %q, got %+v", wantMessage, handErrs)
	}
}

// TestN13_HandFormattedFileReformatted_NoHashMoves covers
// test_node_editing.md's N13: alpha/module.json rewritten by hand as one
// compact line; spex edge add still succeeds, rewrites the file in
// canonical two-space form, and spex diff reports only the meta change of
// alpha and its obligations — the reformat moves no leaf hash.
func TestN13_HandFormattedFileReformatted_NoHashMoves(t *testing.T) {
	f := buildRenameFixture(t)

	modPath := filepath.Join(f.dir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	compact, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, compact, 0644); err != nil {
		t.Fatal(err)
	}

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "implements", TargetID: f.r1ID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("AddEdge: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("AddEdge: want a report, got nil")
	}

	after, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), "{\n  \"") {
		t.Errorf("want canonical two-space indent at the top level, got: %s", after[:20])
	}
	for _, line := range strings.Split(string(after), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent%2 != 0 {
			t.Errorf("want every indent level to be a multiple of two spaces, got %d in line %q", indent, line)
		}
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	changes := merkle.Diff(afterTree, beforeTree)
	var metaChanges, otherChanges []merkle.Change
	for _, c := range changes {
		if c.NodeType == "meta" {
			metaChanges = append(metaChanges, c)
		} else {
			otherChanges = append(otherChanges, c)
		}
	}
	if len(metaChanges) != 1 {
		t.Errorf("want exactly one meta change (alpha's own reformat+edit), got %+v", metaChanges)
	}
	if len(otherChanges) != 0 {
		t.Errorf("want no leaf hash to move — components/apis/test_sections unchanged — got %+v", otherChanges)
	}
}

// TestAddEdge_TargetDoesNotExist_RefusesNamingSearchedArrays exercises
// arch_edge_editor.md's first check outside any named N-scenario: a target
// id absent from the whole tree is refused with the arrays that would have
// carried it, by file and key.
func TestAddEdge_TargetDoesNotExist_RefusesNamingSearchedArrays(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "uses", TargetID: "ffffffffffff"})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("AddEdge: want exactly one refusal, got %+v", refusals)
	}
	if !strings.Contains(refusals[0].Message, "ffffffffffff") {
		t.Errorf("message should name the missing target, got: %s", refusals[0].Message)
	}
	if !strings.Contains(refusals[0].Fix, "alpha/module.json:/components") {
		t.Errorf("fix should name the searched array by file and key, got: %s", refusals[0].Fix)
	}
}

// TestAddEdge_CardinalityOneRetarget_ReplacesAndReportsDisplacedTarget
// exercises N15 (test_node_editing.md): preq_id has cardinality one; R1
// already holds P1, so adding P2 replaces it — the write succeeds, R1's
// preq_id now holds P2, and the write report carries P1 under
// ReplacedTarget so the retarget is visible rather than silent
// (arch_edge_editor.md, "Idempotence").
func TestAddEdge_CardinalityOneRetarget_ReplacesAndReportsDisplacedTarget(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.r1ID, Field: "preq_id", TargetID: f.p2ID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("AddEdge: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) == 0 {
		t.Fatalf("AddEdge: want a write, got %+v", report)
	}
	if report.ReplacedTarget != f.p1ID {
		t.Errorf("ReplacedTarget = %q, want the displaced target %q", report.ReplacedTarget, f.p1ID)
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	for _, r := range mod.Requirements {
		if r.ID == f.r1ID && r.PreqID != f.p2ID {
			t.Errorf("R1's preq_id should now be %s, got %s", f.p2ID, r.PreqID)
		}
	}

	keys := obligationKeys(t, report.Obligations)
	if !containsSubstring(keys, "Comp1") {
		t.Errorf("want a completeness obligation naming Comp1 (R1's implementor), got %+v", report.Obligations)
	}
	if !containsSubstring(keys, f.p1ID) {
		t.Errorf("want a requirement_coverage obligation naming P1, now derived by nothing, got %+v", report.Obligations)
	}
}

// TestRemoveEdge_RequiredCardinalityOne_RefusesRatherThanClear exercises
// N15's other half: preq_id is required, so RemoveEdge cannot clear it to
// an absent state. The after-state Report builds is missing a required
// field, which the validator's own schema and id checks — newly
// introduced by this change — refuse before anything is written
// (arch_edge_editor.md, "Idempotence": "a required cardinality-one field
// is retargeted by one add, never through a cleared state it cannot
// reach").
func TestRemoveEdge_RequiredCardinalityOne_RefusesRatherThanClear(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := RemoveEdge(f.dir, EdgeInput{SourceID: f.r1ID, Field: "preq_id", TargetID: f.p1ID})
	if err != nil {
		t.Fatalf("RemoveEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("RemoveEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 2 {
		t.Fatalf("RemoveEdge: want exactly two refusals (schema + id), got %+v", refusals)
	}

	var schemaEntry, idEntry *RefusalEntry
	for i := range refusals {
		switch refusals[i].Check {
		case "schema":
			schemaEntry = &refusals[i]
		case "id":
			idEntry = &refusals[i]
		}
	}
	if schemaEntry == nil {
		t.Fatalf("want a schema refusal for the missing required preq_id, got %+v", refusals)
	}
	if !strings.Contains(schemaEntry.Message, "preq_id") {
		t.Errorf("schema message should name preq_id, got: %s", schemaEntry.Message)
	}
	if !strings.Contains(schemaEntry.Fix, "preq_id") || !strings.Contains(schemaEntry.Fix, "reference") {
		t.Errorf("schema fix should name the field and its kind, got: %s", schemaEntry.Fix)
	}
	if idEntry == nil {
		t.Fatalf("want an id refusal for the requirement missing its preq_id, got %+v", refusals)
	}
	if !strings.Contains(idEntry.Message, f.r1ID) || !strings.Contains(idEntry.Message, "missing preq_id") {
		t.Errorf("id message should be the validator's own 'missing preq_id' finding, got: %s", idEntry.Message)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("RemoveEdge: a refusal must leave the tree byte-identical")
	}
}

// TestRemoveEdge_AbsentEntryIsNoOp exercises "removing one the field does
// not hold changes nothing and says so": Comp2 does not implement R1, so
// removing that entry is a no-op.
func TestRemoveEdge_AbsentEntryIsNoOp(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := RemoveEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "implements", TargetID: f.r1ID})
	if err != nil {
		t.Fatalf("RemoveEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("RemoveEdge: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 0 {
		t.Fatalf("RemoveEdge: want a no-op report, got %+v", report)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("RemoveEdge: a no-op must leave the tree byte-identical")
	}
}

// TestRemoveEdge_UndeclaredField_Refuses mirrors N9's first check for
// RemoveEdge: a field the source type does not declare as a reference kind
// is refused the same way AddEdge refuses it, since there is no entry to
// search for one it does not declare.
func TestRemoveEdge_UndeclaredField_Refuses(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := RemoveEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "describes", TargetID: f.comp1ID})
	if err != nil {
		t.Fatalf("RemoveEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("RemoveEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("RemoveEdge: want exactly one refusal, got %+v", refusals)
	}
}

// TestEdge_RequiresModule_AddedAndRemoved covers the frame's own fixed
// edge, requires_module (arch_edge_editor.md: "modules are frame nodes and
// the edge belongs to the frame ... so a module dependency is added the
// same way a component dependency is"). beta requiring alpha is added,
// idempotent on a second add, and removed cleanly.
func TestEdge_RequiresModule_AddedAndRemoved(t *testing.T) {
	f := buildRenameFixture(t)
	betaID, _ := addSecondModule(t, f, "beta", "Other")
	alphaID := "000000000001"

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: betaID, Field: "requires_module", TargetID: alphaID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("AddEdge: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) == 0 {
		t.Fatalf("AddEdge: want a write, got %+v", report)
	}

	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatal(err)
	}
	var beta *schema.Module
	for i := range proj.Modules {
		if proj.Modules[i].ID == betaID {
			beta = &proj.Modules[i]
		}
	}
	if beta == nil || len(beta.RequiresModule) != 1 || beta.RequiresModule[0] != alphaID {
		t.Fatalf("beta.RequiresModule = %+v, want [%s]", beta, alphaID)
	}

	// Idempotent on a second add.
	report2, refusals2, err := AddEdge(f.dir, EdgeInput{SourceID: betaID, Field: "requires_module", TargetID: alphaID})
	if err != nil {
		t.Fatalf("AddEdge (second): unexpected error: %v", err)
	}
	if len(refusals2) > 0 {
		t.Fatalf("AddEdge (second): unexpected refusals: %+v", refusals2)
	}
	if report2 == nil || len(report2.Written) != 0 {
		t.Fatalf("AddEdge (second): want a no-op, got %+v", report2)
	}

	removeReport, refusals, err := RemoveEdge(f.dir, EdgeInput{SourceID: betaID, Field: "requires_module", TargetID: alphaID})
	if err != nil {
		t.Fatalf("RemoveEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("RemoveEdge: unexpected refusals: %+v", refusals)
	}
	if removeReport == nil || len(removeReport.Written) == 0 {
		t.Fatalf("RemoveEdge: want a write, got %+v", removeReport)
	}

	projData, err = os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	// A fresh Project value: json.Unmarshal only overwrites keys present in
	// the document, so reusing proj here would leave its stale
	// RequiresModule slice in place even though the key is now absent.
	var projAfterRemove schema.Project
	if err := json.Unmarshal(projData, &projAfterRemove); err != nil {
		t.Fatal(err)
	}
	for i := range projAfterRemove.Modules {
		if projAfterRemove.Modules[i].ID == betaID && len(projAfterRemove.Modules[i].RequiresModule) != 0 {
			t.Errorf("beta.RequiresModule should be empty after removal, got %v", projAfterRemove.Modules[i].RequiresModule)
		}
	}
}

// TestEdge_RequiresModule_CycleRefused covers the same cycle check N10
// exercises for a component field, now over the module dependency graph:
// beta already requires alpha, so alpha requiring beta closes a cycle.
func TestEdge_RequiresModule_CycleRefused(t *testing.T) {
	f := buildRenameFixture(t)
	betaID, _ := addSecondModule(t, f, "beta", "Other")
	alphaID := "000000000001"

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: betaID, Field: "requires_module", TargetID: alphaID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if len(refusals) > 0 || report == nil {
		t.Fatalf("AddEdge: want a write, got report=%+v refusals=%+v", report, refusals)
	}

	report, refusals, err = AddEdge(f.dir, EdgeInput{SourceID: alphaID, Field: "requires_module", TargetID: betaID})
	if err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("AddEdge: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 || refusals[0].Check != "dag" {
		t.Fatalf("AddEdge: want exactly one dag refusal, got %+v", refusals)
	}
}

// TestAddEdge_UnknownSourceID_IsInputError covers the input-error outcome:
// a source id naming no node anywhere in the tree.
func TestAddEdge_UnknownSourceID_IsInputError(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: "aaaaaaaaaaaa", Field: "uses", TargetID: f.comp1ID})
	if err == nil {
		t.Fatal("AddEdge: want an input error for an unknown source id")
	}
	if report != nil || refusals != nil {
		t.Fatalf("AddEdge: want no report or refusals on an input error, got report=%+v refusals=%+v", report, refusals)
	}
}

// TestSetEdgeField_CardinalityOne and TestSetEdgeField_CardinalityMany are
// unit tests for setEdgeField's two cardinality branches — the mechanics
// AddEdge's cardinality-one replace and no-op paths build on.
func TestSetEdgeField_CardinalityOne(t *testing.T) {
	field := schema.Field{Name: "preq_id", Cardinality: "one"}

	entry := map[string]any{}
	if changed, replaced := setEdgeField(entry, field, "aaa"); !changed || replaced != "" {
		t.Fatalf("setting an empty cardinality-one field: changed=%v replaced=%q, want true, \"\"", changed, replaced)
	}
	if entry["preq_id"] != "aaa" {
		t.Fatalf("preq_id = %v, want aaa", entry["preq_id"])
	}

	if changed, replaced := setEdgeField(entry, field, "aaa"); changed || replaced != "" {
		t.Fatalf("re-adding the same target: changed=%v replaced=%q, want false, \"\"", changed, replaced)
	}

	if changed, replaced := setEdgeField(entry, field, "bbb"); !changed || replaced != "aaa" {
		t.Fatalf("adding a different target: changed=%v replaced=%q, want true, \"aaa\"", changed, replaced)
	}
	if entry["preq_id"] != "bbb" {
		t.Fatalf("preq_id = %v, want bbb (replaced)", entry["preq_id"])
	}
}

func TestSetEdgeField_CardinalityMany(t *testing.T) {
	field := schema.Field{Name: "uses", Cardinality: "many"}

	entry := map[string]any{}
	if changed, conflict := setEdgeField(entry, field, "aaa"); !changed || conflict != "" {
		t.Fatalf("appending to an empty many field: changed=%v conflict=%q, want true, \"\"", changed, conflict)
	}
	if changed, _ := setEdgeField(entry, field, "aaa"); changed {
		t.Fatal("re-adding the same target should be a no-op")
	}
	if changed, _ := setEdgeField(entry, field, "bbb"); !changed {
		t.Fatal("adding a second target should change the field")
	}
	arr, _ := entry["uses"].([]any)
	if len(arr) != 2 {
		t.Fatalf("uses = %v, want two entries", arr)
	}
}

// TestClearEdgeField covers clearEdgeField's two cardinality branches,
// including that a cardinality-one field is deleted (not emptied) once
// cleared.
func TestClearEdgeField(t *testing.T) {
	oneField := schema.Field{Name: "preq_id", Cardinality: "one"}
	entry := map[string]any{"preq_id": "aaa"}
	if !clearEdgeField(entry, oneField, "aaa") {
		t.Fatal("clearing the held target should report a change")
	}
	if _, ok := entry["preq_id"]; ok {
		t.Errorf("preq_id should be deleted, not emptied, got %v", entry["preq_id"])
	}
	if clearEdgeField(entry, oneField, "aaa") {
		t.Error("clearing an already-absent target should be a no-op")
	}

	manyField := schema.Field{Name: "uses", Cardinality: "many"}
	entry2 := map[string]any{"uses": []any{"aaa", "bbb"}}
	if !clearEdgeField(entry2, manyField, "aaa") {
		t.Fatal("clearing a held target should report a change")
	}
	arr, _ := entry2["uses"].([]any)
	if len(arr) != 1 || arr[0] != "bbb" {
		t.Fatalf("uses = %v, want [bbb]", arr)
	}
	if clearEdgeField(entry2, manyField, "aaa") {
		t.Error("clearing an absent target should be a no-op")
	}
	if !clearEdgeField(entry2, manyField, "bbb") {
		t.Fatal("clearing the last target should report a change")
	}
	if _, ok := entry2["uses"]; ok {
		t.Errorf("uses should be deleted once empty, got %v", entry2["uses"])
	}
}

// TestEdgeField covers edgeField's module-vs-declared-type dispatch:
// requires_module for a module location, a declared reference field for an
// ordinary node, and false for anything undeclared or non-reference.
func TestEdgeField(t *testing.T) {
	moduleLoc := nodeLocation{isModule: true}
	if _, ok := edgeField(moduleLoc, "uses"); ok {
		t.Error("a module should not declare uses")
	}
	f, ok := edgeField(moduleLoc, "requires_module")
	if !ok || f.Cardinality != "many" || f.Targets[0] != "module" {
		t.Errorf("edgeField(module, requires_module) = %+v, %v", f, ok)
	}

	compLoc := nodeLocation{nodeType: schema.NodeType{
		Name: "component",
		Fields: []schema.Field{
			{Name: "uses", Kind: schema.FieldKindReference, Targets: []string{"component"}},
			{Name: "group", Kind: schema.FieldKindText},
		},
	}}
	if _, ok := edgeField(compLoc, "group"); ok {
		t.Error("a non-reference field must not be usable as an edge")
	}
	if _, ok := edgeField(compLoc, "nonexistent"); ok {
		t.Error("an undeclared field must not be usable as an edge")
	}
	if f, ok := edgeField(compLoc, "uses"); !ok || f.Name != "uses" {
		t.Errorf("edgeField(component, uses) = %+v, %v, want the declared uses field", f, ok)
	}
}
