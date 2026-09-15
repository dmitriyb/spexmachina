package author

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// This file is spec/author/test_leaf_and_profile.md's LeafScaffolder half —
// L1 through L4 and L7, the scenarios reachable by calling Scaffold
// directly. Every scenario in that leaf names `spex leaf scaffold`, but
// AuthorCommands (the CLI entry point for it) is its own later bead, so
// these scenarios drive the worker function the same way
// node_renamer_test.go's TestN8 drives Rename ahead of `spex node rename`
// existing as a command. L5 and L6 (ProfileInspector-only) are in
// cmd/spex/leaf_and_profile_test.go; L8 (spex node add scaffolding through
// the same path) needs NodeEditor and belongs to its bead.

// assertValidateGreen runs every checker `spex validate` runs over dir and
// fails the test if any reports an error — the ten-checker sweep
// node_renamer_test.go's TestN8 also inlines, since no shared helper for it
// exists yet in this package.
func assertValidateGreen(t *testing.T, dir string) {
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
	if len(errs) > 0 {
		t.Fatalf("spec is not green: %+v", errs)
	}
}

// orderedIndexOf fails the test unless each of substrs appears in s, in the
// given order (each search starts after the previous match) — the shape
// L1 and L4 need for "carries in order the ## headings".
func assertInOrder(t *testing.T, s string, substrs ...string) {
	t.Helper()
	pos := 0
	for _, sub := range substrs {
		idx := strings.Index(s[pos:], sub)
		if idx < 0 {
			t.Fatalf("want %q after position %d in:\n%s", sub, pos, s)
		}
		pos += idx + len(sub)
	}
}

// TestREQ_1631cb19fac3_L1_SkeletonCarriesHeadingsAndOwedPlaceholder is L1:
// deleting a declared component's leaf and scaffolding it writes the
// title, the component type's leaf_sections headings in order, and one
// placeholder line carrying a typed link to the one node Comp2's own
// `uses` field names.
func TestREQ_1631cb19fac3_L1_SkeletonCarriesHeadingsAndOwedPlaceholder(t *testing.T) {
	f := buildRenameFixture(t)
	comp2Path := filepath.Join(f.dir, "alpha", "arch_comp2.md")
	if err := os.Remove(comp2Path); err != nil {
		t.Fatalf("remove arch_comp2.md: %v", err)
	}

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID})
	if err != nil {
		t.Fatalf("Scaffold: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Scaffold: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 1 {
		t.Fatalf("Scaffold: want a report writing exactly one file, got %+v", report)
	}
	if len(report.Obligations) != 0 {
		t.Errorf("Scaffold: scaffolding a leaf should oblige nothing, got %+v", report.Obligations)
	}

	data, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "# Comp2\n") {
		t.Fatalf("arch_comp2.md does not open with '# Comp2': %s", content)
	}
	assertInOrder(t, content, "## Responsibilities", "## Interface")

	wantLink := "[[" + f.comp1ID + "|Comp1]]"
	if strings.Count(content, wantLink) != 1 {
		t.Fatalf("arch_comp2.md wants exactly one placeholder link %s, got:\n%s", wantLink, content)
	}

	assertValidateGreen(t, f.dir)
}

// TestREQ_1631cb19fac3_OwedEdgesAllThreeSources covers the two sources L1
// leaves untested: arch_leaf_scaffolder.md's "What a skeleton is" names
// three sources for a component's owed links — its own `implements`, its
// own `uses`, and every api whose `provided_by` names it. Comp1 declares
// `implements` (R1) and no `uses`, and the fixture's one api ("demo run")
// is provided_by Comp1 — the only non-content-bearing type in the default
// profile — so scaffolding Comp1 exercises both the outbound `implements`
// sweep and the inbound `provided_by` sweep owedEdges runs over every
// non-content-bearing type (leaf_scaffolder.go:196-237).
func TestREQ_1631cb19fac3_OwedEdgesAllThreeSources(t *testing.T) {
	f := buildRenameFixture(t)
	comp1Path := filepath.Join(f.dir, "alpha", "arch_comp1.md")
	if err := os.Remove(comp1Path); err != nil {
		t.Fatalf("remove arch_comp1.md: %v", err)
	}

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp1ID})
	if err != nil {
		t.Fatalf("Scaffold: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Scaffold: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 1 {
		t.Fatalf("Scaffold: want a report writing exactly one file, got %+v", report)
	}

	data, err := os.ReadFile(comp1Path)
	if err != nil {
		t.Fatalf("read arch_comp1.md: %v", err)
	}
	content := string(data)

	wantImplements := "[[" + f.r1ID + "|R1]]"
	if strings.Count(content, wantImplements) != 1 {
		t.Fatalf("arch_comp1.md wants exactly one outbound implements placeholder %s, got:\n%s", wantImplements, content)
	}
	wantProvidedBy := "[[" + f.apiID + "|demo run]]"
	if strings.Count(content, wantProvidedBy) != 1 {
		t.Fatalf("arch_comp1.md wants exactly one inbound provided_by placeholder %s, got:\n%s", wantProvidedBy, content)
	}

	assertValidateGreen(t, f.dir)
}

// TestREQ_1631cb19fac3_MissingLeafElsewhereStillObliged guards against
// patchMissingLeaves' tree-building patch leaking into the content check:
// it fakes every declared-but-missing leaf present so merkle.BuildTreeFS
// can hash the tree, and that fake must not also hide a genuinely missing
// leaf from the obligations Report prints. Deleting both arch_comp2.md and
// test_t1.md and scaffolding Comp2 writes only arch_comp2.md; test_t1.md
// stays missing and must still surface as a content obligation the way
// `spex validate` would report it, per arch_obligation_reporter.md's "a
// finding the input already carried is reported as an obligation."
func TestREQ_1631cb19fac3_MissingLeafElsewhereStillObliged(t *testing.T) {
	f := buildRenameFixture(t)
	if err := os.Remove(filepath.Join(f.dir, "alpha", "arch_comp2.md")); err != nil {
		t.Fatalf("remove arch_comp2.md: %v", err)
	}
	t1Path := filepath.Join(f.dir, "alpha", "test_t1.md")
	if err := os.Remove(t1Path); err != nil {
		t.Fatalf("remove test_t1.md: %v", err)
	}

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID})
	if err != nil {
		t.Fatalf("Scaffold: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Scaffold: unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 1 {
		t.Fatalf("Scaffold: want a report writing exactly one file, got %+v", report)
	}

	var found bool
	for _, ob := range report.Obligations {
		if ob.Type == "content" && strings.Contains(ob.Message, "test_t1.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Scaffold: want an obligation naming test_t1.md as missing, got %+v", report.Obligations)
	}

	if _, err := os.Stat(t1Path); err == nil {
		t.Fatal("test_t1.md should not have been written by scaffolding Comp2")
	}
}

// TestREQ_1631cb19fac3_L2_NonEmptyLeafNeverOverwritten is L2: a leaf
// holding prose is refused, byte-identical, with a fix naming the file; the
// same node over a present-but-empty leaf is written.
func TestREQ_1631cb19fac3_L2_NonEmptyLeafNeverOverwritten(t *testing.T) {
	t.Run("prose is refused and left byte-identical", func(t *testing.T) {
		f := buildRenameFixture(t)
		comp2Path := filepath.Join(f.dir, "alpha", "arch_comp2.md")
		before, err := os.ReadFile(comp2Path)
		if err != nil {
			t.Fatalf("read arch_comp2.md: %v", err)
		}

		report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID})
		if err != nil {
			t.Fatalf("Scaffold: unexpected error: %v", err)
		}
		if report != nil {
			t.Fatalf("Scaffold: want no report on refusal, got %+v", report)
		}
		if len(refusals) != 1 {
			t.Fatalf("Scaffold: want exactly one refusal, got %+v", refusals)
		}
		if !strings.Contains(refusals[0].Message, comp2Path) && !strings.Contains(refusals[0].Path, "arch_comp2.md") {
			t.Errorf("refusal does not name the file: %+v", refusals[0])
		}
		if !strings.Contains(refusals[0].Fix, "empty") {
			t.Errorf("refusal fix does not say to empty or move the file: %+v", refusals[0])
		}

		after, err := os.ReadFile(comp2Path)
		if err != nil {
			t.Fatalf("read arch_comp2.md after refusal: %v", err)
		}
		if string(before) != string(after) {
			t.Fatalf("arch_comp2.md changed on a refused scaffold:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("a present but zero-byte leaf is written", func(t *testing.T) {
		f := buildRenameFixture(t)
		comp2Path := filepath.Join(f.dir, "alpha", "arch_comp2.md")
		if err := os.WriteFile(comp2Path, nil, 0644); err != nil {
			t.Fatalf("truncate arch_comp2.md: %v", err)
		}

		report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID})
		if err != nil {
			t.Fatalf("Scaffold: unexpected error: %v", err)
		}
		if len(refusals) > 0 {
			t.Fatalf("Scaffold: unexpected refusals: %+v", refusals)
		}
		if report == nil || len(report.Written) != 1 {
			t.Fatalf("Scaffold: want a report writing exactly one file, got %+v", report)
		}

		data, err := os.ReadFile(comp2Path)
		if err != nil {
			t.Fatalf("read arch_comp2.md: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("arch_comp2.md is still empty after scaffolding")
		}
	})
}

// TestREQ_1631cb19fac3_L3_ScaffoldingIsIdempotent is L3: scaffolding a leaf
// that already carries exactly the skeleton changes nothing.
func TestREQ_1631cb19fac3_L3_ScaffoldingIsIdempotent(t *testing.T) {
	f := buildRenameFixture(t)
	comp2Path := filepath.Join(f.dir, "alpha", "arch_comp2.md")
	if err := os.Remove(comp2Path); err != nil {
		t.Fatalf("remove arch_comp2.md: %v", err)
	}
	if _, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID}); err != nil || len(refusals) > 0 {
		t.Fatalf("first Scaffold: err=%v refusals=%+v", err, refusals)
	}
	firstBytes, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.comp2ID})
	if err != nil {
		t.Fatalf("second Scaffold: unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("second Scaffold: unexpected refusals: %+v", refusals)
	}
	if report == nil {
		t.Fatal("second Scaffold: want a report, got nil")
	}
	if len(report.Written) != 0 {
		t.Errorf("second Scaffold: want nothing written, got %v", report.Written)
	}

	secondBytes, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md after second scaffold: %v", err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("arch_comp2.md changed on the idempotent re-run:\nfirst:  %s\nsecond: %s", firstBytes, secondBytes)
	}
}

// customProfileFixture is L4's second Setup fixture: a project whose
// profile declares two content-bearing types sharing no name with the
// default profile — endpoint (content prefix "ep_", leaf_sections Contract
// and Errors) and resource (no leaf_sections) — with one endpoint and one
// resource declared, neither leaf on disk, the resource's own "supports"
// field naming the endpoint so scaffolding the resource has exactly one
// owed placeholder to assert against.
type customProfileFixture struct {
	dir        string
	endpointID string
	resourceID string
}

func buildCustomProfileFixture(t *testing.T) customProfileFixture {
	t.Helper()
	dir := t.TempDir()

	profileDoc := `{
		"profile_version": 2,
		"node_types": [
			{"name": "endpoint", "plural_key": "endpoints", "scope": "module", "requires_content": true, "content_prefix": "ep_", "leaf_sections": ["Contract", "Errors"]},
			{"name": "resource", "plural_key": "resources", "scope": "module", "requires_content": true, "leaf_sections": [],
				"fields": [{"name": "supports", "kind": "reference", "targets": ["endpoint"], "cardinality": "many"}]}
		]
	}`
	writeFile(t, filepath.Join(dir, "profile.json"), profileDoc)

	proj := `{"name": "custom-project", "modules": [{"id": "000000000001", "name": "core", "path": "core"}]}`
	writeFile(t, filepath.Join(dir, "project.json"), proj)

	coreDir := filepath.Join(dir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}

	endpointID := schema.IdentityHash("core", "endpoint", "Get widget")
	resourceID := schema.IdentityHash("core", "resource", "Widget resource")
	mod := `{
		"name": "core",
		"endpoints": [{"id": "` + endpointID + `", "name": "Get widget", "content": "ep_get_widget.md"}],
		"resources": [{"id": "` + resourceID + `", "name": "Widget resource", "content": "res_widget_resource.md", "supports": ["` + endpointID + `"]}]
	}`
	writeFile(t, filepath.Join(coreDir, "module.json"), mod)

	return customProfileFixture{dir: dir, endpointID: endpointID, resourceID: resourceID}
}

// TestREQ_1631cb19fac3_L4_HeadingsComeFromProfileNotCommand is L4: the
// scaffolder writes whatever leaf_sections the resolved profile declares
// for a type it has never heard of, and profile show prints the same
// headings in the same order; a type declaring no leaf_sections gets no
// "##" heading at all, but still gets its owed placeholder line.
func TestREQ_1631cb19fac3_L4_HeadingsComeFromProfileNotCommand(t *testing.T) {
	f := buildCustomProfileFixture(t)

	profile, err := schema.ResolveProfile(f.dir)
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	var endpointSections []string
	for _, nt := range profile.NodeTypes {
		if nt.Name == "endpoint" {
			endpointSections = nt.LeafSections
		}
	}
	if want := []string{"Contract", "Errors"}; strings.Join(endpointSections, ",") != strings.Join(want, ",") {
		t.Fatalf("resolved profile's endpoint leaf_sections = %v, want %v", endpointSections, want)
	}

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.endpointID})
	if err != nil {
		t.Fatalf("Scaffold(endpoint): unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Scaffold(endpoint): unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 1 {
		t.Fatalf("Scaffold(endpoint): want a report writing exactly one file, got %+v", report)
	}

	epPath := filepath.Join(f.dir, "core", "ep_get_widget.md")
	epData, err := os.ReadFile(epPath)
	if err != nil {
		t.Fatalf("read %s: %v", epPath, err)
	}
	epContent := string(epData)
	if !strings.HasPrefix(epContent, "# Get widget\n") {
		t.Fatalf("endpoint leaf does not open with '# Get widget': %s", epContent)
	}
	assertInOrder(t, epContent, "## Contract", "## Errors")
	if strings.Count(epContent, "##") != 2 {
		t.Fatalf("endpoint leaf wants exactly the two declared headings and nothing else: %s", epContent)
	}
	if strings.Contains(epContent, "[[") {
		t.Fatalf("endpoint leaf wants no placeholder links (nothing non-content-bearing targets it): %s", epContent)
	}

	report, refusals, err = Scaffold(f.dir, ScaffoldInput{ID: f.resourceID})
	if err != nil {
		t.Fatalf("Scaffold(resource): unexpected error: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("Scaffold(resource): unexpected refusals: %+v", refusals)
	}
	if report == nil || len(report.Written) != 1 {
		t.Fatalf("Scaffold(resource): want a report writing exactly one file, got %+v", report)
	}

	resPath := filepath.Join(f.dir, "core", "res_widget_resource.md")
	resData, err := os.ReadFile(resPath)
	if err != nil {
		t.Fatalf("read %s: %v", resPath, err)
	}
	resContent := string(resData)
	if !strings.HasPrefix(resContent, "# Widget resource\n") {
		t.Fatalf("resource leaf does not open with '# Widget resource': %s", resContent)
	}
	if strings.Contains(resContent, "##") {
		t.Fatalf("resource leaf declares no leaf_sections and wants no '##' heading: %s", resContent)
	}
	wantLink := "[[" + f.endpointID + "|Get widget]]"
	if !strings.Contains(resContent, wantLink) {
		t.Fatalf("resource leaf wants its owed placeholder link %s: %s", wantLink, resContent)
	}
}

// TestREQ_1631cb19fac3_L7_NotContentBearingRefused is L7: a node of a type
// the profile marks as not content-bearing (the default profile's api)
// has no leaf to scaffold; the fix lists the content-bearing types.
func TestREQ_1631cb19fac3_L7_NotContentBearingRefused(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: f.apiID})
	if err != nil {
		t.Fatalf("Scaffold(api): unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Scaffold(api): want no report, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Scaffold(api): want exactly one refusal, got %+v", refusals)
	}
	if !strings.Contains(refusals[0].Message, "api") {
		t.Errorf("refusal does not name the type api: %+v", refusals[0])
	}
	if !strings.Contains(refusals[0].Fix, "component") {
		t.Errorf("refusal fix does not list the content-bearing types: %+v", refusals[0])
	}

	entries, err := os.ReadDir(filepath.Join(f.dir, "alpha"))
	if err != nil {
		t.Fatalf("read alpha dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "arch_demo") || strings.HasPrefix(e.Name(), "api_") {
			t.Errorf("no file should have been written for a non-content-bearing node, found %s", e.Name())
		}
	}
}

// TestScaffold_NodeNotFound covers arch_leaf_scaffolder.md's "a node that
// does not exist is refused with the array that was searched" — not one of
// test_leaf_and_profile.md's lettered scenarios, but the same refusal
// contract exercised directly.
func TestScaffold_NodeNotFound(t *testing.T) {
	f := buildRenameFixture(t)

	report, refusals, err := Scaffold(f.dir, ScaffoldInput{ID: "000000000000"})
	if err != nil {
		t.Fatalf("Scaffold: unexpected error: %v", err)
	}
	if report != nil {
		t.Fatalf("Scaffold: want no report, got %+v", report)
	}
	if len(refusals) != 1 {
		t.Fatalf("Scaffold: want exactly one refusal, got %+v", refusals)
	}
	if !strings.Contains(refusals[0].Message, "000000000000") {
		t.Errorf("refusal does not name the searched id: %+v", refusals[0])
	}
	if !strings.Contains(refusals[0].Fix, "searched") {
		t.Errorf("refusal fix does not name the arrays searched: %+v", refusals[0])
	}
}
