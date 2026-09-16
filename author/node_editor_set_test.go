package author

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// setProjectRequirementPriority overwrites id's priority in dir's
// project.json directly — buildRenameFixture gives every project
// requirement priority 2, and N17 needs P1 to start at 1.
func setProjectRequirementPriority(t *testing.T, dir, id string, priority int) {
	t.Helper()
	path := filepath.Join(dir, "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	for i := range proj.Requirements {
		if proj.Requirements[i].ID == id {
			proj.Requirements[i].Priority = &priority
		}
	}
	writeJSON(t, path, proj)
}

// setModuleRequirementDescription overwrites id's description in dir's
// alpha/module.json directly — N21 needs a known starting value for the
// no-op half of its scenario.
func setModuleRequirementDescription(t *testing.T, dir, id, desc string) {
	t.Helper()
	path := filepath.Join(dir, "alpha", "module.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}
	for i := range mod.Requirements {
		if mod.Requirements[i].ID == id {
			mod.Requirements[i].Description = desc
		}
	}
	writeJSON(t, path, mod)
}

// TestN16_SetModuleRequirementDescription_ObligesImplementingLeaf covers
// test_node_editing.md's N16: setting R1's description rewrites its entry
// in alpha/module.json in place, leaves every other field and project.json
// untouched, and the write report's obligations hold exactly the
// requirement-changed rule's one entry naming Comp1 — never a meta
// obligation for Comp2, since a requirement in the module changed.
func TestN16_SetModuleRequirementDescription_ObligesImplementingLeaf(t *testing.T) {
	f := buildRenameFixture(t)

	beforeProj, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Set(f.dir, NodeSetInput{
		ID:     f.r1ID,
		Fields: map[string]string{"description": "R1's new description"},
	})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Set: want a report, got nil")
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
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

	afterProj, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
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
}

// TestN17_SetProjectRequirementPriority_WalksDownToImplementor covers
// test_node_editing.md's N17: a project requirement's priority lands in
// project.json as a JSON number, and the obligation is the completeness
// walk down through R1 to Comp1. --unset on a required-by-validator field
// is refused with the validator's own "id" presence-check entry; a
// non-integer value is refused through the validator's own "schema" entry,
// never a Go input error; and the same holds for a module requirement's
// enumerated "type" field.
func TestN17_SetProjectRequirementPriority_WalksDownToImplementor(t *testing.T) {
	f := buildRenameFixture(t)
	setProjectRequirementPriority(t, f.dir, f.p1ID, 1)

	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Set(f.dir, NodeSetInput{ID: f.p1ID, Fields: map[string]string{"priority": "2"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Set: want a report, got nil")
	}

	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
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

	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
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

	settledProj := projData

	// --unset priority is refused with the tree unchanged: the schema
	// leaves the field optional, but the validator's own presence check
	// (the "id" check) does not.
	report2, refusals2, err := Set(f.dir, NodeSetInput{ID: f.p1ID, Unset: []string{"priority"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if report2 != nil {
		t.Fatalf("Set: want no report on refusal, got %+v", report2)
	}
	if len(refusals2) == 0 {
		t.Fatal("Set: want a refusal for unsetting a project requirement's priority")
	}
	var foundPriorityRefusal bool
	for _, r := range refusals2 {
		if r.Check == "id" && strings.Contains(r.Message, "missing priority") {
			foundPriorityRefusal = true
		}
	}
	if !foundPriorityRefusal {
		t.Fatalf("want the validator's id-check refusal for missing priority, got %+v", refusals2)
	}
	after2, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(settledProj) != string(after2) {
		t.Error("a refusal must leave project.json byte-identical")
	}

	// --field priority=five is refused through the validator's own schema
	// entry, not a Go input error: the malformed value is written to the
	// entry so Report's schema check catches the type mismatch.
	report3, refusals3, err := Set(f.dir, NodeSetInput{ID: f.p1ID, Fields: map[string]string{"priority": "five"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if report3 != nil {
		t.Fatalf("Set: want no report on refusal, got %+v", report3)
	}
	if len(refusals3) == 0 {
		t.Fatal("Set: want a refusal for a non-integer priority")
	}
	var foundSchemaRefusal bool
	for _, r := range refusals3 {
		if r.Check == "schema" && strings.Contains(r.Fix, "priority") && strings.Contains(r.Fix, "integer") {
			foundSchemaRefusal = true
		}
	}
	if !foundSchemaRefusal {
		t.Fatalf("want the validator's schema refusal naming priority as an integer, got %+v", refusals3)
	}
	after3, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(settledProj) != string(after3) {
		t.Error("a refusal must leave project.json byte-identical")
	}

	// R1's enumerated "type" is refused the same way, naming the
	// enumeration.
	report4, refusals4, err := Set(f.dir, NodeSetInput{ID: f.r1ID, Fields: map[string]string{"type": "optional"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if report4 != nil {
		t.Fatalf("Set: want no report on refusal, got %+v", report4)
	}
	if len(refusals4) == 0 {
		t.Fatal("Set: want a refusal for an out-of-enumeration type")
	}
	var foundEnumRefusal bool
	for _, r := range refusals4 {
		if r.Check == "schema" && strings.Contains(r.Fix, "functional") && strings.Contains(r.Fix, "non_functional") {
			foundEnumRefusal = true
		}
	}
	if !foundEnumRefusal {
		t.Fatalf("want the validator's schema refusal listing the enumeration, got %+v", refusals4)
	}
	after4, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterMod) != string(after4) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}
}

// TestN18_UnsetPendingDerivation_OnceModuleDerives covers test_node_editing.md's
// N18: once a module requirement derives P2 (preq_id) and a component
// implements it, unsetting P2's derivation mark changes project.json but
// obliges nothing — the default profile declares derivation unhashed, so no
// requirement leaf moved — and spex diff against a snapshot taken after the
// setup reports only the project meta envelope leaf changed.
func TestN18_UnsetPendingDerivation_OnceModuleDerives(t *testing.T) {
	f := buildRenameFixture(t)

	r2ID := schema.IdentityHash("alpha", "requirement", "R2")
	if _, refusals, err := Add(f.dir, NodeAddInput{
		TypeName: "requirement", Module: "alpha", Name: "R2",
		Fields: map[string]string{"type": "functional", "preq_id": f.p2ID},
	}); err != nil {
		t.Fatalf("Add(R2): unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("Add(R2): unexpected refusals: %+v", refusals)
	}

	if _, refusals, err := AddEdge(f.dir, EdgeInput{SourceID: f.comp2ID, Field: "implements", TargetID: r2ID}); err != nil {
		t.Fatalf("AddEdge: unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("AddEdge: unexpected refusals: %+v", refusals)
	}

	if errs := allValidatorErrors(t, f.dir); len(errs) != 0 {
		t.Fatalf("want the fixture green before the unset (pending_derivation is a note, not an error), got %+v", errs)
	}

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Set(f.dir, NodeSetInput{ID: f.p2ID, Unset: []string{"derivation"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Set: want a report, got nil")
	}
	if len(report.Obligations) != 0 {
		t.Errorf("Obligations = %+v, want none: derivation is unhashed and moves no requirement leaf", report.Obligations)
	}

	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatal(err)
	}
	for _, r := range proj.Requirements {
		if r.ID == f.p2ID && r.Derivation != "" {
			t.Errorf("P2.Derivation = %q, want empty", r.Derivation)
		}
	}

	if errs := allValidatorErrors(t, f.dir); len(errs) != 0 {
		t.Errorf("spec is not green after removing the pending mark: %+v", errs)
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	changes := merkle.Diff(afterTree, beforeTree)
	if len(changes) != 1 || changes[0].NodeType != "meta" || changes[0].Module != "" {
		t.Errorf("want exactly one project-level meta change, got %+v", changes)
	}

	// A second --unset derivation on P2 is a no-op.
	report2, refusals2, err := Set(f.dir, NodeSetInput{ID: f.p2ID, Unset: []string{"derivation"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals2) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals2)
	}
	if report2 == nil || len(report2.Written) != 0 {
		t.Errorf("second unset should be a no-op, got %+v", report2)
	}
}

// TestN19_RequiredFieldNeverUnset covers test_node_editing.md's N19:
// unsetting a required declared field is refused with the validator's own
// schema entry for the field it would leave missing. Unsetting a reference
// field (R1's preq_id) is refused first as an ownership refusal, by N20's
// rule, before requiredness is ever considered.
func TestN19_RequiredFieldNeverUnset(t *testing.T) {
	f := buildRenameFixture(t)

	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Set(f.dir, NodeSetInput{ID: f.r1ID, Unset: []string{"type"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Set: want no report on refusal, got %+v", report)
	}
	if len(refusals) == 0 {
		t.Fatal("Set: want a refusal for unsetting the required type")
	}
	var found bool
	for _, r := range refusals {
		if r.Check == "schema" && strings.Contains(r.Fix, "type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want the validator's schema refusal for missing type, got %+v", refusals)
	}
	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}

	report2, refusals2, err := Set(f.dir, NodeSetInput{ID: f.r1ID, Unset: []string{"preq_id"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if report2 != nil {
		t.Fatalf("Set: want no report on refusal, got %+v", report2)
	}
	if len(refusals2) != 1 {
		t.Fatalf("Set: want exactly one refusal (NodeEditor's own ownership guard), got %+v", refusals2)
	}
	if !strings.Contains(refusals2[0].Fix, "spex edge add") || !strings.Contains(refusals2[0].Fix, "spex edge remove") {
		t.Errorf("fix should name spex edge add and spex edge remove, got: %s", refusals2[0].Fix)
	}
	afterMod2, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod2) {
		t.Error("a refusal must leave alpha/module.json byte-identical")
	}
}

// TestN20_OwnershipRefusalsNameTheirSurface covers test_node_editing.md's
// N20: identity, derived and reference fields each refuse naming the
// surface that owns them, before any file is touched — an ownership
// refusal NodeEditor raises itself, never the validator's. "colour", an
// undeclared field, is the one exception: it is refused through Report's
// own schema check, the same one N4's parity oracle would find in a hand
// copy.
func TestN20_OwnershipRefusalsNameTheirSurface(t *testing.T) {
	f := buildRenameFixture(t)

	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeProj, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		input NodeSetInput
		want  []string
	}{
		{"name", NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"name": "Core"}}, []string{"spex node rename"}},
		{"id", NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"id": "000000000000"}}, []string{"derived"}},
		{"content", NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"content": "arch_other.md"}}, []string{"derived"}},
		{"implements", NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"implements": f.r1ID}}, []string{"spex edge add", "spex edge remove"}},
		{"preq_id", NodeSetInput{ID: f.r1ID, Fields: map[string]string{"preq_id": f.p2ID}}, []string{"spex edge add", "spex edge remove"}},
		{"module id", NodeSetInput{ID: "000000000001", Fields: map[string]string{"description": "anything"}}, []string{"project.json", "module.json"}},
		{"set and unset the same field", NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"description": "x"}, Unset: []string{"description"}}, []string{"both"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, refusals, err := Set(f.dir, tc.input)
			if err != nil {
				t.Fatalf("Set: unexpected error: %v", err)
			}
			if report != nil {
				t.Fatalf("Set: want no report on refusal, got %+v", report)
			}
			if len(refusals) != 1 {
				t.Fatalf("Set: want exactly one refusal, got %+v", refusals)
			}
			for _, want := range tc.want {
				if !strings.Contains(refusals[0].Fix, want) {
					t.Errorf("fix should contain %q, got: %s", want, refusals[0].Fix)
				}
			}
		})
	}

	t.Run("colour", func(t *testing.T) {
		report, refusals, err := Set(f.dir, NodeSetInput{ID: f.comp1ID, Fields: map[string]string{"colour": "red"}})
		if err != nil {
			t.Fatalf("Set: unexpected error: %v", err)
		}
		if report != nil {
			t.Fatalf("Set: want no report on refusal, got %+v", report)
		}
		if len(refusals) != 1 {
			t.Fatalf("Set: want exactly one refusal, got %+v", refusals)
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

	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("no case in N20 should have written to alpha/module.json")
	}
	afterProj, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeProj) != string(afterProj) {
		t.Error("no case in N20 should have written to project.json")
	}
}

// TestN21_NoOpIsCanonicalWrite covers test_node_editing.md's N21: setting a
// field to the value it already holds is a byte-identical no-op with empty
// obligations; over a hand-formatted file, a real change is still written
// in canonical form, and the reformat itself moves no requirement leaf
// hash — the obligations are exactly N16's, nothing extra from the reformat.
func TestN21_NoOpIsCanonicalWrite(t *testing.T) {
	f := buildRenameFixture(t)
	setModuleRequirementDescription(t, f.dir, f.r1ID, "D")

	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	beforeMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, refusals, err := Set(f.dir, NodeSetInput{ID: f.r1ID, Fields: map[string]string{"description": "D"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Set: want a report, got nil")
	}
	if len(report.Written) != 0 {
		t.Errorf("Written = %v, want none for a no-op", report.Written)
	}
	if len(report.Obligations) != 0 {
		t.Errorf("Obligations = %+v, want none for a no-op", report.Obligations)
	}

	afterMod, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeMod) != string(afterMod) {
		t.Error("a no-op set must leave the tree byte-identical")
	}

	afterTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if changes := merkle.Diff(afterTree, beforeTree); len(changes) != 0 {
		t.Errorf("spex diff must report no change for a no-op set, got %+v", changes)
	}

	// Rewrite the same file by hand as compact, alphabetically-keyed JSON,
	// then make a real change: the write must land in canonical form.
	modPath := filepath.Join(f.dir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	handFormatted, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, modPath, string(handFormatted))

	snapshotTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}

	report2, refusals2, err := Set(f.dir, NodeSetInput{ID: f.r1ID, Fields: map[string]string{"description": "D2"}})
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if len(refusals2) > 0 {
		t.Fatalf("Set: unexpected refusals: %+v", refusals2)
	}
	if report2 == nil {
		t.Fatal("Set: want a report, got nil")
	}

	afterData, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterData), "\n  \"") {
		t.Errorf("want the file reformatted to two-space indent, got: %s", afterData)
	}

	afterTree2, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	changes2 := merkle.Diff(afterTree2, snapshotTree)
	var modifiedReqs []merkle.Change
	for _, c := range changes2 {
		if c.NodeType == "requirement" && c.Type == merkle.Modified {
			modifiedReqs = append(modifiedReqs, c)
		}
	}
	if len(modifiedReqs) != 1 || modifiedReqs[0].Key != f.r1ID {
		t.Errorf("want exactly one modified requirement (R1), got %+v", modifiedReqs)
	}

	completeness := obligationsOfType(report2.Obligations, "incomplete_change")
	if len(completeness) != 1 {
		t.Fatalf("want exactly the one Comp1 obligation, nothing extra from the reformat, got %+v", report2.Obligations)
	}
	if completeness[0].Path != f.r1ID || len(completeness[0].Related) != 1 || completeness[0].Related[0] != f.comp1ID {
		t.Errorf("want the N16 Comp1 obligation, got %+v", completeness[0])
	}
}

// --- Unit tests for Set's own helpers ---

func TestConvertSetFieldValue(t *testing.T) {
	intField := schema.Field{Name: "priority", Kind: schema.FieldKindInteger}
	if v := convertSetFieldValue(intField, "3"); v != 3 {
		t.Errorf("integer field: got %v, want 3", v)
	}
	if v := convertSetFieldValue(intField, "bogus"); v != "bogus" {
		t.Errorf("integer field, unparseable value: got %v, want the raw string", v)
	}

	textField := schema.Field{Name: "type", Kind: schema.FieldKindText}
	if v := convertSetFieldValue(textField, "functional"); v != "functional" {
		t.Errorf("text field: got %v, want functional", v)
	}

	if v := convertSetFieldValue(schema.Field{}, "anything"); v != "anything" {
		t.Errorf("zero-value field (description or undeclared): got %v, want anything", v)
	}
}

func TestSetValueUnchanged(t *testing.T) {
	if !setValueUnchanged(float64(2), 2) {
		t.Error("want int 2 to match an existing JSON-decoded float64 2")
	}
	if setValueUnchanged(float64(2), 3) {
		t.Error("want int 3 not to match an existing float64 2")
	}
	if !setValueUnchanged("D", "D") {
		t.Error("want equal strings to match")
	}
	if setValueUnchanged("D", "D2") {
		t.Error("want different strings not to match")
	}
	if setValueUnchanged(nil, 2) {
		t.Error("want a missing existing value not to match")
	}
}

func TestFieldOwnershipRefusal(t *testing.T) {
	nt := schema.NodeType{
		Name:            "component",
		RequiresContent: true,
		Fields: []schema.Field{
			{Name: "implements", Kind: schema.FieldKindReference},
		},
	}
	loc := nodeLocation{ownerFile: "alpha/module.json", nodeType: nt}

	for _, name := range []string{"name", "id", "content", "implements"} {
		if _, owned := fieldOwnershipRefusal(name, loc, "abc123456789"); !owned {
			t.Errorf("%q should be an ownership refusal", name)
		}
	}
	if _, owned := fieldOwnershipRefusal("colour", loc, "abc123456789"); owned {
		t.Error("an undeclared field must not be an ownership refusal")
	}

	noContentLoc := nodeLocation{ownerFile: "project.json", nodeType: schema.NodeType{Name: "requirement"}}
	if _, owned := fieldOwnershipRefusal("content", noContentLoc, "abc123456789"); owned {
		t.Error("content should not be an ownership refusal on a type that carries no content")
	}
}
