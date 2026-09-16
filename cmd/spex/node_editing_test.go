package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/author"
	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// This file is cmd/spex's half of spec/author/test_node_editing.md, the
// "Node editing tests" test section (aa4656487ccd) — N1-N21, run over the
// real `spex node add|set|remove|rename` and `spex edge add|remove` command
// trees. author/node_editor_test.go, author/node_editor_set_test.go,
// author/node_renamer_test.go and author/edge_editor_test.go already
// exercise NodeEditor, NodeRenamer and EdgeEditor directly
// (Add/Set/Remove/Rename/AddEdge/RemoveEdge) under the same N1-N21 names;
// this file is their CLI-level mirror, the same relationship
// leaf_and_profile_test.go has to author/leaf_scaffolder_test.go.

// runNodeEditingSpex assembles the node, edge, diff, validate, hash-id and
// render command trees — every surface a Node editing scenario drives.
func runNodeEditingSpex(t *testing.T, args ...string) (stdout string, execErr error) {
	t.Helper()
	root := cli.NewRootCmd()
	root.AddCommand(newNodeCmd(), newEdgeCmd(), newDiffCmd(), newValidateCmd(), newHashIDCmd(), newRenderCmd())

	errBuf := new(bytes.Buffer)
	root.SetErr(errBuf)
	root.SetArgs(args)

	stdout = captureStdout(t, func() {
		execErr = root.Execute()
	})
	return stdout, execErr
}

// nodeEditingFixture is test_node_editing.md's shared Setup fixture: module
// alpha, project requirements P1/P2, R1 (preq -> P1), Comp1 (implements
// R1), Comp2 (uses Comp1), test section T1 (describes Comp1 and Comp2), api
// "demo run" (provided_by Comp1) — the same shape author's own
// buildObligationFixture/buildRenameFixture build, reconstructed here from
// schema's exported types since this package cannot reach those unexported
// test helpers.
type nodeEditingFixture struct {
	dir              string
	p1ID, p2ID, r1ID string
	comp1ID, comp2ID string
	t1ID, apiID      string
}

func buildNodeEditingFixture(t *testing.T, dir string) nodeEditingFixture {
	t.Helper()

	prio := 2
	f := nodeEditingFixture{
		dir:     dir,
		p1ID:    schema.IdentityHash("project", "requirement", "P1"),
		p2ID:    schema.IdentityHash("project", "requirement", "P2"),
		r1ID:    schema.IdentityHash("alpha", "requirement", "R1"),
		comp1ID: schema.IdentityHash("alpha", "component", "Comp1"),
		comp2ID: schema.IdentityHash("alpha", "component", "Comp2"),
		t1ID:    schema.IdentityHash("alpha", "test_section", "T1"),
		apiID:   schema.IdentityHash("alpha", "api", "demo run"),
	}

	proj := schema.Project{
		Name: "test-project",
		Requirements: []schema.Requirement{
			{ID: f.p1ID, Type: "functional", Title: "P1", Priority: &prio},
			{ID: f.p2ID, Type: "functional", Title: "P2", Priority: &prio, Derivation: "pending"},
		},
		Modules: []schema.Module{
			{ID: "000000000001", Name: "alpha", Path: "alpha"},
		},
	}
	writeAuthorJSON(t, filepath.Join(dir, "project.json"), proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}

	mod := schema.ModuleSpec{
		Name: "alpha",
		Requirements: []schema.ModuleRequirement{
			{ID: f.r1ID, PreqID: f.p1ID, Type: "functional", Title: "R1"},
		},
		Components: []schema.Component{
			{ID: f.comp1ID, Name: "Comp1", Content: "arch_comp1.md", Implements: []string{f.r1ID}},
			{ID: f.comp2ID, Name: "Comp2", Content: "arch_comp2.md", Uses: []string{f.comp1ID}},
		},
		TestSections: []schema.TestSection{
			{ID: f.t1ID, Name: "T1", Content: "test_t1.md", Describes: []string{f.comp1ID, f.comp2ID}},
		},
		APIs: []schema.API{
			{ID: f.apiID, Name: "demo run", ProvidedBy: []string{f.comp1ID}},
		},
	}
	writeAuthorJSON(t, filepath.Join(alphaDir, "module.json"), mod)

	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n\nImplements [["+f.r1ID+"|R1]].\n")
	writeTestFile(t, alphaDir, "arch_comp2.md", "# Comp2\n\nUses [["+f.comp1ID+"|Comp1]].\n")
	writeTestFile(t, alphaDir, "test_t1.md", "# T1\n\nDescribes [["+f.comp1ID+"|Comp1]] and [["+f.comp2ID+"|Comp2]].\n")

	return f
}

// decodeWriteReport decodes stdout as an author.WriteReport, the shape
// every accepted write prints.
func decodeWriteReport(t *testing.T, out string) author.WriteReport {
	t.Helper()
	var report author.WriteReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stdout is not a write report: %v\n%s", err, out)
	}
	return report
}

// decodeRefusals decodes stdout as a bare []author.RefusalEntry, the shape
// every refused write prints.
func decodeRefusals(t *testing.T, out string) []author.RefusalEntry {
	t.Helper()
	var refusals []author.RefusalEntry
	if err := json.Unmarshal([]byte(out), &refusals); err != nil {
		t.Fatalf("stdout is not a refusal document: %v\n%s", err, out)
	}
	return refusals
}

// decodeDiffJSON decodes stdout as diff.go's own diffOutput shape.
func decodeDiffJSON(t *testing.T, out string) diffOutput {
	t.Helper()
	var doc diffOutput
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not a diff document: %v\n%s", err, out)
	}
	return doc
}

// decodeValidationReport decodes stdout as a validator.ValidationReport.
func decodeValidationReport(t *testing.T, out string) validator.ValidationReport {
	t.Helper()
	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stdout is not a validation report: %v\n%s", err, out)
	}
	return report
}

// canonicalObjectKeys reads back a canonically-written file's own key order
// for the array-entry object whose "id" value is idValue, by indentation
// rather than by decoding into a Go struct (which would discard the order
// entirely): every key line at the same indent as the "id" line, until
// indentation drops back out of the object. It relies on nothing but the
// two-space, one-key-per-line shape canonicalizeDoc's json.Encoder produces.
func canonicalObjectKeys(t *testing.T, data []byte, idValue string) []string {
	t.Helper()
	lines := strings.Split(string(data), "\n")

	target := -1
	indent := 0
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, `"id": "`+idValue+`"`) {
			target = i
			indent = len(line) - len(trimmed)
			break
		}
	}
	if target == -1 {
		t.Fatalf("no \"id\": %q line found in:\n%s", idValue, data)
	}

	var keys []string
	for i := target; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimLeft(line, " ")
		curIndent := len(line) - len(trimmed)
		if curIndent < indent {
			break
		}
		if curIndent != indent || !strings.HasPrefix(trimmed, `"`) {
			continue
		}
		end := strings.Index(trimmed[1:], `"`)
		if end == -1 {
			continue
		}
		keys = append(keys, trimmed[1:1+end])
	}
	return keys
}

// hashID runs `spex hash-id` and returns the printed identity hash, trimmed
// of its trailing newline — the same oracle N1 and N8 name for a derived id.
func hashID(t *testing.T, dir, typeName, module, name string) string {
	t.Helper()
	args := []string{"hash-id", "--type", typeName, "--name", name, "--spec-dir", dir}
	if module != "" {
		args = append(args, "--module", module)
	}
	out, err := runNodeEditingSpex(t, args...)
	if err != nil {
		t.Fatalf("hash-id: unexpected error: %v", err)
	}
	return strings.TrimSpace(out)
}

// obligationKeys extracts every obligation's Message, the shape N-scenario
// assertions search with containsSubstr — mirroring author's own
// obligationKeys (obligation_reporter_test.go), unreachable from this
// package.
func obligationKeys(obligations []merkle.DiffError) []string {
	keys := make([]string, len(obligations))
	for i, o := range obligations {
		keys[i] = o.Message
	}
	return keys
}

func containsSubstr(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// addSecondModuleViaCLI registers a second module named modName with one
// component compName, entirely through `spex node add` — the "beta" module
// N12 needs to exercise a cross-module target, built the same way N3 already
// proved --type module works rather than hand-authoring project.json.
func addSecondModuleViaCLI(t *testing.T, dir, modName, compName string) string {
	t.Helper()
	if _, err := runNodeEditingSpex(t, "node", "add", modName, "--type", "module", "--spec-dir", dir); err != nil {
		t.Fatalf("node add --type module %s: unexpected error: %v", modName, err)
	}
	if _, err := runNodeEditingSpex(t, "node", "add", compName, "--type", "component", "--module", modName, "--spec-dir", dir); err != nil {
		t.Fatalf("node add --type component %s: unexpected error: %v", compName, err)
	}
	return hashID(t, dir, "component", modName, compName)
}

// N1: Adding a component places it, derives its id and scaffolds its leaf.
//
// NOTE on divergence from test_node_editing.md's N1 prose: as
// author/node_editor_test.go's own TestN1 documents (and
// drifts/drift-spexmachina-yih0.10.json records, non-blocking), "spex
// validate is green" cannot hold for a fresh, undescribed component under
// the default profile's component-describes-test_section coverage chain —
// the fixture's only test_section (T1) describes Comp1 and Comp2, not the
// newly added Widget. The gap surfaces as the component's own
// test_coverage obligation, never a refusal, which is what this test
// asserts in the drift report's place.
func TestN1_AddingComponentPlacesDerivesScaffolds(t *testing.T) {
	dir := t.TempDir()
	buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatalf("build snapshot tree: %v", err)
	}
	seedProjectState(t, dir, tree, time.Now())

	out, err := runNodeEditingSpex(t, "node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)

	widgetID := hashID(t, dir, "component", "alpha", "Widget")

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	var widget *schema.Component
	for i := range mod.Components {
		if mod.Components[i].ID == widgetID {
			widget = &mod.Components[i]
		}
	}
	if widget == nil {
		t.Fatalf("no component with hash-id-derived id %s found in %+v", widgetID, mod.Components)
	}
	if widget.Name != "Widget" || widget.Content != "arch_widget.md" {
		t.Errorf("widget entry = %+v, want name Widget, content arch_widget.md", widget)
	}

	content, err := os.ReadFile(filepath.Join(dir, "alpha", "arch_widget.md"))
	if err != nil {
		t.Fatalf("read arch_widget.md: %v", err)
	}
	if !strings.HasPrefix(string(content), "# Widget") {
		t.Errorf("arch_widget.md = %q, want to open with '# Widget'", content)
	}

	// spex validate: the only finding is the new component's own
	// test_coverage gap, travelling as an obligation on the write report.
	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want validate to report the new component's test_coverage gap")
	}
	valReport := decodeValidationReport(t, valOut)
	if len(valReport.Errors) != 1 || valReport.Errors[0].Check != "test_coverage" {
		t.Errorf("want exactly the new component's own test_coverage gap, got %+v", valReport.Errors)
	}

	testCoverageObligations := 0
	for _, o := range report.Obligations {
		if o.Type == "test_coverage" {
			testCoverageObligations++
		}
	}
	if testCoverageObligations != 1 {
		t.Errorf("want the test_coverage gap to travel as an obligation, got %+v", report.Obligations)
	}

	metaKeys := obligationKeys(report.Obligations)
	for _, comp := range []string{"Comp1", "Comp2"} {
		if !containsSubstr(metaKeys, comp) {
			t.Errorf("want a meta obligation naming %s, got %+v", comp, report.Obligations)
		}
	}

	// spex diff --json: exactly one added component (Widget) plus module
	// alpha's own meta change; the errors array carries the same
	// meta-obligation entries the write report printed under obligations.
	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr == nil {
		t.Fatal("want diff to exit non-zero: the meta obligations land in its errors array")
	}
	diff := decodeDiffJSON(t, diffOut)

	var addedComponents, metaChanges []diffChange
	for _, c := range diff.Changes {
		switch {
		case c.NodeType == "component" && c.Type == "added":
			addedComponents = append(addedComponents, c)
		case c.NodeType == "meta" && c.Module == "alpha":
			metaChanges = append(metaChanges, c)
		}
	}
	if len(addedComponents) != 1 || addedComponents[0].Path != widgetID {
		t.Errorf("added components = %+v, want exactly one with path %s", addedComponents, widgetID)
	}
	if len(metaChanges) == 0 {
		t.Error("want module alpha's meta leaf to show up as changed")
	}

	var diffErrMsgs []string
	for _, e := range diff.Errors {
		diffErrMsgs = append(diffErrMsgs, e.Message)
	}
	for _, comp := range []string{"Comp1", "Comp2"} {
		if !containsSubstr(diffErrMsgs, comp) {
			t.Errorf("diff errors do not name %s, got %+v", comp, diff.Errors)
		}
	}
}

// N2: A project-scoped type lands in project.json.
func TestN2_ProjectScopedTypeLandsInProjectJSON(t *testing.T) {
	dir := t.TempDir()
	buildNodeEditingFixture(t, dir)

	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "add", "New rule", "--type", "requirement",
		"--field", "type=functional", "--field", "priority=2", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)

	newID := hashID(t, dir, "requirement", "", "New rule")

	projData, err := os.ReadFile(filepath.Join(dir, "project.json"))
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

	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("alpha/module.json must be untouched by a project-scoped add")
	}

	var reqKeys []string
	for _, o := range report.Obligations {
		if o.Type == "requirement_coverage" {
			reqKeys = append(reqKeys, o.Message)
		}
	}
	if !containsSubstr(reqKeys, "New rule") {
		t.Errorf("want a requirement_coverage obligation naming 'New rule', got %+v", report.Obligations)
	}

	// spex validate: the same finding the write report printed as an
	// obligation, this time as the one refusal-shaped error a fresh,
	// undescribed project requirement earns.
	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want validate to report the new requirement's own coverage gap")
	}
	valReport := decodeValidationReport(t, valOut)
	if len(valReport.Errors) != 1 || valReport.Errors[0].Check != "requirement_coverage" {
		t.Errorf("want exactly one requirement_coverage error, got %+v", valReport.Errors)
	}
	if len(reqKeys) != 1 || valReport.Errors[0].Message != reqKeys[0] {
		t.Errorf("validate message %q must be byte-identical to the write report's obligation message %+v", valReport.Errors[0].Message, reqKeys)
	}
}

// N3: A module is registered and skeletoned by one command.
func TestN3_ModuleRegisteredAndSkeletoned(t *testing.T) {
	dir := t.TempDir()
	buildNodeEditingFixture(t, dir)

	out, err := runNodeEditingSpex(t, "node", "add", "beta", "--type", "module", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add: unexpected error: %v\n%s", err, out)
	}
	_ = decodeWriteReport(t, out)

	betaID := hashID(t, dir, "module", "", "beta")

	projData, err := os.ReadFile(filepath.Join(dir, "project.json"))
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

	modData, err := os.ReadFile(filepath.Join(dir, "beta", "module.json"))
	if err != nil {
		t.Fatalf("read beta/module.json: %v", err)
	}
	if string(modData) != "{\n  \"name\": \"beta\"\n}\n" {
		t.Errorf("beta/module.json = %q, want exactly {\"name\": \"beta\"} in canonical two-space form", modData)
	}

	if _, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir); err != nil {
		t.Fatalf("validate over the fixture with the new empty module should be green: %v", err)
	}

	renderOut, err := runNodeEditingSpex(t, "render", "--format", "json", "--slim", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("render: unexpected error: %v", err)
	}
	var slim struct {
		Nodes []struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Name   string `json:"name"`
			Module string `json:"module"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(renderOut), &slim); err != nil {
		t.Fatalf("render output is not valid JSON: %v\n%s", err, renderOut)
	}
	found := false
	for _, n := range slim.Nodes {
		if n.Type == "module" && n.Name == "beta" {
			found = true
		}
	}
	if !found {
		t.Errorf("render --format json --slim does not list beta among the modules: %+v", slim.Nodes)
	}
}

// N4: An undeclared type is refused with the declared list as the fix; the
// same run over a fixture whose profile declares the type succeeds.
func TestN4_UndeclaredTypeRefused(t *testing.T) {
	dir := t.TempDir()
	buildNodeEditingFixture(t, dir)

	before, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "add", "Anything", "--type", "impl_section", "--module", "alpha", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for an undeclared type")
	}
	if exitCodeOf(err) != author.ExitRefusal {
		t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
	}
	refusals := decodeRefusals(t, out)
	if len(refusals) != 1 {
		t.Fatalf("want exactly one refusal, got %+v", refusals)
	}
	for _, want := range []string{"requirement", "component", "data_flow", "test_section", "api", "module"} {
		if !strings.Contains(refusals[0].Fix, want) {
			t.Errorf("fix should list declared type %q, got: %s", want, refusals[0].Fix)
		}
	}
	if !strings.Contains(refusals[0].Message, "impl_section") {
		t.Errorf("message should name the undeclared type, got: %s", refusals[0].Message)
	}

	after, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
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
	writeAuthorJSON(t, filepath.Join(dir, "profile.json"), custom)

	out, err = runNodeEditingSpex(t, "node", "add", "Anything", "--type", "impl_section", "--module", "alpha", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add over a profile declaring impl_section: unexpected error: %v\n%s", err, out)
	}
	_ = decodeWriteReport(t, out)

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
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

// N5: A name the tokenizer would not reproduce is refused with the
// declarable form.
func TestN5_UndeclarableNameRefused(t *testing.T) {
	dir := t.TempDir()
	buildNodeEditingFixture(t, dir)

	before, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "add", "demo run [--json]", "--type", "api", "--module", "alpha", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for an undeclarable name")
	}
	refusals := decodeRefusals(t, out)
	if len(refusals) == 0 {
		t.Fatal("want at least one refusal for an undeclarable name")
	}

	var found *author.RefusalEntry
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

	after, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity oracle: the same entry written by hand fails spex validate
	// with the same message.
	var mod schema.ModuleSpec
	if err := json.Unmarshal(before, &mod); err != nil {
		t.Fatal(err)
	}
	mod.APIs = append(mod.APIs, schema.API{
		ID:   schema.IdentityHash("alpha", "api", "demo run [--json]"),
		Name: "demo run [--json]",
	})
	writeAuthorJSON(t, filepath.Join(dir, "alpha", "module.json"), mod)

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want validate to fail over the hand-written undeclarable name")
	}
	valReport := decodeValidationReport(t, valOut)
	handMsg := ""
	for _, e := range valReport.Errors {
		if e.Check == "id" && strings.Contains(e.Message, "demo run [--json]") {
			handMsg = e.Message
		}
	}
	if handMsg == "" {
		t.Fatalf("want validate to report the undeclarable api name, got %+v", valReport.Errors)
	}
	if handMsg != found.Message {
		t.Errorf("command refusal message %q != hand-edit validator message %q", found.Message, handMsg)
	}
}

// N6: Removing a referenced node refuses and lists every inbound reference.
func TestN6_RemovingReferencedNodeRefusesListingInboundRefs(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeLeaf, err := os.ReadFile(filepath.Join(dir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "remove", f.comp1ID, "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for removing a referenced node")
	}
	if exitCodeOf(err) != author.ExitRefusal {
		t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
	}
	refusals := decodeRefusals(t, out)
	if len(refusals) == 0 {
		t.Fatal("want at least one refusal for a referenced node")
	}

	var idRefusals, linkRefusals []author.RefusalEntry
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

	// The scenario names each inbound reference individually, by field and
	// by file:line — not merely by count.
	findRefusal := func(refs []author.RefusalEntry, pathContains string) *author.RefusalEntry {
		for i := range refs {
			if strings.Contains(refs[i].Path, pathContains) {
				return &refs[i]
			}
		}
		return nil
	}

	comp2Uses := findRefusal(idRefusals, "alpha/module.json:/components/"+f.comp2ID)
	if comp2Uses == nil || !strings.Contains(comp2Uses.Message, "uses") {
		t.Errorf("want an id refusal naming Comp2's uses field at alpha/module.json:/components/%s, got %+v", f.comp2ID, idRefusals)
	}
	t1Describes := findRefusal(idRefusals, "alpha/module.json:/test_sections/"+f.t1ID)
	if t1Describes == nil || !strings.Contains(t1Describes.Message, "describes") {
		t.Errorf("want an id refusal naming T1's describes field at alpha/module.json:/test_sections/%s, got %+v", f.t1ID, idRefusals)
	}
	apiProvidedBy := findRefusal(idRefusals, "alpha/module.json:/apis/"+f.apiID)
	if apiProvidedBy == nil || !strings.Contains(apiProvidedBy.Message, "provided_by") {
		t.Errorf("want an id refusal naming the api's provided_by field at alpha/module.json:/apis/%s, got %+v", f.apiID, idRefusals)
	}

	comp2Link := findRefusal(linkRefusals, "alpha/arch_comp2.md:")
	if comp2Link == nil {
		t.Errorf("want a link refusal at alpha/arch_comp2.md:<line>, got %+v", linkRefusals)
	}
	t1Link := findRefusal(linkRefusals, "alpha/test_t1.md:")
	if t1Link == nil {
		t.Errorf("want a link refusal at alpha/test_t1.md:<line>, got %+v", linkRefusals)
	}
	if comp2Link != nil && t1Link != nil && comp2Link.Path == t1Link.Path {
		t.Errorf("want the two link refusals to name different files, both got %s", comp2Link.Path)
	}
	for _, r := range linkRefusals {
		if r.Path == "" {
			t.Errorf("want every link refusal to carry a non-empty file:line path, got %+v", r)
		}
	}

	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a refusal must leave module.json byte-identical")
	}
	afterLeaf, err := os.ReadFile(filepath.Join(dir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatalf("arch_comp1.md must survive a refused removal: %v", err)
	}
	if string(beforeLeaf) != string(afterLeaf) {
		t.Error("a refusal must leave arch_comp1.md byte-identical")
	}
}

// N7: A forced removal lists what it left dangling.
//
// NOTE on divergence from test_node_editing.md's N7 prose: as
// drifts/drift-spexmachina-yih0.15.json records (non-blocking), "spex diff
// --json reports ... the surviving_name error for Comp1" does not hold for
// this fixture. The forced removal takes arch_comp1.md — the one file where
// "Comp1" was a bare prose token — with it; every other corpus mention is
// wikilink display text ("[[<id>|Comp1]]" in arch_comp2.md and
// test_t1.md), which validator.CheckRemovedNames's tokenizer folds into one
// non-matching token per link rather than isolating "Comp1" as its own
// phrase. So the sweep finds nothing to report, and this test asserts what
// `spex diff --json` actually returns: one removed component and no
// surviving_name error.
func TestN7_ForcedRemovalListsWhatItLeftDangling(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	out, err := runNodeEditingSpex(t, "node", "remove", f.comp1ID, "--force", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node remove --force: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)
	if report.RetiredName != "Comp1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "Comp1")
	}

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
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
	if _, err := os.Stat(filepath.Join(dir, "alpha", "arch_comp1.md")); !os.IsNotExist(err) {
		t.Errorf("arch_comp1.md should be gone, stat err = %v", err)
	}

	danglingKeys := obligationKeys(report.Obligations)
	if !containsSubstr(danglingKeys, f.comp1ID) {
		t.Fatalf("want the dangling obligations to name %s, got %+v", f.comp1ID, report.Obligations)
	}

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want validate to report the dangling references left by a forced removal")
	}
	valReport := decodeValidationReport(t, valOut)
	if len(valReport.Errors) == 0 {
		t.Fatal("want spex validate to report the dangling references left by a forced removal")
	}
	for _, e := range valReport.Errors {
		if !containsSubstr(danglingKeys, e.Message) {
			t.Errorf("validate finding %+v not among the obligations the command printed: %+v", e, report.Obligations)
		}
	}

	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr == nil {
		t.Fatal("want diff to exit non-zero: the incomplete_change obligation lands in its errors array")
	}
	diff := decodeDiffJSON(t, diffOut)
	var removedComponents []diffChange
	for _, c := range diff.Changes {
		if c.NodeType == "component" && c.Type == "removed" {
			removedComponents = append(removedComponents, c)
		}
	}
	if len(removedComponents) != 1 || removedComponents[0].Path != f.comp1ID {
		t.Errorf("removed components = %+v, want exactly one with path %s", removedComponents, f.comp1ID)
	}

	// See the NOTE above TestN7's declaration and
	// drifts/drift-spexmachina-yih0.15.json: the spec's own "and the
	// surviving_name error for Comp1" does not hold for this fixture, so
	// this asserts what the command actually returns rather than dropping
	// the clause unmarked.
	for _, e := range diff.Errors {
		if e.Type == "surviving_name" {
			t.Errorf("want no surviving_name error for this fixture (see drifts/drift-spexmachina-yih0.15.json), got %+v", e)
		}
	}
	if len(diff.Errors) != 1 || diff.Errors[0].Type != "incomplete_change" {
		t.Errorf("want exactly the incomplete_change entry for module alpha's unchanged Comp2 leaf, got %+v", diff.Errors)
	}
}

// N8: A rename is one transaction across arrays, leaves and the content
// file.
func TestN8_RenameIsOneTransaction(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	out, err := runNodeEditingSpex(t, "node", "rename", f.comp1ID, "Core", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node rename: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)
	if report.RetiredName != "Comp1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "Comp1")
	}

	newID := hashID(t, dir, "component", "alpha", "Core")

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
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

	if len(mod.TestSections) != 1 {
		t.Fatalf("unexpected test sections: %+v", mod.TestSections)
	}
	if !containsStr(mod.TestSections[0].Describes, newID) || containsStr(mod.TestSections[0].Describes, f.comp1ID) {
		t.Errorf("T1.Describes = %v, want to carry %s and not %s", mod.TestSections[0].Describes, newID, f.comp1ID)
	}
	if len(mod.APIs) != 1 || !containsStr(mod.APIs[0].ProvidedBy, newID) || containsStr(mod.APIs[0].ProvidedBy, f.comp1ID) {
		t.Errorf("api.ProvidedBy = %v, want to carry %s and not %s", mod.APIs[0].ProvidedBy, newID, f.comp1ID)
	}

	if _, err := os.Stat(filepath.Join(dir, "alpha", "arch_comp1.md")); !os.IsNotExist(err) {
		t.Errorf("arch_comp1.md should be gone, stat err = %v", err)
	}
	coreContent, err := os.ReadFile(filepath.Join(dir, "alpha", "arch_core.md"))
	if err != nil {
		t.Fatalf("read arch_core.md: %v", err)
	}
	if !strings.Contains(string(coreContent), "Implements [["+f.r1ID+"|R1]]") {
		t.Errorf("arch_core.md lost its original content: %s", coreContent)
	}

	comp2Content, err := os.ReadFile(filepath.Join(dir, "alpha", "arch_comp2.md"))
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	if !strings.Contains(string(comp2Content), "[["+newID+"|Comp1]]") {
		t.Errorf("arch_comp2.md did not repoint its link to %s with display text preserved: %s", newID, comp2Content)
	}
	if strings.Contains(string(comp2Content), f.comp1ID) {
		t.Errorf("arch_comp2.md still names the retired id %s: %s", f.comp1ID, comp2Content)
	}

	t1Content, err := os.ReadFile(filepath.Join(dir, "alpha", "test_t1.md"))
	if err != nil {
		t.Fatalf("read test_t1.md: %v", err)
	}
	if !strings.Contains(string(t1Content), "[["+newID+"|Comp1]]") {
		t.Errorf("test_t1.md did not repoint its Comp1 link to %s: %s", newID, t1Content)
	}
	if !strings.Contains(string(t1Content), "[["+f.comp2ID+"|Comp2]]") {
		t.Errorf("test_t1.md should keep its unrelated Comp2 link untouched: %s", t1Content)
	}

	if _, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir); err != nil {
		t.Fatalf("validate over the renamed fixture should be green: %v", err)
	}

	// diff may or may not exit non-zero here: arch_core.md still opens with
	// the retired heading "# Comp1" (NodeRenamer moves the file and
	// repoints links but never rewrites the leaf's own prose), so the
	// removal sweep's surviving_name check can fire on that residual
	// mention. N8's own text carves this out explicitly — "Whether a
	// surviving_name error fires for the mentions of Comp1 the fixture's
	// prose still carries is the removal sweep's rule and is not asserted
	// here" — so only the component change shape is asserted, not the
	// errors array as a whole.
	diffOut, _ := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	diff := decodeDiffJSON(t, diffOut)

	var removed, added []diffChange
	for _, c := range diff.Changes {
		if c.NodeType != "component" {
			continue
		}
		switch c.Type {
		case "removed":
			removed = append(removed, c)
		case "added":
			added = append(added, c)
		}
	}
	if len(removed) != 1 || removed[0].Path != f.comp1ID {
		t.Errorf("removed components = %+v, want exactly one with path %s", removed, f.comp1ID)
	}
	if len(added) != 1 || added[0].Path != newID {
		t.Errorf("added components = %+v, want exactly one with path %s", added, newID)
	}
	for _, e := range diff.Errors {
		if e.Type != "surviving_name" {
			t.Errorf("want nothing dangling but a possible surviving_name note after a rename, got %+v", e)
		}
	}
}

// N9: An edge is checked against the profile before it is written.
func TestN9_EdgeCheckedAgainstProfileBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)
	before, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("field not declared on source type", func(t *testing.T) {
		out, err := runNodeEditingSpex(t, "edge", "add", f.comp2ID, "describes", f.comp1ID, "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for an undeclared reference field")
		}
		refusals := decodeRefusals(t, out)
		if len(refusals) != 1 {
			t.Fatalf("want exactly one refusal, got %+v", refusals)
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
		out, err := runNodeEditingSpex(t, "edge", "add", f.comp2ID, "uses", f.r1ID, "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for a disallowed target type")
		}
		refusals := decodeRefusals(t, out)
		if len(refusals) != 1 {
			t.Fatalf("want exactly one refusal, got %+v", refusals)
		}
		if !strings.Contains(refusals[0].Fix, "component") {
			t.Errorf("fix should name component as the only target type uses permits, got: %s", refusals[0].Fix)
		}
	})

	after, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}
}

// N10: A cycle is refused before it is written.
func TestN10_CycleRefusedBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)
	before, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "edge", "add", f.comp1ID, "uses", f.comp2ID, "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for an edge that closes a cycle")
	}
	refusals := decodeRefusals(t, out)
	if len(refusals) != 1 {
		t.Fatalf("want exactly one refusal, got %+v", refusals)
	}
	if refusals[0].Check != "dag" {
		t.Errorf("Check = %q, want %q (the validator's own dag check)", refusals[0].Check, "dag")
	}
	if !strings.Contains(refusals[0].Message, "cycle") {
		t.Errorf("message should name the cycle, got: %s", refusals[0].Message)
	}

	after, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity oracle: the same entry written by hand fails spex validate
	// with the same dag message.
	handDir := t.TempDir()
	handF := buildNodeEditingFixture(t, handDir)
	handModPath := filepath.Join(handDir, "alpha", "module.json")
	handData, err := os.ReadFile(handModPath)
	if err != nil {
		t.Fatal(err)
	}
	var handMod schema.ModuleSpec
	if err := json.Unmarshal(handData, &handMod); err != nil {
		t.Fatal(err)
	}
	for i := range handMod.Components {
		if handMod.Components[i].ID == handF.comp1ID {
			handMod.Components[i].Uses = append(handMod.Components[i].Uses, handF.comp2ID)
		}
	}
	writeAuthorJSON(t, handModPath, handMod)

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", handDir)
	if err == nil {
		t.Fatal("want validate to fail over the hand-edited cycle")
	}
	valReport := decodeValidationReport(t, valOut)
	handMsg := ""
	for _, e := range valReport.Errors {
		if e.Check == "dag" {
			handMsg = e.Message
		}
	}
	if handMsg == "" {
		t.Fatalf("want a hand-edit dag error, got %+v", valReport.Errors)
	}
	if handMsg != refusals[0].Message {
		t.Errorf("command message %q != hand-edit validator message %q", refusals[0].Message, handMsg)
	}
}

// N11: An existing edge is a no-op, and its removal restores the tree.
func TestN11_ExistingEdgeIsNoOpAndRemovalRestoresTree(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())
	beforeBytes, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	addOut, err := runNodeEditingSpex(t, "edge", "add", f.comp2ID, "uses", f.comp1ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("edge add: unexpected error: %v\n%s", err, addOut)
	}
	addReport := decodeWriteReport(t, addOut)
	if len(addReport.Written) != 0 {
		t.Errorf("want no files written for an existing entry, got %+v", addReport.Written)
	}
	afterAdd, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterAdd) {
		t.Error("a no-op edge add must leave the tree byte-identical")
	}

	removeOut, err := runNodeEditingSpex(t, "edge", "remove", f.comp2ID, "uses", f.comp1ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("edge remove: unexpected error: %v\n%s", err, removeOut)
	}
	removeReport := decodeWriteReport(t, removeOut)
	if len(removeReport.Written) == 0 {
		t.Fatalf("want a write, got %+v", removeReport)
	}

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	for _, c := range mod.Components {
		if c.ID == f.comp2ID && len(c.Uses) != 0 {
			t.Errorf("Comp2.Uses should be empty after removal, got %v", c.Uses)
		}
	}

	keys := obligationKeys(removeReport.Obligations)
	for _, name := range []string{"Comp1", "Comp2"} {
		if !containsSubstr(keys, name) {
			t.Errorf("want an obligation naming %s, got %+v", name, removeReport.Obligations)
		}
	}

	diffOut, _ := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	diff := decodeDiffJSON(t, diffOut)
	var metaChanges, otherChanges []diffChange
	for _, c := range diff.Changes {
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

// N12: A cross-module target is refused on a module-local field.
func TestN12_CrossModuleTargetRefusedOnModuleLocalField(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)
	otherID := addSecondModuleViaCLI(t, dir, "beta", "Other")

	before, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "edge", "add", f.apiID, "provided_by", otherID, "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for a cross-module provided_by target")
	}
	refusals := decodeRefusals(t, out)
	if len(refusals) != 1 {
		t.Fatalf("want exactly one refusal, got %+v", refusals)
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

	after, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a refusal must leave the tree byte-identical")
	}
}

// N13: A hand-formatted file is reformatted, and no hash moves.
func TestN13_HandFormattedFileReformattedNoHashMoves(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	modPath := filepath.Join(dir, "alpha", "module.json")
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

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	out, err := runNodeEditingSpex(t, "edge", "add", f.comp2ID, "implements", f.r1ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("edge add: unexpected error: %v\n%s", err, out)
	}
	_ = decodeWriteReport(t, out)

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

	// The profile's key order for node fields: Comp2 now carries implements
	// (this add) alongside its original content and uses, all after id/name.
	gotKeys := canonicalObjectKeys(t, after, f.comp2ID)
	wantKeys := []string{"id", "name", "content", "implements", "uses"}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Errorf("Comp2's field key order = %v, want %v", gotKeys, wantKeys)
	}

	// The same add over a copy of the fixture left hand-formatted a
	// different way (one line, no indent at all, rather than this test's
	// own four-space rewrite) produces a byte-identical file.
	dir2 := t.TempDir()
	f2 := buildNodeEditingFixture(t, dir2)
	modPath2 := filepath.Join(dir2, "alpha", "module.json")
	data2, err := os.ReadFile(modPath2)
	if err != nil {
		t.Fatal(err)
	}
	var raw2 map[string]any
	if err := json.Unmarshal(data2, &raw2); err != nil {
		t.Fatal(err)
	}
	oneLine, err := json.Marshal(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath2, oneLine, 0644); err != nil {
		t.Fatal(err)
	}
	tree2, err := merkle.BuildTree(dir2)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir2, tree2, time.Now())
	if out2, err := runNodeEditingSpex(t, "edge", "add", f2.comp2ID, "implements", f2.r1ID, "--spec-dir", dir2); err != nil {
		t.Fatalf("edge add over the second, differently hand-formatted copy: unexpected error: %v\n%s", err, out2)
	}
	after2, err := os.ReadFile(modPath2)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(after2) {
		t.Errorf("edge add over a differently hand-formatted copy produced a different file:\nfirst:\n%s\nsecond:\n%s", after, after2)
	}

	diffOut, _ := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	diff := decodeDiffJSON(t, diffOut)
	var metaChanges, otherChanges []diffChange
	for _, c := range diff.Changes {
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

// N14: The commands need no initialised project.
func TestN14_NoInitialisedProjectNeeded(t *testing.T) {
	plainDir := t.TempDir()
	buildNodeEditingFixture(t, plainDir)

	initDir := t.TempDir()
	buildNodeEditingFixture(t, initDir)
	spexDir := filepath.Join(initDir, ".spex")
	if err := os.MkdirAll(spexDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, spexDir, "snapshot.json", `{"fake":"snapshot"}`)
	beforeSpex, err := os.ReadFile(filepath.Join(spexDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}

	if out, err := runNodeEditingSpex(t, "node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", plainDir); err != nil {
		t.Fatalf("node add over the uninitialised fixture: unexpected error: %v\n%s", err, out)
	}
	if out, err := runNodeEditingSpex(t, "node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", initDir); err != nil {
		t.Fatalf("node add over the initialised fixture: unexpected error: %v\n%s", err, out)
	}

	plainMod, err := os.ReadFile(filepath.Join(plainDir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	initMod, err := os.ReadFile(filepath.Join(initDir, "alpha", "module.json"))
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
		t.Error(".spex/ must be byte-identical before and after: node add reads and writes nothing under it")
	}
}

// N15: A cardinality-one field is retargeted by one add, and a required
// one is never cleared. The CLI-level mirror of
// TestAddEdge_CardinalityOneRetarget_ReplacesAndReportsDisplacedTarget and
// TestRemoveEdge_RequiredCardinalityOne_RefusesRatherThanClear
// (author/edge_editor_test.go).
func TestN15_CardinalityOneRetargetedByAddNeverClearedByRemove(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	addOut, err := runNodeEditingSpex(t, "edge", "add", f.r1ID, "preq_id", f.p2ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("edge add: unexpected error: %v\n%s", err, addOut)
	}
	addReport := decodeWriteReport(t, addOut)
	if len(addReport.Written) == 0 {
		t.Fatalf("edge add: want a write, got %+v", addReport)
	}
	if addReport.ReplacedTarget != f.p1ID {
		t.Errorf("replaced_target = %q, want the displaced target %q", addReport.ReplacedTarget, f.p1ID)
	}

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
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

	addKeys := obligationKeys(addReport.Obligations)
	if !containsSubstr(addKeys, "Comp1") {
		t.Errorf("want a completeness obligation naming Comp1 (R1's implementor), got %+v", addReport.Obligations)
	}
	if !containsSubstr(addKeys, f.p1ID) {
		t.Errorf("want a requirement_coverage obligation naming P1, now derived by nothing, got %+v", addReport.Obligations)
	}

	diffOut, _ := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	diff := decodeDiffJSON(t, diffOut)
	var modifiedReq, metaChanges []diffChange
	for _, c := range diff.Changes {
		switch {
		case c.NodeType == "requirement" && c.Type == "modified":
			modifiedReq = append(modifiedReq, c)
		case c.NodeType == "meta" && c.Module == "alpha":
			metaChanges = append(metaChanges, c)
		}
	}
	if len(modifiedReq) != 1 {
		t.Errorf("want exactly one modified requirement (R1), got %+v", diff.Changes)
	}
	if len(metaChanges) != 1 {
		t.Errorf("want exactly one meta change (alpha), got %+v", diff.Changes)
	}
	if len(diff.Changes) != 2 {
		t.Errorf("want exactly two changes total, got %+v", diff.Changes)
	}

	afterAdd, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	removeOut, err := runNodeEditingSpex(t, "edge", "remove", f.r1ID, "preq_id", f.p2ID, "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for removing a required cardinality-one field")
	}
	refusals := decodeRefusals(t, removeOut)
	if len(refusals) != 2 {
		t.Fatalf("want exactly two refusals (schema + id), got %+v", refusals)
	}

	var schemaEntry, idEntry *author.RefusalEntry
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

	afterRemove, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterAdd) != string(afterRemove) {
		t.Error("a refusal must leave the tree byte-identical")
	}

	// Parity oracle: the same entry with preq_id deleted by hand fails
	// spex validate with the same two check values and messages.
	handDir := t.TempDir()
	handF := buildNodeEditingFixture(t, handDir)
	handModPath := filepath.Join(handDir, "alpha", "module.json")
	handData, err := os.ReadFile(handModPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(handData, &raw); err != nil {
		t.Fatal(err)
	}
	reqs := raw["requirements"].([]any)
	for _, r := range reqs {
		reqMap := r.(map[string]any)
		if reqMap["id"] == handF.r1ID {
			delete(reqMap, "preq_id")
		}
	}
	handOut, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(handModPath, handOut, 0644); err != nil {
		t.Fatal(err)
	}

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", handDir)
	if err == nil {
		t.Fatal("want validate to fail over the hand-edited missing preq_id")
	}
	valReport := decodeValidationReport(t, valOut)
	var handSchemaMsg, handIDMsg string
	for _, e := range valReport.Errors {
		switch e.Check {
		case "schema":
			handSchemaMsg = e.Message
		case "id":
			if strings.Contains(e.Message, "missing preq_id") {
				handIDMsg = e.Message
			}
		}
	}
	if handSchemaMsg == "" {
		t.Fatalf("want a hand-edit schema error, got %+v", valReport.Errors)
	}
	if handIDMsg == "" {
		t.Fatalf("want a hand-edit id error naming missing preq_id, got %+v", valReport.Errors)
	}
	if handSchemaMsg != schemaEntry.Message {
		t.Errorf("command schema message %q != hand-edit validator message %q", schemaEntry.Message, handSchemaMsg)
	}
	if handIDMsg != idEntry.Message {
		t.Errorf("command id message %q != hand-edit validator message %q", idEntry.Message, handIDMsg)
	}
}

// obligationsOfType filters obligations by DiffError.Type — this package's
// mirror of author's own obligationsOfType (obligation_reporter_test.go),
// unreachable from here.
func obligationsOfType(obligations []merkle.DiffError, typ string) []merkle.DiffError {
	var out []merkle.DiffError
	for _, o := range obligations {
		if o.Type == typ {
			out = append(out, o)
		}
	}
	return out
}

// setProjectRequirementPriority overwrites id's priority in dir's
// project.json directly, bypassing the command under test — N17 needs P1
// to start at priority 1 before the scenario's own --field priority=2 set.
func setProjectRequirementPriority(t *testing.T, dir, id string, priority int) {
	t.Helper()
	path := filepath.Join(dir, "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatal(err)
	}
	for i := range proj.Requirements {
		if proj.Requirements[i].ID == id {
			proj.Requirements[i].Priority = &priority
		}
	}
	writeAuthorJSON(t, path, proj)
}

// setModuleRequirementDescription overwrites id's description in dir's
// alpha/module.json directly — N21 needs a known starting value for the
// no-op half of its scenario.
func setModuleRequirementDescription(t *testing.T, dir, id, desc string) {
	t.Helper()
	path := filepath.Join(dir, "alpha", "module.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatal(err)
	}
	for i := range mod.Requirements {
		if mod.Requirements[i].ID == id {
			mod.Requirements[i].Description = desc
		}
	}
	writeAuthorJSON(t, path, mod)
}

// N16: Setting a module requirement's description obliges exactly the
// implementing leaves.
func TestN16_SetModuleRequirementDescription_ObligesImplementingLeaf(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	beforeProj, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--field", "description=R1's new description", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)

	modData, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatal(err)
	}
	var r1 *schema.ModuleRequirement
	for i := range mod.Requirements {
		if mod.Requirements[i].ID == f.r1ID {
			r1 = &mod.Requirements[i]
		}
	}
	if r1 == nil {
		t.Fatalf("R1 not found in %+v", mod.Requirements)
	}
	if r1.Description != "R1's new description" {
		t.Errorf("R1.Description = %q, want %q", r1.Description, "R1's new description")
	}
	if r1.PreqID != f.p1ID || r1.Type != "functional" || r1.Title != "R1" {
		t.Errorf("R1's other fields must be unchanged, got %+v", r1)
	}

	afterProj, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeProj) != string(afterProj) {
		t.Error("project.json must be untouched by a module-scoped set")
	}

	completeness := obligationsOfType(report.Obligations, "incomplete_change")
	if len(completeness) != 1 {
		t.Fatalf("want exactly one completeness obligation, got %+v", report.Obligations)
	}
	if completeness[0].Path != f.r1ID {
		t.Errorf("obligation Path = %q, want R1's id %s", completeness[0].Path, f.r1ID)
	}
	if len(completeness[0].Related) != 1 || completeness[0].Related[0] != f.comp1ID {
		t.Errorf("obligation Related = %v, want exactly [%s]", completeness[0].Related, f.comp1ID)
	}
	if strings.Contains(completeness[0].Message, "Comp2") {
		t.Errorf("obligation must not name Comp2 (the requirement-changed rule), got %q", completeness[0].Message)
	}

	// spex diff --json against the snapshot: R1 modified beside alpha's
	// meta change, and its errors array holds that same single entry.
	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr == nil {
		t.Fatal("want diff to exit non-zero: the incomplete_change obligation lands in its errors array")
	}
	diff := decodeDiffJSON(t, diffOut)

	var modifiedReqs, metaChanges []diffChange
	for _, c := range diff.Changes {
		switch {
		case c.NodeType == "requirement" && c.Type == "modified":
			modifiedReqs = append(modifiedReqs, c)
		case c.NodeType == "meta" && c.Module == "alpha":
			metaChanges = append(metaChanges, c)
		}
	}
	if len(modifiedReqs) != 1 || modifiedReqs[0].Path != f.r1ID {
		t.Errorf("modified requirements = %+v, want exactly one with path %s", modifiedReqs, f.r1ID)
	}
	if len(metaChanges) != 1 {
		t.Errorf("want exactly one meta change (alpha), got %+v", diff.Changes)
	}
	if len(diff.Changes) != 2 {
		t.Errorf("want exactly two changes total, got %+v", diff.Changes)
	}
	if len(diff.Errors) != 1 || diff.Errors[0].Path != completeness[0].Path || diff.Errors[0].Message != completeness[0].Message {
		t.Errorf("diff errors = %+v, want exactly the write report's own completeness entry %+v", diff.Errors, completeness[0])
	}

	// Parity oracle, accepting direction: the same description written by
	// hand into a copy's module.json gives spex diff --json the same
	// entry, so the command discovered nothing the hand edit would not
	// have earned.
	handDir := t.TempDir()
	buildNodeEditingFixture(t, handDir)
	handTree, err := merkle.BuildTree(handDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, handDir, handTree, time.Now())
	setModuleRequirementDescription(t, handDir, f.r1ID, "R1's new description")

	handDiffOut, handDiffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", handDir)
	if handDiffErr == nil {
		t.Fatal("want the hand-edited diff to exit non-zero too")
	}
	handDiff := decodeDiffJSON(t, handDiffOut)
	if len(handDiff.Errors) != 1 || handDiff.Errors[0].Path != completeness[0].Path || handDiff.Errors[0].Message != completeness[0].Message {
		t.Errorf("hand-edit diff errors = %+v, want the same entry the command discovered %+v", handDiff.Errors, completeness[0])
	}
}

// N17: A project requirement's field lands in project.json and the walk
// down is the obligation.
func TestN17_SetProjectRequirementPriority_WalksDownToImplementor(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)
	setProjectRequirementPriority(t, dir, f.p1ID, 1)

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "set", f.p1ID, "--field", "priority=2", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)

	projData, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatal(err)
	}
	var p1 *schema.Requirement
	for i := range proj.Requirements {
		if proj.Requirements[i].ID == f.p1ID {
			p1 = &proj.Requirements[i]
		}
	}
	if p1 == nil || p1.Priority == nil || *p1.Priority != 2 {
		t.Fatalf("P1.Priority = %+v, want 2", p1)
	}
	if !strings.Contains(string(projData), `"priority": 2`) {
		t.Errorf("priority must be written as a JSON number, got: %s", projData)
	}

	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("alpha/module.json must be untouched by a project-scoped set")
	}

	completeness := obligationsOfType(report.Obligations, "incomplete_change")
	if len(completeness) != 1 {
		t.Fatalf("want exactly one completeness obligation, got %+v", report.Obligations)
	}
	if completeness[0].Path != f.p1ID {
		t.Errorf("obligation Path = %q, want P1's id %s", completeness[0].Path, f.p1ID)
	}
	if !containsStr(completeness[0].Related, f.comp1ID) {
		t.Errorf("obligation Related = %v, want to name Comp1", completeness[0].Related)
	}

	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr == nil {
		t.Fatal("want diff to exit non-zero: the incomplete_change obligation lands in its errors array")
	}
	diff := decodeDiffJSON(t, diffOut)
	var modifiedReqs, metaChanges []diffChange
	for _, c := range diff.Changes {
		switch {
		case c.NodeType == "requirement" && c.Type == "modified":
			modifiedReqs = append(modifiedReqs, c)
		case c.NodeType == "meta" && c.Module == "":
			metaChanges = append(metaChanges, c)
		}
	}
	if len(modifiedReqs) != 1 || modifiedReqs[0].Path != f.p1ID {
		t.Errorf("modified requirements = %+v, want exactly one with path %s", modifiedReqs, f.p1ID)
	}
	if len(metaChanges) != 1 {
		t.Errorf("want exactly one project-level meta change, got %+v", diff.Changes)
	}
	if len(diff.Errors) != 1 || diff.Errors[0].Path != completeness[0].Path || diff.Errors[0].Message != completeness[0].Message {
		t.Errorf("diff errors = %+v, want exactly the write report's own completeness entry %+v", diff.Errors, completeness[0])
	}

	settledProj := projData

	// --unset priority is refused with the tree unchanged: the schema
	// leaves the field optional, but the validator's own presence check
	// (the "id" check) does not.
	out2, err := runNodeEditingSpex(t, "node", "set", f.p1ID, "--unset", "priority", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for unsetting a project requirement's priority")
	}
	if exitCodeOf(err) != author.ExitRefusal {
		t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
	}
	refusals2 := decodeRefusals(t, out2)
	var foundPriorityRefusal *author.RefusalEntry
	for i := range refusals2 {
		if refusals2[i].Check == "id" && strings.Contains(refusals2[i].Message, "missing priority") {
			foundPriorityRefusal = &refusals2[i]
		}
	}
	if foundPriorityRefusal == nil {
		t.Fatalf("want the validator's id-check refusal for missing priority, got %+v", refusals2)
	}
	after2, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(settledProj) != string(after2) {
		t.Error("a refusal must leave project.json byte-identical")
	}

	// --field priority=five is refused through the validator's own schema
	// entry, its fix naming the field as an integer.
	out3, err := runNodeEditingSpex(t, "node", "set", f.p1ID, "--field", "priority=five", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for a non-integer priority")
	}
	refusals3 := decodeRefusals(t, out3)
	var schemaEntry *author.RefusalEntry
	for i := range refusals3 {
		if refusals3[i].Check == "schema" && strings.Contains(refusals3[i].Fix, "priority") && strings.Contains(refusals3[i].Fix, "integer") {
			schemaEntry = &refusals3[i]
		}
	}
	if schemaEntry == nil {
		t.Fatalf("want the validator's schema refusal naming priority as an integer, got %+v", refusals3)
	}
	after3, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(settledProj) != string(after3) {
		t.Error("a refusal must leave project.json byte-identical")
	}

	// R1's enumerated "type" is refused the same way, naming the
	// enumeration.
	out4, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--field", "type=optional", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for an out-of-enumeration type")
	}
	refusals4 := decodeRefusals(t, out4)
	var enumEntry *author.RefusalEntry
	for i := range refusals4 {
		if refusals4[i].Check == "schema" && strings.Contains(refusals4[i].Fix, "functional") && strings.Contains(refusals4[i].Fix, "non_functional") {
			enumEntry = &refusals4[i]
		}
	}
	if enumEntry == nil {
		t.Fatalf("want the validator's schema refusal listing the enumeration, got %+v", refusals4)
	}
	after4, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterMod) != string(after4) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}

	// Parity oracle: the hand copy with "priority": "five" fails spex
	// validate with the same schema entry.
	handDir := t.TempDir()
	handF := buildNodeEditingFixture(t, handDir)
	handPath := filepath.Join(handDir, "project.json")
	handData, err := os.ReadFile(handPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(handData, &raw); err != nil {
		t.Fatal(err)
	}
	reqs := raw["requirements"].([]any)
	for _, r := range reqs {
		reqMap := r.(map[string]any)
		if reqMap["id"] == handF.p1ID {
			reqMap["priority"] = "five"
		}
	}
	handOut, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(handPath, handOut, 0644); err != nil {
		t.Fatal(err)
	}

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", handDir)
	if err == nil {
		t.Fatal("want validate to fail over the hand-edited non-integer priority")
	}
	valReport := decodeValidationReport(t, valOut)
	var handSchemaMsg string
	for _, e := range valReport.Errors {
		if e.Check == "schema" && strings.Contains(e.Path, "priority") {
			handSchemaMsg = e.Message
		}
	}
	if handSchemaMsg == "" {
		t.Fatalf("want a hand-edit schema error naming priority, got %+v", valReport.Errors)
	}
	if handSchemaMsg != schemaEntry.Message {
		t.Errorf("command schema message %q != hand-edit validator message %q", schemaEntry.Message, handSchemaMsg)
	}
}

// N18: Removing the pending mark once a module derives the requirement.
func TestN18_UnsetPendingDerivation_OnceModuleDerives(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	r2ID := hashID(t, dir, "requirement", "alpha", "R2")
	if out, err := runNodeEditingSpex(t, "node", "add", "R2", "--type", "requirement", "--module", "alpha",
		"--field", "type=functional", "--field", "preq_id="+f.p2ID, "--spec-dir", dir); err != nil {
		t.Fatalf("node add R2: unexpected error: %v\n%s", err, out)
	}
	if out, err := runNodeEditingSpex(t, "edge", "add", f.comp2ID, "implements", r2ID, "--spec-dir", dir); err != nil {
		t.Fatalf("edge add: unexpected error: %v\n%s", err, out)
	}

	if valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir); err != nil {
		t.Fatalf("want the fixture green before the unset (pending_derivation is a note, not an error): %v\n%s", err, valOut)
	}

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	out, err := runNodeEditingSpex(t, "node", "set", f.p2ID, "--unset", "derivation", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)
	if len(report.Obligations) != 0 {
		t.Errorf("Obligations = %+v, want none: derivation is unhashed and moves no requirement leaf", report.Obligations)
	}

	projData, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatal(err)
	}
	var p2 *schema.Requirement
	for i := range proj.Requirements {
		if proj.Requirements[i].ID == f.p2ID {
			p2 = &proj.Requirements[i]
		}
	}
	if p2 == nil {
		t.Fatalf("P2 not found in %+v", proj.Requirements)
	}
	if p2.Derivation != "" {
		t.Errorf("P2.Derivation = %q, want empty", p2.Derivation)
	}
	if p2.Type != "functional" || p2.Priority == nil || *p2.Priority != 2 {
		t.Errorf("P2's other fields must be unchanged, got %+v", p2)
	}

	if _, err := runNodeEditingSpex(t, "validate", "--spec-dir", dir); err != nil {
		t.Fatal("spec should be green with no note after removing the pending mark")
	}

	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr != nil {
		t.Fatalf("diff: unexpected error: %v\n%s", diffErr, diffOut)
	}
	diff := decodeDiffJSON(t, diffOut)
	if len(diff.Changes) != 1 || diff.Changes[0].NodeType != "meta" || diff.Changes[0].Module != "" {
		t.Errorf("want exactly one project-level meta change, got %+v", diff.Changes)
	}
	if len(diff.Errors) != 0 {
		t.Errorf("want no diff errors, got %+v", diff.Errors)
	}

	// A second --unset derivation on P2 is a no-op.
	out2, err := runNodeEditingSpex(t, "node", "set", f.p2ID, "--unset", "derivation", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out2)
	}
	report2 := decodeWriteReport(t, out2)
	if len(report2.Written) != 0 {
		t.Errorf("second unset should be a no-op, got %+v", report2)
	}
	afterProj, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(projData) != string(afterProj) {
		t.Error("a no-op unset must leave project.json byte-identical")
	}
}

// N19: A required field is never unset.
func TestN19_RequiredFieldNeverUnset(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--unset", "type", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for unsetting the required type")
	}
	if exitCodeOf(err) != author.ExitRefusal {
		t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
	}
	refusals := decodeRefusals(t, out)
	var typeEntry *author.RefusalEntry
	for i := range refusals {
		if refusals[i].Check == "schema" && strings.Contains(refusals[i].Fix, "type") {
			typeEntry = &refusals[i]
		}
	}
	if typeEntry == nil {
		t.Fatalf("want the validator's schema refusal for missing type, got %+v", refusals)
	}
	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}

	out2, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--unset", "preq_id", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want a non-zero exit for unsetting the reference field preq_id")
	}
	refusals2 := decodeRefusals(t, out2)
	if len(refusals2) != 1 {
		t.Fatalf("want exactly one refusal (NodeEditor's own ownership guard), got %+v", refusals2)
	}
	if !strings.Contains(refusals2[0].Fix, "spex edge add") || !strings.Contains(refusals2[0].Fix, "spex edge remove") {
		t.Errorf("fix should name spex edge add and spex edge remove, got: %s", refusals2[0].Fix)
	}
	afterMod2, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod2) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}

	// Parity oracle: the same entry with type deleted by hand fails spex
	// validate with the same check value and message.
	handDir := t.TempDir()
	handF := buildNodeEditingFixture(t, handDir)
	handModPath := filepath.Join(handDir, "alpha", "module.json")
	handData, err := os.ReadFile(handModPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(handData, &raw); err != nil {
		t.Fatal(err)
	}
	reqs := raw["requirements"].([]any)
	for _, r := range reqs {
		reqMap := r.(map[string]any)
		if reqMap["id"] == handF.r1ID {
			delete(reqMap, "type")
		}
	}
	handOut, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(handModPath, handOut, 0644); err != nil {
		t.Fatal(err)
	}

	valOut, err := runNodeEditingSpex(t, "validate", "--spec-dir", handDir)
	if err == nil {
		t.Fatal("want validate to fail over the hand-edited missing type")
	}
	valReport := decodeValidationReport(t, valOut)
	var handMsg string
	for _, e := range valReport.Errors {
		if e.Check == "schema" && strings.Contains(e.Message, "type") {
			handMsg = e.Message
		}
	}
	if handMsg == "" {
		t.Fatalf("want a hand-edit schema error naming type, got %+v", valReport.Errors)
	}
	if handMsg != typeEntry.Message {
		t.Errorf("command message %q != hand-edit validator message %q", typeEntry.Message, handMsg)
	}
}

// N20: Identity, derived and reference fields each name the command that
// owns them.
func TestN20_OwnershipRefusalsNameTheirSurface(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)

	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeProj, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"name", []string{f.comp1ID, "--field", "name=Core"}, []string{"spex node rename"}},
		{"id", []string{f.comp1ID, "--field", "id=000000000000"}, []string{"derived"}},
		{"content", []string{f.comp1ID, "--field", "content=arch_other.md"}, []string{"derived"}},
		{"implements", []string{f.comp1ID, "--field", "implements=" + f.r1ID}, []string{"spex edge add", "spex edge remove"}},
		{"preq_id", []string{f.r1ID, "--field", "preq_id=" + f.p2ID}, []string{"spex edge add", "spex edge remove"}},
		{"module id", []string{"000000000001", "--field", "description=anything"}, []string{"project.json", "module.json"}},
		{"set and unset the same field", []string{f.comp1ID, "--field", "description=x", "--unset", "description"}, []string{"both"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"node", "set"}, tc.args...)
			args = append(args, "--spec-dir", dir)
			out, err := runNodeEditingSpex(t, args...)
			if err == nil {
				t.Fatalf("want a non-zero exit for %s, got clean output: %s", tc.name, out)
			}
			if exitCodeOf(err) != author.ExitRefusal {
				t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
			}
			refusals := decodeRefusals(t, out)
			if len(refusals) != 1 {
				t.Fatalf("want exactly one refusal, got %+v", refusals)
			}
			if refusals[0].Path == "" {
				t.Error("refusal should carry a non-empty Path")
			}
			for _, want := range tc.want {
				if !strings.Contains(refusals[0].Fix, want) {
					t.Errorf("fix should contain %q, got: %s", want, refusals[0].Fix)
				}
			}
		})
	}

	t.Run("colour", func(t *testing.T) {
		out, err := runNodeEditingSpex(t, "node", "set", f.comp1ID, "--field", "colour=red", "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for an undeclared field")
		}
		refusals := decodeRefusals(t, out)
		if len(refusals) != 1 {
			t.Fatalf("want exactly one refusal, got %+v", refusals)
		}
		if refusals[0].Check != "schema" {
			t.Errorf("Check = %q, want %q (Report's own check, not an ownership refusal)", refusals[0].Check, "schema")
		}
		for _, want := range []string{"id", "name", "content", "implements", "uses"} {
			if !strings.Contains(refusals[0].Fix, want) {
				t.Errorf("fix should list declared field %q, got: %s", want, refusals[0].Fix)
			}
		}
	})

	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("no case in N20 should have written to alpha/module.json")
	}
	afterProj, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeProj) != string(afterProj) {
		t.Error("no case in N20 should have written to project.json")
	}
}

// N21: The current value is a no-op, and the write is canonical.
func TestN21_NoOpIsCanonicalWrite(t *testing.T) {
	dir := t.TempDir()
	f := buildNodeEditingFixture(t, dir)
	setModuleRequirementDescription(t, dir, f.r1ID, "D")

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())
	beforeMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--field", "description=D", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out)
	}
	report := decodeWriteReport(t, out)
	if len(report.Written) != 0 {
		t.Errorf("Written = %v, want none for a no-op", report.Written)
	}
	if len(report.Obligations) != 0 {
		t.Errorf("Obligations = %+v, want none for a no-op", report.Obligations)
	}

	afterMod, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a no-op set must leave the tree byte-identical")
	}

	diffOut, diffErr := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr != nil {
		t.Fatalf("diff: unexpected error: %v\n%s", diffErr, diffOut)
	}
	diff := decodeDiffJSON(t, diffOut)
	if len(diff.Changes) != 0 {
		t.Errorf("spex diff must report no change for a no-op set, got %+v", diff.Changes)
	}

	// Rewrite the same file by hand as compact, one-line JSON, then make a
	// real change: the write must land in canonical form.
	modPath := filepath.Join(dir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	compact, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, compact, 0644); err != nil {
		t.Fatal(err)
	}

	snapshotTree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, snapshotTree, time.Now())

	out2, err := runNodeEditingSpex(t, "node", "set", f.r1ID, "--field", "description=D2", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node set: unexpected error: %v\n%s", err, out2)
	}
	report2 := decodeWriteReport(t, out2)

	afterData, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(afterData), "{\n  \"") {
		t.Errorf("want the file reformatted to two-space indent, got: %s", afterData[:20])
	}

	diffOut2, diffErr2 := runNodeEditingSpex(t, "diff", "--json", "--spec-dir", dir)
	if diffErr2 == nil {
		t.Fatal("want diff to exit non-zero: the incomplete_change obligation lands in its errors array")
	}
	diff2 := decodeDiffJSON(t, diffOut2)
	var modifiedReqs []diffChange
	for _, c := range diff2.Changes {
		if c.NodeType == "requirement" && c.Type == "modified" {
			modifiedReqs = append(modifiedReqs, c)
		}
	}
	if len(modifiedReqs) != 1 || modifiedReqs[0].Path != f.r1ID {
		t.Errorf("want exactly one modified requirement (R1), got %+v", diff2.Changes)
	}

	completeness := obligationsOfType(report2.Obligations, "incomplete_change")
	if len(completeness) != 1 {
		t.Fatalf("want exactly the one Comp1 obligation, nothing extra from the reformat, got %+v", report2.Obligations)
	}
	if completeness[0].Path != f.r1ID || len(completeness[0].Related) != 1 || completeness[0].Related[0] != f.comp1ID {
		t.Errorf("want the N16 Comp1 obligation, got %+v", completeness[0])
	}

	// The same set over a copy left hand-formatted a different way
	// (four-space indent, as in N13) produces a byte-identical file.
	dir2 := t.TempDir()
	f2 := buildNodeEditingFixture(t, dir2)
	setModuleRequirementDescription(t, dir2, f2.r1ID, "D")
	modPath2 := filepath.Join(dir2, "alpha", "module.json")
	data2, err := os.ReadFile(modPath2)
	if err != nil {
		t.Fatal(err)
	}
	var raw2 map[string]any
	if err := json.Unmarshal(data2, &raw2); err != nil {
		t.Fatal(err)
	}
	fourSpace, err := json.MarshalIndent(raw2, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath2, fourSpace, 0644); err != nil {
		t.Fatal(err)
	}
	tree2, err := merkle.BuildTree(dir2)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir2, tree2, time.Now())

	if out3, err := runNodeEditingSpex(t, "node", "set", f2.r1ID, "--field", "description=D2", "--spec-dir", dir2); err != nil {
		t.Fatalf("node set over the differently hand-formatted copy: unexpected error: %v\n%s", err, out3)
	}
	after2, err := os.ReadFile(modPath2)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterData) != string(after2) {
		t.Errorf("node set over a differently hand-formatted copy produced a different file:\nfirst:\n%s\nsecond:\n%s", afterData, after2)
	}
}
