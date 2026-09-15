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

// buildRenameFixture is test_node_editing.md's shared Setup fixture: module
// alpha, project requirements P1/P2, R1 (preq -> P1), Comp1 (implements
// R1), Comp2 (uses Comp1), test section T1 (describes Comp1 and Comp2), api
// "demo run" (provided_by Comp1) — exactly buildObligationFixture's shape,
// with priorities filled in. ObligationReporter's own tests
// (obligation_reporter_test.go) only care about errors a change
// *introduces*, so an already-missing priority never surfaces there; the
// node-editing scenarios assert a fully green `spex validate`, which
// requires every project requirement to carry one.
func buildRenameFixture(t *testing.T) obligationFixture {
	t.Helper()
	f := buildObligationFixture(t)

	path := filepath.Join(f.dir, "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	prio := 2
	for i := range proj.Requirements {
		proj.Requirements[i].Priority = &prio
	}
	writeJSON(t, path, proj)

	return f
}

// TestN8_RenameIsOneTransaction is test_node_editing.md's N8: renaming
// Comp1 to Core rewrites alpha/module.json's own entry, every inbound
// reference (Comp2's uses, T1's describes, the api's provided_by), every
// typed link naming Comp1's id in arch_comp2.md and test_t1.md (display
// text unchanged), and moves arch_comp1.md to arch_core.md — as one
// transaction, with spex validate green afterwards and spex diff reporting
// exactly one removed and one added component with nothing dangling.
func TestN8_RenameIsOneTransaction(t *testing.T) {
	f := buildRenameFixture(t)

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatalf("build before-tree: %v", err)
	}

	report, refusals, err := Rename(f.dir, RenameInput{ID: f.comp1ID, NewName: "Core"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Rename: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Rename: want a report, got nil")
	}

	newID := schema.IdentityHash("alpha", "component", "Core")

	if report.RetiredName != "Comp1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "Comp1")
	}

	// alpha/module.json carries Core with the derived id; Comp2's uses,
	// T1's describes and the api's provided_by carry the new id.
	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}

	var core *schema.Component
	for i := range mod.Components {
		if mod.Components[i].Name == "Comp1" {
			t.Fatalf("Comp1 still present in components: %+v", mod.Components[i])
		}
		if mod.Components[i].ID == newID {
			core = &mod.Components[i]
		}
	}
	if core == nil {
		t.Fatalf("no component with derived id %s found in %+v", newID, mod.Components)
	}
	if core.Name != "Core" || core.Content != "arch_core.md" {
		t.Errorf("renamed entry = %+v, want name Core, content arch_core.md", core)
	}

	var comp2 schema.Component
	for _, c := range mod.Components {
		if c.Name == "Comp2" {
			comp2 = c
		}
	}
	if len(comp2.Uses) != 1 || comp2.Uses[0] != newID {
		t.Errorf("Comp2.Uses = %v, want [%s]", comp2.Uses, newID)
	}

	if len(mod.TestSections) != 1 || len(mod.TestSections[0].Describes) != 2 {
		t.Fatalf("unexpected test sections: %+v", mod.TestSections)
	}
	if !containsStr(mod.TestSections[0].Describes, newID) || containsStr(mod.TestSections[0].Describes, f.comp1ID) {
		t.Errorf("T1.Describes = %v, want to carry %s and not %s", mod.TestSections[0].Describes, newID, f.comp1ID)
	}

	if len(mod.APIs) != 1 || !containsStr(mod.APIs[0].ProvidedBy, newID) || containsStr(mod.APIs[0].ProvidedBy, f.comp1ID) {
		t.Errorf("api.ProvidedBy = %v, want to carry %s and not %s", mod.APIs[0].ProvidedBy, newID, f.comp1ID)
	}

	// The content file moved: arch_core.md exists with arch_comp1.md's
	// content, arch_comp1.md is gone.
	if _, err := os.Stat(filepath.Join(f.dir, "alpha", "arch_comp1.md")); !os.IsNotExist(err) {
		t.Errorf("arch_comp1.md should be gone, stat err = %v", err)
	}
	coreContent, err := os.ReadFile(filepath.Join(f.dir, "alpha", "arch_core.md"))
	if err != nil {
		t.Fatalf("read arch_core.md: %v", err)
	}
	if !strings.Contains(string(coreContent), "Implements [["+f.r1ID+"|R1]]") {
		t.Errorf("arch_core.md lost its original content: %s", coreContent)
	}

	// arch_comp2.md and test_t1.md link the new id with their display text
	// unchanged.
	comp2Content, err := os.ReadFile(filepath.Join(f.dir, "alpha", "arch_comp2.md"))
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	if !strings.Contains(string(comp2Content), "[["+newID+"|Comp1]]") {
		t.Errorf("arch_comp2.md did not repoint its link to %s with display text preserved: %s", newID, comp2Content)
	}
	if strings.Contains(string(comp2Content), f.comp1ID) {
		t.Errorf("arch_comp2.md still names the retired id %s: %s", f.comp1ID, comp2Content)
	}

	t1Content, err := os.ReadFile(filepath.Join(f.dir, "alpha", "test_t1.md"))
	if err != nil {
		t.Fatalf("read test_t1.md: %v", err)
	}
	if !strings.Contains(string(t1Content), "[["+newID+"|Comp1]]") {
		t.Errorf("test_t1.md did not repoint its Comp1 link to %s: %s", newID, t1Content)
	}
	if !strings.Contains(string(t1Content), "[["+f.comp2ID+"|Comp2]]") {
		t.Errorf("test_t1.md should keep its unrelated Comp2 link untouched: %s", t1Content)
	}

	// spex validate is green.
	fsys := os.DirFS(f.dir)
	var allErrs []validator.ValidationError
	allErrs = append(allErrs, validator.CheckSchemaFS(fsys)...)
	allErrs = append(allErrs, validator.CheckIDsFS(fsys)...)
	allErrs = append(allErrs, validator.CheckIDDerivationFS(fsys)...)
	allErrs = append(allErrs, validator.CheckDAGFS(fsys)...)
	allErrs = append(allErrs, validator.CheckLinksFS(fsys)...)
	if len(allErrs) > 0 {
		t.Errorf("spec is not green after rename: %+v", allErrs)
	}

	// spex diff reports exactly one removed and one added component, and
	// nothing dangling (asserted above via the empty validator error set).
	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatalf("build after-tree: %v", err)
	}
	changes := merkle.Diff(afterTree, beforeTree)

	var removedComponents, addedComponents []merkle.Change
	for _, c := range changes {
		if c.NodeType != "component" {
			continue
		}
		switch c.Type {
		case merkle.Removed:
			removedComponents = append(removedComponents, c)
		case merkle.Added:
			addedComponents = append(addedComponents, c)
		}
	}
	if len(removedComponents) != 1 || removedComponents[0].Key != f.comp1ID {
		t.Errorf("removed components = %+v, want exactly one with key %s", removedComponents, f.comp1ID)
	}
	if len(addedComponents) != 1 || addedComponents[0].Key != newID {
		t.Errorf("added components = %+v, want exactly one with key %s", addedComponents, newID)
	}
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// TestRename_NoOpWhenNameUnchanged covers arch_node_renamer.md's "What it
// refuses": "a new name equal to the old is a no-op that says so" — not a
// refusal, and nothing is written.
func TestRename_NoOpWhenUnchanged(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read fixture module.json: %v", err)
	}

	report, refusals, err := Rename(f.dir, RenameInput{ID: f.comp1ID, NewName: "Comp1"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Rename: want no refusal for a no-op, got %+v", refusals)
	}
	if report == nil {
		t.Fatal("Rename: want a report for a no-op, got nil")
	}
	if len(report.Written) != 0 {
		t.Errorf("Written = %v, want none for a no-op", report.Written)
	}
	if report.RetiredName != "" {
		t.Errorf("RetiredName = %q, want empty for a no-op (nothing was retired)", report.RetiredName)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read module.json after no-op: %v", err)
	}
	if string(before) != string(after) {
		t.Error("a no-op rename must leave the tree byte-identical")
	}
}

// TestRename_RefusesModuleID covers "a module id is refused too" — no
// validator check exists for this, so it is NodeRenamer's own guard.
func TestRename_RefusesModuleID(t *testing.T) {
	f := buildRenameFixture(t)
	before, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatalf("read fixture project.json: %v", err)
	}

	report, refusals, err := Rename(f.dir, RenameInput{ID: "000000000001", NewName: "beta"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Rename: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Rename: want exactly one refusal, got %+v", refusals)
	}
	if refusals[0].Fix == "" {
		t.Error("a module-id refusal must carry a fix")
	}
	if !strings.Contains(refusals[0].Fix, "project.json") {
		t.Errorf("fix should name the project.json edit a module rename needs, got: %s", refusals[0].Fix)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatalf("read project.json after refusal: %v", err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}
}

// TestRename_RefusesNameCollision covers "a new name that collides with an
// existing node of the same type and scope is refused with the existing
// node's id as the fix" — surfaced through Report's own "id" check
// (duplicate ID), never re-implemented by NodeRenamer. Renaming Comp2 to
// Comp1 in this fixture collides two ways at once (Comp2 already uses
// Comp1, so the same edit also folds a "uses" edge into a self-cycle) —
// realistic fallout of forcing a collision on a connected node, not a
// defect — so this only asserts that the duplicate-id refusal Report finds
// is among them, not that it is the only one.
func TestRename_RefusesNameCollision(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Rename(f.dir, RenameInput{ID: f.comp2ID, NewName: "Comp1"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Rename: want no report on refusal, got %+v", report)
	}
	if len(refusals) == 0 {
		t.Fatal("Rename: want at least one refusal for a colliding rename")
	}

	var dup *RefusalEntry
	for i, r := range refusals {
		if r.Check == "id" && strings.Contains(r.Message, "duplicate ID") {
			dup = &refusals[i]
		}
	}
	if dup == nil {
		t.Fatalf("want a duplicate-id refusal among %+v", refusals)
	}
	if !strings.Contains(dup.Fix, f.comp1ID) {
		t.Errorf("fix should name the colliding node's id %s, got: %s", f.comp1ID, dup.Fix)
	}
}

// TestRename_RefusesUndeclarableName covers "a name the tokenizer would not
// reproduce is refused with the form it would reproduce" — the same
// declarability rule checkNameRecoverability applies at declaration time,
// surfaced through Report rather than re-implemented here.
func TestRename_RefusesUndeclarableName(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Rename(f.dir, RenameInput{ID: f.comp2ID, NewName: "Widget [--json]"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Rename: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Rename: want exactly one refusal, got %+v", refusals)
	}
	if refusals[0].Check != "id" {
		t.Errorf("Check = %q, want %q", refusals[0].Check, "id")
	}
	if !strings.Contains(refusals[0].Fix, "declare it as") {
		t.Errorf("fix should show the declarable form, got: %s", refusals[0].Fix)
	}
}

// TestRename_RewritesSingleValuedReferenceField covers preq_id: a
// cardinality-"one" reference field, stored as a bare string rather than
// an array, which patchReferenceFields must rewrite the same way it
// rewrites a many-valued field like uses/describes/provided_by (already
// exercised by TestN8 above).
func TestRename_RewritesSingleValuedReferenceField(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Rename(f.dir, RenameInput{ID: f.p1ID, NewName: "P1 Renamed"})
	if err != nil {
		t.Fatalf("Rename: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Rename: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Rename: want a report, got nil")
	}

	newID := schema.IdentityHash("project", "requirement", "P1 Renamed")
	if report.RetiredName != "P1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "P1")
	}

	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	found := false
	for _, r := range proj.Requirements {
		if r.ID == newID {
			found = true
			if r.Title != "P1 Renamed" {
				t.Errorf("renamed requirement name = %q, want %q", r.Title, "P1 Renamed")
			}
		}
		if r.ID == f.p1ID {
			t.Errorf("old requirement id %s still present", f.p1ID)
		}
	}
	if !found {
		t.Fatalf("no project requirement with derived id %s found in %+v", newID, proj.Requirements)
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}
	if len(mod.Requirements) != 1 || mod.Requirements[0].PreqID != newID {
		t.Errorf("R1.PreqID = %+v, want %s", mod.Requirements, newID)
	}
}

// TestRename_UnknownIDIsAnInputError covers the case arch_node_renamer.md
// does not name explicitly but every worker must handle: an id that
// resolves to no node in the tree is an input error, not a refusal — no
// change was ever attempted for Report to judge.
func TestRename_UnknownIDIsAnInputError(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Rename(f.dir, RenameInput{ID: "abcdef123456", NewName: "Anything"})
	if err == nil {
		t.Fatal("Rename: want an error for an unknown id")
	}
	if report != nil || refusals != nil {
		t.Errorf("Rename: want no report and no refusals on an input error, got report=%+v refusals=%+v", report, refusals)
	}
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Core", "core"},
		{"New rule", "new_rule"},
		{"  Multi   Space ", "multi_space"},
		{"Already_snake", "already_snake"},
		{"Widget [--json]", "widget_json"},
		{"Mixed123Test", "mixed123test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := snakeCase(tt.name); got != tt.want {
				t.Errorf("snakeCase(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
