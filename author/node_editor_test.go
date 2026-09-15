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

// allValidatorErrors runs every checker `spex validate` runs — the ten
// checkers, not just the five Report gates a write on — mirroring
// TestN8_RenameIsOneTransaction's own green-tree assertion in
// node_renamer_test.go.
func allValidatorErrors(t *testing.T, dir string) []validator.ValidationError {
	t.Helper()
	fsys := os.DirFS(dir)
	var errs []validator.ValidationError
	errs = append(errs, validator.CheckSchemaFS(fsys)...)
	errs = append(errs, validator.CheckIDsFS(fsys)...)
	errs = append(errs, validator.CheckIDDerivationFS(fsys)...)
	errs = append(errs, validator.CheckDAGFS(fsys)...)
	errs = append(errs, validator.CheckLinksFS(fsys)...)
	errs = append(errs, validator.CheckContentPathsFS(fsys)...)
	errs = append(errs, validator.CheckNameConsistencyFS(fsys)...)
	errs = append(errs, validator.CheckTestCoverageFS(fsys)...)
	reqErrs, _ := validator.CheckRequirementCoverageFS(fsys)
	errs = append(errs, reqErrs...)
	errs = append(errs, validator.CheckCoupledSectionsFS(fsys)...)
	return errs
}

// TestN1_AddingComponent_PlacesDerivesScaffolds covers test_node_editing.md's
// N1: adding a component to an existing module places it under the type's
// plural key, derives its id the same way spex hash-id would, sets the
// conventional content path and scaffolds the leaf, leaves spex validate
// green, and spex diff reports exactly one added component plus module
// alpha's own meta change.
func TestN1_AddingComponent_PlacesDerivesScaffolds(t *testing.T) {
	f := buildRenameFixture(t)

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatalf("build before-tree: %v", err)
	}

	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "component", Module: "alpha", Name: "Widget"})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Add: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Add: want a report, got nil")
	}

	widgetID := schema.IdentityHash("alpha", "component", "Widget")

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}
	var widget *schema.Component
	for i := range mod.Components {
		if mod.Components[i].ID == widgetID {
			widget = &mod.Components[i]
		}
	}
	if widget == nil {
		t.Fatalf("no component with derived id %s found in %+v", widgetID, mod.Components)
	}
	if widget.Name != "Widget" || widget.Content != "arch_widget.md" {
		t.Errorf("widget entry = %+v, want name Widget, content arch_widget.md", widget)
	}

	content, err := os.ReadFile(filepath.Join(f.dir, "alpha", "arch_widget.md"))
	if err != nil {
		t.Fatalf("read arch_widget.md: %v", err)
	}
	if !strings.HasPrefix(string(content), "# Widget") {
		t.Errorf("arch_widget.md = %q, want to open with '# Widget'", content)
	}

	// NOTE on divergence from test_node_editing.md's N1 prose: the scenario
	// as written claims "spex validate is green" after adding Widget. Read
	// literally, that cannot hold under the documented mechanism a fresh,
	// undescribed component always trips the default profile's
	// component-describes-test_section coverage chain
	// (validator/test_coverage_checker.go), and nothing about a plain
	// "spex node add component" supplies a describer — the fixture's own T1
	// only describes Comp1 and Comp2. Reproducing the scenario's literal
	// claim would require NodeEditor to also wire a test_section edge no
	// input of this command names, which arch_node_editor.md's "Adding a
	// node" table never lists among what NodeEditor decides. This is filed
	// as drifts/drift-spexmachina-yih0.10.json rather than silently
	// special-cased here; the assertion below is what the documented
	// mechanism actually produces: the coverage gap surfaces as an
	// obligation (test_coverage is a nonRefusalChecker), never a refusal,
	// and it is the only finding.
	errs := allValidatorErrors(t, f.dir)
	if len(errs) != 1 || errs[0].Check != "test_coverage" {
		t.Errorf("want exactly the new component's own test_coverage gap, got %+v", errs)
	}
	testCoverageObligations := obligationsOfType(report.Obligations, "test_coverage")
	if len(testCoverageObligations) != 1 {
		t.Errorf("want the test_coverage gap to travel as an obligation, got %+v", report.Obligations)
	}

	metaObligations := obligationsOfType(report.Obligations, "incomplete_change")
	keys := obligationKeys(t, metaObligations)
	for _, comp := range []string{"Comp1", "Comp2"} {
		if !containsSubstring(keys, comp) {
			t.Errorf("want a meta obligation naming %s, got %+v", comp, keys)
		}
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatalf("build after-tree: %v", err)
	}
	changes := merkle.Diff(afterTree, beforeTree)

	var addedComponents, metaChanges []merkle.Change
	for _, c := range changes {
		switch {
		case c.NodeType == "component" && c.Type == merkle.Added:
			addedComponents = append(addedComponents, c)
		case c.NodeType == "meta" && c.Module == "000000000001":
			metaChanges = append(metaChanges, c)
		}
	}
	if len(addedComponents) != 1 || addedComponents[0].Key != widgetID {
		t.Errorf("added components = %+v, want exactly one with key %s", addedComponents, widgetID)
	}
	if len(metaChanges) == 0 {
		t.Error("want module alpha's meta leaf to show up as changed")
	}
}

// TestN2_ProjectScopedTypeLandsInProjectJSON covers N2: a project-scoped
// requirement lands in project.json's own requirements array, not any
// module.json, and the validator's one finding for an underived project
// requirement travels as an obligation, never a refusal.
func TestN2_ProjectScopedTypeLandsInProjectJSON(t *testing.T) {
	f := buildRenameFixture(t)

	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Add(f.dir, NodeAddInput{
		TypeName: "requirement",
		Name:     "New rule",
		Fields:   map[string]string{"type": "functional", "priority": "2"},
	})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Add: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Add: want a report, got nil")
	}

	newID := schema.IdentityHash("project", "requirement", "New rule")
	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatal(err)
	}
	var newReq *schema.Requirement
	for i := range proj.Requirements {
		if proj.Requirements[i].ID == newID {
			newReq = &proj.Requirements[i]
		}
	}
	if newReq == nil {
		t.Fatalf("no requirement with derived id %s found in %+v", newID, proj.Requirements)
	}
	if newReq.Priority == nil || *newReq.Priority != 2 {
		t.Errorf("priority = %v, want 2", newReq.Priority)
	}

	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("alpha/module.json must be untouched by a project-scoped add")
	}

	reqCoverage := obligationsOfType(report.Obligations, "requirement_coverage")
	reqKeys := obligationKeys(t, reqCoverage)
	if !containsSubstring(reqKeys, "New rule") {
		t.Errorf("want a requirement_coverage obligation naming 'New rule', got %+v", report.Obligations)
	}
}

// TestN3_ModuleRegisteredAndSkeletoned covers N3: --type module registers
// the module in project.json and writes its module.json skeleton in one
// command, leaving spex validate green.
func TestN3_ModuleRegisteredAndSkeletoned(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "module", Name: "beta"})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Add: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Add: want a report, got nil")
	}

	betaID := schema.IdentityHash("module", "beta")
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
		if proj.Modules[i].Name == "beta" {
			beta = &proj.Modules[i]
		}
	}
	if beta == nil {
		t.Fatalf("no module named beta found in %+v", proj.Modules)
	}
	if beta.ID != betaID || beta.Path != "beta" {
		t.Errorf("beta module = %+v, want id %s, path beta", beta, betaID)
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "beta", "module.json"))
	if err != nil {
		t.Fatalf("read beta/module.json: %v", err)
	}
	if string(modData) != "{\n  \"name\": \"beta\"\n}\n" {
		t.Errorf("beta/module.json = %q, want exactly {\"name\": \"beta\"} in canonical two-space form", modData)
	}
	var betaDoc map[string]json.RawMessage
	if err := json.Unmarshal(modData, &betaDoc); err != nil {
		t.Fatal(err)
	}
	if len(betaDoc) != 1 {
		t.Errorf("beta/module.json declares %v, want name and nothing else", betaDoc)
	}

	if errs := allValidatorErrors(t, f.dir); len(errs) > 0 {
		t.Errorf("spec is not green after adding an empty module: %+v", errs)
	}
}

// TestN4_UndeclaredTypeRefused covers N4: a type the resolved profile does
// not declare is refused with the declared list as the fix, never a fixed
// list compiled into NodeEditor — the same run over a fixture whose profile
// declares the type succeeds.
func TestN4_UndeclaredTypeRefused(t *testing.T) {
	f := buildRenameFixture(t)

	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "impl_section", Module: "alpha", Name: "Anything"})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Add: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Add: want exactly one refusal, got %+v", refusals)
	}
	for _, want := range []string{"requirement", "component", "data_flow", "test_section", "api", "module"} {
		if !strings.Contains(refusals[0].Fix, want) {
			t.Errorf("fix should list declared type %q, got: %s", want, refusals[0].Fix)
		}
	}
	if !strings.Contains(refusals[0].Message, "impl_section") {
		t.Errorf("message should name the undeclared type, got: %s", refusals[0].Message)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity: the same run over a fixture whose profile declares
	// impl_section succeeds.
	custom := schema.DefaultProfile()
	custom.NodeTypes = append(append([]schema.NodeType{}, custom.NodeTypes...), schema.NodeType{
		Name:      "impl_section",
		PluralKey: "impl_sections",
		Scope:     "module",
	})
	writeJSON(t, filepath.Join(f.dir, "profile.json"), custom)

	report, refusals, err = Add(f.dir, NodeAddInput{TypeName: "impl_section", Module: "alpha", Name: "Anything"})
	if err != nil {
		t.Fatalf("Add: unexpected error over a profile declaring impl_section: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Add: unexpected refusals over a profile declaring impl_section: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Add: want a report, got nil")
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(modData, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["impl_sections"]; !ok {
		t.Errorf("alpha/module.json should now carry an impl_sections array, got %s", modData)
	}
}

// TestN5_UndeclarableName_RefusedByReport covers N5: a name the
// declarability tokenizer would not reproduce is refused with the
// declarable form as the fix, surfaced through Report's own "id" check
// rather than re-implemented by NodeEditor.
func TestN5_UndeclarableName_RefusedByReport(t *testing.T) {
	f := buildRenameFixture(t)

	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "api", Module: "alpha", Name: "demo run [--json]"})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Add: want no report on refusal, got %+v", report)
	}
	if len(refusals) == 0 {
		t.Fatal("Add: want at least one refusal for an undeclarable name")
	}

	var found *RefusalEntry
	for i, r := range refusals {
		if r.Check == "id" && strings.Contains(r.Fix, "declare it as") {
			found = &refusals[i]
		}
	}
	if found == nil {
		t.Fatalf("want a refusal naming the declarable form, got %+v", refusals)
	}
	if !strings.Contains(found.Fix, "demo run --json") {
		t.Errorf("fix should show the declarable form 'demo run --json', got: %s", found.Fix)
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}
}

// TestN6_RemovingReferencedNode_RefusesListingInboundRefs covers N6:
// removing Comp1 while Comp2's uses, T1's describes, the api's provided_by
// and the typed links in arch_comp2.md and test_t1.md still name it is
// refused, nothing written, each inbound reference paired with the
// spex edge remove invocation that would retarget it.
func TestN6_RemovingReferencedNode_RefusesListingInboundRefs(t *testing.T) {
	f := buildRenameFixture(t)

	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeLeaf, err := os.ReadFile(filepath.Join(f.dir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Remove(f.dir, NodeRemoveInput{ID: f.comp1ID})
	if err != nil {
		t.Fatalf("Remove: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Remove: want no report on refusal, got %+v", report)
	}
	if len(refusals) == 0 {
		t.Fatal("Remove: want at least one refusal for a referenced node")
	}

	var idRefusals, linkRefusals []RefusalEntry
	for _, r := range refusals {
		switch r.Check {
		case "id":
			idRefusals = append(idRefusals, r)
		case "link":
			linkRefusals = append(linkRefusals, r)
		}
	}
	if len(idRefusals) < 3 {
		t.Errorf("want at least 3 'id' refusals (Comp2's uses, T1's describes, the api's provided_by), got %+v", idRefusals)
	}
	for _, r := range idRefusals {
		if !strings.Contains(r.Fix, "spex edge remove") || !strings.Contains(r.Fix, "--force") {
			t.Errorf("want the fix to name spex edge remove and --force, got: %s", r.Fix)
		}
	}
	if len(linkRefusals) < 2 {
		t.Errorf("want at least 2 'link' refusals (arch_comp2.md, test_t1.md), got %+v", linkRefusals)
	}

	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a refusal must leave module.json byte-identical")
	}
	afterLeaf, err := os.ReadFile(filepath.Join(f.dir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatalf("arch_comp1.md must survive a refused removal: %v", err)
	}
	if string(beforeLeaf) != string(afterLeaf) {
		t.Error("a refusal must leave arch_comp1.md byte-identical")
	}
}

// TestN7_ForcedRemoval_ListsWhatItLeftDangling covers N7: --force removes
// Comp1's entry and its leaf despite the still-live inbound references N6
// found, reports the same findings instead of refusing on them, prints the
// retired name for the vocabulary sweep, and spex diff reports exactly one
// removed component.
func TestN7_ForcedRemoval_ListsWhatItLeftDangling(t *testing.T) {
	f := buildRenameFixture(t)

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Remove(f.dir, NodeRemoveInput{ID: f.comp1ID, Force: true})
	if err != nil {
		t.Fatalf("Remove: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Remove: unexpected refusals on a forced removal: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Remove: want a report, got nil")
	}
	if report.RetiredName != "Comp1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "Comp1")
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	for _, c := range mod.Components {
		if c.ID == f.comp1ID {
			t.Fatalf("Comp1 still present in components: %+v", c)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "alpha", "arch_comp1.md")); !os.IsNotExist(err) {
		t.Errorf("arch_comp1.md should be gone, stat err = %v", err)
	}

	// The same inbound references N6 found are printed as dangling
	// obligations, and spex validate now reports exactly that set (the
	// fixture was fully green before the forced removal).
	danglingKeys := obligationKeys(t, report.Obligations)
	if !containsSubstring(danglingKeys, f.comp1ID) {
		t.Fatalf("want the dangling obligations to name %s, got %+v", f.comp1ID, danglingKeys)
	}

	errs := allValidatorErrors(t, f.dir)
	if len(errs) == 0 {
		t.Fatal("want spex validate to report the dangling references left by a forced removal")
	}
	for _, e := range errs {
		if !containsSubstring(danglingKeys, e.Message) {
			t.Errorf("validate finding %+v not among the obligations the command printed: %+v", e, danglingKeys)
		}
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	changes := merkle.Diff(afterTree, beforeTree)
	var removedComponents []merkle.Change
	for _, c := range changes {
		if c.NodeType == "component" && c.Type == merkle.Removed {
			removedComponents = append(removedComponents, c)
		}
	}
	if len(removedComponents) != 1 || removedComponents[0].Key != f.comp1ID {
		t.Errorf("removed components = %+v, want exactly one with key %s", removedComponents, f.comp1ID)
	}
}

// TestN14_NoInitialisedProjectNeeded covers N14: spex node add produces a
// byte-identical spec/ tree whether or not the project carries an
// initialised, healthy .spex/ — and leaves that .spex/ byte-identical too,
// since NodeEditor reads and writes nothing under it.
func TestN14_NoInitialisedProjectNeeded(t *testing.T) {
	plain := buildRenameFixture(t)

	initialised := buildRenameFixture(t)
	spexDir := filepath.Join(initialised.dir, ".spex")
	if err := os.MkdirAll(spexDir, 0755); err != nil {
		t.Fatal(err)
	}
	snapshotContent := `{"fake":"snapshot"}`
	writeFile(t, filepath.Join(spexDir, "snapshot.json"), snapshotContent)
	beforeSpex, err := os.ReadFile(filepath.Join(spexDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, refusals, err := Add(plain.dir, NodeAddInput{TypeName: "component", Module: "alpha", Name: "Widget"}); err != nil {
		t.Fatalf("Add (plain): unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("Add (plain): unexpected refusals: %+v", refusals)
	}
	if _, refusals, err := Add(initialised.dir, NodeAddInput{TypeName: "component", Module: "alpha", Name: "Widget"}); err != nil {
		t.Fatalf("Add (initialised): unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("Add (initialised): unexpected refusals: %+v", refusals)
	}

	plainMod, err := os.ReadFile(filepath.Join(plain.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	initMod, err := os.ReadFile(filepath.Join(initialised.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plainMod) != string(initMod) {
		t.Error("spec/ tree diverged depending on whether .spex/ was initialised")
	}

	afterSpex, err := os.ReadFile(filepath.Join(spexDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeSpex) != string(afterSpex) {
		t.Error(".spex/ must be byte-identical before and after: NodeEditor reads and writes nothing under it")
	}
}

// TestResolveAddNodeType_ModuleScopedRequiresModule covers the input-error
// path: a module-scoped-only type named with no --module is neither a
// refusal (the type IS declared) nor silently defaulted to project scope.
func TestResolveAddNodeType_ModuleScopedRequiresModule(t *testing.T) {
	profile := schema.DefaultProfile()
	_, moduleRequired, ok := resolveAddNodeType(profile, "component", "")
	if !ok {
		t.Fatal("component must be a declared type")
	}
	if !moduleRequired {
		t.Error("want moduleRequired for a module-scoped-only type with no module given")
	}
}

func TestAdd_ModuleScopedTypeWithNoModule_IsInputError(t *testing.T) {
	f := buildRenameFixture(t)
	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "component", Name: "Widget"})
	if err == nil {
		t.Fatal("Add: want an input error when a module-scoped type is named with no module")
	}
	if report != nil || refusals != nil {
		t.Errorf("Add: want no report and no refusals on an input error, got report=%+v refusals=%+v", report, refusals)
	}
}

func TestAdd_UnknownModule_IsInputError(t *testing.T) {
	f := buildRenameFixture(t)
	report, refusals, err := Add(f.dir, NodeAddInput{TypeName: "component", Module: "nosuch", Name: "Widget"})
	if err == nil {
		t.Fatal("Add: want an input error for an unknown module")
	}
	if report != nil || refusals != nil {
		t.Errorf("Add: want no report and no refusals on an input error, got report=%+v refusals=%+v", report, refusals)
	}
}

func TestRemove_UnknownIDIsAnInputError(t *testing.T) {
	f := buildRenameFixture(t)
	report, refusals, err := Remove(f.dir, NodeRemoveInput{ID: "abcdef123456"})
	if err == nil {
		t.Fatal("Remove: want an error for an unknown id")
	}
	if report != nil || refusals != nil {
		t.Errorf("Remove: want no report and no refusals on an input error, got report=%+v refusals=%+v", report, refusals)
	}
}

// TestRemove_RefusesModuleID covers "a module id is refused, forced or
// not" — checked with force too, since arch_node_editor.md makes a point of
// it applying regardless.
func TestRemove_RefusesModuleID(t *testing.T) {
	f := buildRenameFixture(t)
	for _, force := range []bool{false, true} {
		report, refusals, err := Remove(f.dir, NodeRemoveInput{ID: "000000000001", Force: force})
		if err != nil {
			t.Fatalf("Remove(force=%v): unexpected error: %v", force, err)
		}
		if report != nil {
			t.Fatalf("Remove(force=%v): want no report on refusal, got %+v", force, report)
		}
		if len(refusals) != 1 {
			t.Fatalf("Remove(force=%v): want exactly one refusal, got %+v", force, refusals)
		}
		if !strings.Contains(refusals[0].Fix, "project.json") {
			t.Errorf("Remove(force=%v): fix should name the project.json edit, got: %s", force, refusals[0].Fix)
		}
	}
}

func TestConvertFieldValue(t *testing.T) {
	textField := schema.Field{Name: "group", Kind: schema.FieldKindText}
	if v, err := convertFieldValue(textField, "cli"); err != nil || v != "cli" {
		t.Errorf("text field: got (%v, %v), want (cli, nil)", v, err)
	}

	intField := schema.Field{Name: "priority", Kind: schema.FieldKindInteger}
	if v, err := convertFieldValue(intField, "2"); err != nil || v != 2 {
		t.Errorf("integer field: got (%v, %v), want (2, nil)", v, err)
	}
	if _, err := convertFieldValue(intField, "abc"); err == nil {
		t.Error("integer field: want an error for a non-integer value")
	}

	oneRefField := schema.Field{Name: "preq_id", Kind: schema.FieldKindReference, Cardinality: "one"}
	if v, err := convertFieldValue(oneRefField, "abcdef123456"); err != nil || v != "abcdef123456" {
		t.Errorf("cardinality-one reference: got (%v, %v), want (abcdef123456, nil)", v, err)
	}

	manyRefField := schema.Field{Name: "depends_on", Kind: schema.FieldKindReference, Cardinality: "many"}
	v, err := convertFieldValue(manyRefField, "abc111111111, abc222222222 ,")
	if err != nil {
		t.Fatalf("cardinality-many reference: unexpected error: %v", err)
	}
	got, ok := v.([]string)
	if !ok || len(got) != 2 || got[0] != "abc111111111" || got[1] != "abc222222222" {
		t.Errorf("cardinality-many reference: got %#v, want [abc111111111 abc222222222]", v)
	}
}

// TestAdd_MissingRequiredField_RefusedByReport covers "requires every field
// the profile marks required": preq_id is required on a module-scoped
// requirement, and omitting it is refused through Report's own schema
// check, naming the field and its kind.
func TestAdd_MissingRequiredField_RefusedByReport(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Add(f.dir, NodeAddInput{
		TypeName: "requirement", Module: "alpha", Name: "R2",
		Fields: map[string]string{"type": "functional"},
	})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Add: want no report on refusal, got %+v", report)
	}
	if len(refusals) == 0 {
		t.Fatal("Add: want a refusal for the missing required preq_id")
	}
	var found bool
	for _, r := range refusals {
		if r.Check == "schema" && strings.Contains(r.Fix, "preq_id") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a refusal naming preq_id as the missing required field, got %+v", refusals)
	}
}

// TestAdd_UndeclaredField_RefusedByReport covers "a field the type does not
// declare is refused with the type's fields as the fix": NodeEditor names
// no field of its own, so an unrecognized field name reaches Report's
// schema check as an additional property.
func TestAdd_UndeclaredField_RefusedByReport(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Add(f.dir, NodeAddInput{
		TypeName: "component", Module: "alpha", Name: "Widget",
		Fields: map[string]string{"reviewed_by": "someone"},
	})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Add: want no report on refusal, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Add: want exactly one refusal, got %+v", refusals)
	}
	if refusals[0].Check != "schema" {
		t.Errorf("Check = %q, want %q", refusals[0].Check, "schema")
	}
	for _, want := range []string{"id", "name", "content", "implements", "uses"} {
		if !strings.Contains(refusals[0].Fix, want) {
			t.Errorf("fix should list declared field %q, got: %s", want, refusals[0].Fix)
		}
	}
}

func TestFindField(t *testing.T) {
	nt := schema.NodeType{Fields: []schema.Field{{Name: "implements"}, {Name: "uses"}}}
	if _, ok := findField(nt, "implements"); !ok {
		t.Error("want implements to be found")
	}
	if _, ok := findField(nt, "reviewed_by"); ok {
		t.Error("want reviewed_by not to be found")
	}
}

func TestDeclaredTypeNames(t *testing.T) {
	names := declaredTypeNames(schema.DefaultProfile())
	want := []string{"requirement", "component", "data_flow", "test_section", "api", "module"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}
