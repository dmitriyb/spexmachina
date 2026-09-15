package author

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// This file is spec/author/test_migration.md's scenarios M1 through M6,
// driving Migrate directly — AuthorCommands (the `spex migrate` CLI entry
// point) is its own later bead, the same way node_renamer_test.go's TestN8
// and leaf_scaffolder_test.go's L-series drive Rename and Scaffold ahead of
// `spex node rename`/`spex leaf scaffold` existing as commands.
//
// Every fixture starts from buildRenameFixture (obligation_reporter_test.go
// / node_renamer_test.go's shared, already-green setup: module alpha,
// project requirements P1/P2, module requirement R1 deriving from P1, Comp1/
// Comp2, test section T1, api "demo run") and is bent into shape by editing
// the raw JSON on disk — buildRenameFixture's own writeJSON goes through
// schema.Requirement, whose wire tag is already "name" (schema.go's Title
// field), so a pre-versioning "title"-keyed document can only be produced by
// editing the encoded bytes directly, never by marshaling the typed struct.

// retitleRequirements loads path as generic JSON, renames every entry of its
// arrayKey array from "name" to "title" (spec format version 1's one
// deliberate break, undone here to build a pre-versioning fixture), and
// writes it back.
func retitleRequirements(t *testing.T, path, arrayKey string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	arr, ok := doc[arrayKey].([]any)
	if !ok {
		t.Fatalf("%s has no %s array", path, arrayKey)
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		obj["title"] = obj["name"]
		delete(obj, "name")
	}
	writeJSON(t, path, doc)
}

// addRawArray loads path as generic JSON, sets it's top-level key to
// entries, and writes it back — how buildLegacyFixture attaches an
// undeclared array (impl_sections, milestones) no profile-declared type
// owns.
func addRawArray(t *testing.T, path, key string, entries []map[string]any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	doc[key] = entries
	writeJSON(t, path, doc)
}

// stampRawSpecVersion loads path as generic JSON, sets spec_version, and
// writes it back — how buildCurrentFixture builds "tmp/current/".
func stampRawSpecVersion(t *testing.T, path string, version int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	doc["spec_version"] = version
	writeJSON(t, path, doc)
}

// buildTitledFixture is test_migration.md's Setup "tmp/titled/": a spec
// whose requirements, at both scopes, carry title instead of name, and
// whose project.json has no spec_version.
func buildTitledFixture(t *testing.T) obligationFixture {
	t.Helper()
	f := buildRenameFixture(t)
	retitleRequirements(t, filepath.Join(f.dir, "project.json"), "requirements")
	retitleRequirements(t, filepath.Join(f.dir, "alpha", "module.json"), "requirements")
	return f
}

// buildLegacyFixture is test_migration.md's Setup "tmp/legacy/": tmp/titled/
// plus an impl_sections array on module alpha with two entries whose
// content paths resolve to impl_a.md and impl_b.md, and a milestones array
// in project.json.
func buildLegacyFixture(t *testing.T) obligationFixture {
	t.Helper()
	f := buildTitledFixture(t)

	addRawArray(t, filepath.Join(f.dir, "alpha", "module.json"), "impl_sections", []map[string]any{
		{"name": "Section A", "content": "impl_a.md"},
		{"name": "Section B", "content": "impl_b.md"},
	})
	writeFile(t, filepath.Join(f.dir, "alpha", "impl_a.md"), "# Legacy section A\n")
	writeFile(t, filepath.Join(f.dir, "alpha", "impl_b.md"), "# Legacy section B\n")

	addRawArray(t, filepath.Join(f.dir, "project.json"), "milestones", []map[string]any{
		{"name": "Beta", "date": "2024-01-01"},
	})

	return f
}

// buildCurrentFixture is test_migration.md's Setup "tmp/current/": a valid
// spec format version 1 tree, spec_version stamped.
func buildCurrentFixture(t *testing.T) obligationFixture {
	t.Helper()
	f := buildRenameFixture(t)
	stampRawSpecVersion(t, filepath.Join(f.dir, "project.json"), schema.SupportedSpecVersion)
	return f
}

// treeSnapshot reads every file under dir into a map keyed by path relative
// to dir, so a test can assert two states are byte-identical (including
// formatting) without re-deriving the walk each time.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		m[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return m
}

// requirementNames decodes path generically and returns the "name" (or
// "title", when still present) value of every entry in its arrayKey array,
// alongside their ids, so a scenario can check identity survived the rename
// untouched.
func requirementIDs(t *testing.T, path, arrayKey string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	arr, _ := doc[arrayKey].([]any)
	var ids []string
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := obj["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// TestM1_TitleToNameRenameValidates is M1: migrating tmp/titled/ renames
// every requirement's title to name at both scopes, stamps spec_version,
// reports each rename by file and JSON pointer, moves no id, and leaves
// spex validate green.
func TestM1_TitleToNameRenameValidates(t *testing.T) {
	f := buildTitledFixture(t)

	fsys := os.DirFS(f.dir)
	if errs := validator.CheckSchemaFS(fsys); len(errs) == 0 {
		t.Fatal("fixture should fail schema validation before migrate")
	}

	projIDsBefore := requirementIDs(t, filepath.Join(f.dir, "project.json"), "requirements")
	modIDsBefore := requirementIDs(t, filepath.Join(f.dir, "alpha", "module.json"), "requirements")

	report, refusals, err := Migrate(f.dir)
	if err != nil {
		t.Fatalf("Migrate: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Migrate: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("Migrate: want a report, got nil")
	}
	if report.AlreadyCurrent {
		t.Fatal("Migrate: fixture needed migration, got AlreadyCurrent")
	}
	if len(report.Renamed) != 3 {
		t.Fatalf("Renamed = %+v, want 3 entries (P1, P2, R1)", report.Renamed)
	}
	for _, r := range report.Renamed {
		if r.File != "project.json" && r.File != "alpha/module.json" {
			t.Errorf("Renamed entry names unexpected file %q", r.File)
		}
		if r.Pointer == "" {
			t.Errorf("Renamed entry %+v carries no pointer", r)
		}
	}

	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	if strings.Contains(string(projData), `"title"`) {
		t.Errorf("project.json still carries a title key: %s", projData)
	}
	if strings.Contains(string(modData), `"title"`) {
		t.Errorf("alpha/module.json still carries a title key: %s", modData)
	}

	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	if proj.SpecVersion == nil || *proj.SpecVersion != schema.SupportedSpecVersion {
		t.Errorf("project.json spec_version = %v, want %d", proj.SpecVersion, schema.SupportedSpecVersion)
	}

	projIDsAfter := requirementIDs(t, filepath.Join(f.dir, "project.json"), "requirements")
	modIDsAfter := requirementIDs(t, filepath.Join(f.dir, "alpha", "module.json"), "requirements")
	if !reflect.DeepEqual(projIDsBefore, projIDsAfter) {
		t.Errorf("project requirement ids changed: before %v, after %v", projIDsBefore, projIDsAfter)
	}
	if !reflect.DeepEqual(modIDsBefore, modIDsAfter) {
		t.Errorf("module requirement ids changed: before %v, after %v", modIDsBefore, modIDsAfter)
	}

	assertValidateGreen(t, f.dir)
}

// TestM2_UndeclaredArraysRemovedAndOrphansReported is M2: migrating
// tmp/legacy/ removes impl_sections from alpha/module.json and milestones
// from project.json, reports each removed entry by array, name and (when it
// had one) orphaned content path, leaves the orphaned files on disk
// untouched, and leaves spex validate green.
func TestM2_UndeclaredArraysRemovedAndOrphansReported(t *testing.T) {
	f := buildLegacyFixture(t)

	report, refusals, err := Migrate(f.dir)
	if err != nil {
		t.Fatalf("Migrate: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Migrate: unexpected refusals: %+v", refusals)
	}
	if report == nil || report.AlreadyCurrent {
		t.Fatalf("Migrate: want a real migration report, got %+v", report)
	}

	modData, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	if strings.Contains(string(modData), "impl_sections") {
		t.Errorf("alpha/module.json still carries impl_sections: %s", modData)
	}
	projData, err := os.ReadFile(filepath.Join(f.dir, "project.json"))
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	if strings.Contains(string(projData), "milestones") {
		t.Errorf("project.json still carries milestones: %s", projData)
	}

	want := []RemovedEntry{
		{File: "alpha/module.json", Array: "impl_sections", Name: "Section A", Content: "alpha/impl_a.md"},
		{File: "alpha/module.json", Array: "impl_sections", Name: "Section B", Content: "alpha/impl_b.md"},
		{File: "project.json", Array: "milestones", Name: "Beta"},
	}
	if len(report.Removed) != len(want) {
		t.Fatalf("Removed = %+v, want %+v", report.Removed, want)
	}
	for _, w := range want {
		found := false
		for _, got := range report.Removed {
			if reflect.DeepEqual(got, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Removed missing entry %+v, got %+v", w, report.Removed)
		}
	}

	for _, orphan := range []string{"impl_a.md", "impl_b.md"} {
		p := filepath.Join(f.dir, "alpha", orphan)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("orphaned file %s should still exist on disk: %v", p, err)
		}
	}

	assertValidateGreen(t, f.dir)
}

// TestM3_SecondRunIsNoOp is M3: running Migrate again over a tree M1 already
// migrated changes nothing — AlreadyCurrent, no writes, byte-identical tree.
func TestM3_SecondRunIsNoOp(t *testing.T) {
	f := buildTitledFixture(t)

	if _, refusals, err := Migrate(f.dir); err != nil {
		t.Fatalf("first Migrate: unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("first Migrate: unexpected refusals: %+v", refusals)
	}

	before := treeSnapshot(t, f.dir)

	report, refusals, err := Migrate(f.dir)
	if err != nil {
		t.Fatalf("second Migrate: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("second Migrate: unexpected refusals: %+v", refusals)
	}
	if report == nil || !report.AlreadyCurrent {
		t.Fatalf("second Migrate: want AlreadyCurrent, got %+v", report)
	}
	if len(report.Written) != 0 || len(report.Renamed) != 0 || len(report.Removed) != 0 {
		t.Errorf("second Migrate: want an empty report beyond AlreadyCurrent, got %+v", report)
	}

	after := treeSnapshot(t, f.dir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("second Migrate rewrote the tree: before %v, after %v", before, after)
	}
}

// TestM4_CurrentSpecUntouched is M4: migrating an already-current tree
// reports the same AlreadyCurrent outcome as M3 and leaves the tree
// byte-identical, including formatting, since a no-op writes nothing.
func TestM4_CurrentSpecUntouched(t *testing.T) {
	f := buildCurrentFixture(t)
	before := treeSnapshot(t, f.dir)

	report, refusals, err := Migrate(f.dir)
	if err != nil {
		t.Fatalf("Migrate: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Migrate: unexpected refusals: %+v", refusals)
	}
	if report == nil || !report.AlreadyCurrent {
		t.Fatalf("Migrate: want AlreadyCurrent, got %+v", report)
	}

	after := treeSnapshot(t, f.dir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Migrate rewrote an already-current tree: before %v, after %v", before, after)
	}
}

// TestM5_UnmigratableAuthoringDefectStillMigrates is M5: a module
// requirement that will still fail validation after migration (missing
// preq_id) does not block the rename — Migrate exits clean, performs the
// rename, and the remaining defect surfaces only once spex validate runs
// separately, as the one finding left: missing preq_id, nothing else.
func TestM5_UnmigratableAuthoringDefectStillMigrates(t *testing.T) {
	f := buildTitledFixture(t)

	modPath := filepath.Join(f.dir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("read alpha/module.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}
	arr := doc["requirements"].([]any)
	r1 := arr[0].(map[string]any)
	if r1["id"] != f.r1ID {
		t.Fatalf("fixture assumption broken: first module requirement is not R1: %+v", r1)
	}
	delete(r1, "preq_id")
	writeJSON(t, modPath, doc)

	// R1 was P1's only derivation; marking P1 "pending" keeps
	// requirement_coverage from also firing on this fixture, so the only
	// authoring defect left standing is the one this scenario is actually
	// about — R1's missing preq_id, not an incidental coverage gap this
	// specific test setup would otherwise introduce.
	projPath := filepath.Join(f.dir, "project.json")
	projData, err := os.ReadFile(projPath)
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	var projDoc map[string]any
	if err := json.Unmarshal(projData, &projDoc); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	p1 := projDoc["requirements"].([]any)[0].(map[string]any)
	if p1["id"] != f.p1ID {
		t.Fatalf("fixture assumption broken: first project requirement is not P1: %+v", p1)
	}
	p1["derivation"] = "pending"
	writeJSON(t, projPath, projDoc)

	report, refusals, err := Migrate(f.dir)
	if err != nil {
		t.Fatalf("Migrate: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Migrate: unexpected refusals: %+v", refusals)
	}
	if report == nil || report.AlreadyCurrent {
		t.Fatalf("Migrate: want a real migration report, got %+v", report)
	}

	renamedR1 := false
	for _, r := range report.Renamed {
		if r.File == "alpha/module.json" {
			renamedR1 = true
		}
	}
	if !renamedR1 {
		t.Errorf("Migrate: R1 should still be renamed despite the missing preq_id, got %+v", report.Renamed)
	}

	modData, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("read alpha/module.json after migrate: %v", err)
	}
	if strings.Contains(string(modData), `"title"`) {
		t.Errorf("alpha/module.json still carries a title key after migrate: %s", modData)
	}

	fsys := os.DirFS(f.dir)
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

	// Removing preq_id is one authoring defect, but two independent
	// checkers both name it — schema's required-property check and
	// IDValidator's own preq_id presence check (id_validator.go) — so
	// "nothing else" means every finding traces to preq_id, not that
	// exactly one ValidationError comes back.
	if len(errs) == 0 {
		t.Fatal("spex validate after migrate found nothing; want the missing preq_id defect")
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "preq_id") {
			t.Errorf("spex validate found an unrelated finding %+v; migrate should leave nothing standing but the missing preq_id", e)
		}
	}
}

// TestM6_NoInitNeededTouchesNothingOutsideSpec is M6: Migrate needs no
// initialised project (a bare fixture with no .spex/ migrates cleanly), and
// touches nothing outside the spec directory it was given (a healthy .spex/
// sitting beside an adopter's spec is byte-identical before and after). It
// also asserts M6's central claim directly: both runs exit 0 with
// byte-identical trees under spec/, so the migration's output does not
// depend on whether a .spex/ happens to sit beside it — bare and specDir
// are independent copies of the same buildLegacyFixture fixture, so a
// divergence here would mean Migrate reads or is influenced by something
// outside the spec directory it was given.
func TestM6_NoInitNeededTouchesNothingOutsideSpec(t *testing.T) {
	bare := buildLegacyFixture(t)
	if _, refusals, err := Migrate(bare.dir); err != nil {
		t.Fatalf("Migrate (no .spex/ anywhere): unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("Migrate (no .spex/ anywhere): unexpected refusals: %+v", refusals)
	}
	assertValidateGreen(t, bare.dir)
	bareTree := treeSnapshot(t, bare.dir)

	adopter := buildLegacyFixture(t)
	specDir := copyTree(t, adopter.dir)
	spexDir := filepath.Join(filepath.Dir(specDir), ".spex")
	if err := os.MkdirAll(spexDir, 0755); err != nil {
		t.Fatalf("mkdir .spex/: %v", err)
	}
	writeFile(t, filepath.Join(spexDir, "snapshot.json"), `{"fake":"snapshot"}`)
	before := treeSnapshot(t, spexDir)

	if _, refusals, err := Migrate(specDir); err != nil {
		t.Fatalf("Migrate (adopter with .spex/): unexpected error: %v", err)
	} else if len(refusals) > 0 {
		t.Fatalf("Migrate (adopter with .spex/): unexpected refusals: %+v", refusals)
	}

	after := treeSnapshot(t, spexDir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf(".spex/ changed across Migrate: before %v, after %v", before, after)
	}
	assertValidateGreen(t, specDir)

	specTree := treeSnapshot(t, specDir)
	if !reflect.DeepEqual(bareTree, specTree) {
		t.Errorf("migrated trees under spec/ differ between the no-.spex/ run and the .spex/-adjacent run: bare %v, adopter %v", bareTree, specTree)
	}
}
