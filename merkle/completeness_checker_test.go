package merkle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// completenessFixture bundles the identity hashes produced by
// setupCompletenessSpecDir so tests can build ClassifiedChange values with
// the same hashes that the fixture's module.json declares.
type completenessFixture struct {
	specDir    string
	alphaHash  string
	req1Hash   string
	req2Hash   string
	comp1Hash  string
	comp2Hash  string
	comp3Hash  string
	projReq1   string
	projReq5   string
}

// setupCompletenessSpecDir creates a spec directory with requirements and
// implements edges keyed by identity hashes. All module-level IDs match the
// canonical IdentityHash derivation so that ClassifiedChange.Path values
// used in tests line up with what the spec files declare.
func setupCompletenessSpecDir(t *testing.T) completenessFixture {
	t.Helper()
	dir := t.TempDir()

	fx := completenessFixture{
		specDir:   dir,
		alphaHash: schema.IdentityHash("module", "Alpha"),
		req1Hash:  schema.IdentityHash("alpha", "requirement", "Req 1"),
		req2Hash:  schema.IdentityHash("alpha", "requirement", "Req 2"),
		comp1Hash: schema.IdentityHash("alpha", "component", "CompA"),
		comp2Hash: schema.IdentityHash("alpha", "component", "CompB"),
		comp3Hash: schema.IdentityHash("alpha", "component", "CompC"),
		projReq1:  schema.IdentityHash("project", "requirement", "000000000001"),
		projReq5:  schema.IdentityHash("project", "requirement", "5"),
	}

	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": "000000000001", "type": "functional", "title": "Proj Req 1"},
			{"id": "000000000005", "type": "functional", "title": "Proj Req 5"}
		],
		"modules": [
			{"id": "000000000001", "name": "Alpha", "path": "alpha"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := fmt.Sprintf(`{
		"name": "alpha",
		"requirements": [
			{"id": %q, "type": "functional", "title": "Req 1", "preq_id": %q},
			{"id": %q, "type": "functional", "title": "Req 2", "preq_id": %q}
		],
		"components": [
			{"id": %q, "name": "CompA", "content": "arch_comp_a.md", "implements": [%q]},
			{"id": %q, "name": "CompB", "content": "arch_comp_b.md", "implements": [%q]},
			{"id": %q, "name": "CompC", "content": "arch_comp_c.md", "implements": [%q]}
		]
	}`,
		fx.req1Hash, fx.projReq1,
		fx.req2Hash, fx.projReq1,
		fx.comp1Hash, fx.req1Hash,
		fx.comp2Hash, fx.req2Hash,
		fx.comp3Hash, fx.req2Hash,
	)
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_comp_a.md", "# CompA\n")
	writeFile(t, alphaDir, "arch_comp_b.md", "# CompB\n")
	writeFile(t, alphaDir, "arch_comp_c.md", "# CompC\n")

	return fx
}

// reqChange and compChange build ClassifiedChange values with the correct
// NodeType and Impact fields so tests stay focused on the assertion.
func reqChange(path string, module string, t ChangeType) ClassifiedChange {
	return ClassifiedChange{
		Change: Change{Path: path, Type: t, NodeType: "requirement", Module: module},
		Impact: Structural,
	}
}

func compChange(path, module string, t ChangeType) ClassifiedChange {
	return ClassifiedChange{
		Change: Change{Path: path, Type: t, NodeType: "component", Module: module},
		Impact: ArchImpl,
	}
}

// TestREQ8_C1_ModifiedRequirementWithComponentChanged verifies that no error is
// returned when a modified requirement's implementing component content also changed.
func TestREQ8_C1_ModifiedRequirementWithComponentChanged(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified),
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C2_ModifiedRequirementWithoutComponentChanged verifies that an error
// is returned when a modified requirement's implementing component content did not change.
func TestREQ8_C2_ModifiedRequirementWithoutComponentChanged(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Type != "incomplete_change" {
		t.Errorf("expected type incomplete_change, got %q", errs[0].Type)
	}
	if errs[0].Path != fx.req1Hash {
		t.Errorf("expected path %s, got %q", fx.req1Hash, errs[0].Path)
	}
	if len(errs[0].Related) != 1 || errs[0].Related[0] != fx.comp1Hash {
		t.Errorf("expected related [%s], got %v", fx.comp1Hash, errs[0].Related)
	}
}

// TestREQ8_C3_AddedRequirementNoImplementor verifies that an error is returned
// when an added requirement has no implementing component.
func TestREQ8_C3_AddedRequirementNoImplementor(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// A hash that no component declares in its implements array.
	orphanHash := schema.IdentityHash("alpha", "requirement", "Req 99")

	changes := []ClassifiedChange{
		reqChange(orphanHash, fx.alphaHash, Added),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "not implemented") {
		t.Errorf("expected message about not implemented, got %q", errs[0].Message)
	}
	if errs[0].Path != orphanHash {
		t.Errorf("expected path %s, got %q", orphanHash, errs[0].Path)
	}
}

// TestREQ8_C4_AddedRequirementImplementorUnchanged verifies that an error is
// returned when an added requirement's implementing component did not change.
func TestREQ8_C4_AddedRequirementImplementorUnchanged(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// Req 2 is implemented by CompB and CompC. Neither is in the diff.
	changes := []ClassifiedChange{
		reqChange(fx.req2Hash, fx.alphaHash, Added),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	related := map[string]bool{}
	for _, e := range errs {
		if len(e.Related) == 1 {
			related[e.Related[0]] = true
		}
	}
	if !related[fx.comp2Hash] || !related[fx.comp3Hash] {
		t.Errorf("expected errors for %s and %s, got related set %v", fx.comp2Hash, fx.comp3Hash, related)
	}
}

// TestREQ8_C5_RemovedRequirementStillReferenced verifies that an error is returned
// when a removed requirement is still referenced in a component's implements array.
func TestREQ8_C5_RemovedRequirementStillReferenced(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.req2Hash, fx.alphaHash, Removed),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	// CompB and CompC still reference req2.
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "still implements removed requirement") {
			t.Errorf("expected message about still implementing removed requirement, got %q", e.Message)
		}
	}
}

// TestREQ8_C6_ProjectRequirementNoModuleDerivation verifies that an error is
// returned when a project requirement changes but no module requirement derives from it.
func TestREQ8_C6_ProjectRequirementNoModuleDerivation(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.projReq5, "", Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := fmt.Sprintf("no module requirement derives from project requirement %s", fx.projReq5)
	if !strings.Contains(errs[0].Message, want) {
		t.Errorf("expected message containing %q, got %q", want, errs[0].Message)
	}
}

// TestREQ8_C7_ProjectRequirementChainIncomplete verifies that an error is returned
// when a project requirement changes, a module requirement derives from it, but the
// implementing component's content leaf did not change.
func TestREQ8_C7_ProjectRequirementChainIncomplete(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// projReq1 modified. Module reqs 1 and 2 both derive from it.
	// Req 1 → CompA. Req 2 → CompB, CompC. None of the components are in the diff.
	changes := []ClassifiedChange{
		reqChange(fx.projReq1, "", Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (CompA, CompB, CompC), got %d: %v", len(errs), errs)
	}
	related := map[string]bool{}
	for _, e := range errs {
		if len(e.Related) == 1 {
			related[e.Related[0]] = true
		}
	}
	for _, want := range []string{fx.comp1Hash, fx.comp2Hash, fx.comp3Hash} {
		if !related[want] {
			t.Errorf("missing error for component %s; related set: %v", want, related)
		}
	}
}

// TestREQ8_C8_ProjectRequirementChainComplete verifies no errors when a project
// requirement changes and the full chain (module req → component) is complete.
func TestREQ8_C8_ProjectRequirementChainComplete(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// projReq1 changed. CompA, CompB, CompC all in the diff — all derived
	// module requirements (req1, req2) are fully covered.
	changes := []ClassifiedChange{
		reqChange(fx.projReq1, "", Modified),
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
		compChange(fx.comp2Hash, fx.alphaHash, Modified),
		compChange(fx.comp3Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C9_MetaChangedWithoutRequirementChanges verifies component edge
// check when module meta changes but no requirement leaves changed.
func TestREQ8_C9_MetaChangedWithoutRequirementChanges(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{
			Change: Change{Path: "meta/" + fx.alphaHash, Type: Modified, NodeType: "meta", Module: fx.alphaHash},
			Impact: Structural,
		},
	}

	errs := CheckCompleteness(changes, fx.specDir)
	// All 3 components should be flagged since none of their content leaves changed.
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (one per component), got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Path != "meta/"+fx.alphaHash {
			t.Errorf("expected path meta/%s, got %q", fx.alphaHash, e.Path)
		}
	}
}

// TestREQ8_C10_MultipleRequirementsPartialCoverage verifies that errors are
// returned only for components that didn't change, not for those that did.
func TestREQ8_C10_MultipleRequirementsPartialCoverage(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// Req 1 and 2 modified. CompA (implements req1) changed.
	// CompB and CompC (implement req2) did NOT change.
	changes := []ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified),
		reqChange(fx.req2Hash, fx.alphaHash, Modified),
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Path != fx.req2Hash {
			t.Errorf("expected errors for requirement %s, got path %q", fx.req2Hash, e.Path)
		}
	}
}

// TestREQ8_C11_NoStructuralOrRequirementChanges verifies that no errors are
// returned when only impl_only and arch_impl changes are present.
func TestREQ8_C11_NoStructuralOrRequirementChanges(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	implOnly := ClassifiedChange{
		Change: Change{
			Path:     schema.IdentityHash("alpha", "impl_section", "Impl1"),
			Type:     Modified,
			NodeType: "impl_section",
			Module:   fx.alphaHash,
		},
		Impact: ImplOnly,
	}

	changes := []ClassifiedChange{
		implOnly,
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_CheckCompleteness_NilChanges verifies that nil input returns nil.
func TestREQ8_CheckCompleteness_NilChanges(t *testing.T) {
	errs := CheckCompleteness(nil, "")
	if errs != nil {
		t.Fatalf("expected nil for nil input, got %v", errs)
	}
}

// TestREQ8_CheckCompleteness_EmptyChanges verifies that empty input returns nil.
func TestREQ8_CheckCompleteness_EmptyChanges(t *testing.T) {
	errs := CheckCompleteness([]ClassifiedChange{}, "")
	if errs != nil {
		t.Fatalf("expected nil for empty input, got %v", errs)
	}
}

// TestREQ8_ProjectRequirement_Added_NoDerivation verifies error when an added
// project requirement has no module requirement deriving from it.
func TestREQ8_ProjectRequirement_Added_NoDerivation(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.projReq5, "", Added),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_ProjectRequirement_Removed_StillReferenced verifies error when a removed
// project requirement is still referenced by module requirements via preq_id.
func TestREQ8_ProjectRequirement_Removed_StillReferenced(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// projReq1 removed. Module reqs 1 and 2 still have preq_id = projReq1.
	changes := []ClassifiedChange{
		reqChange(fx.projReq1, "", Removed),
	}

	errs := CheckCompleteness(changes, fx.specDir)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (one per derived module requirement), got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "still derives from removed project requirement") {
			t.Errorf("unexpected message: %q", e.Message)
		}
	}
	related := map[string]bool{}
	for _, e := range errs {
		if len(e.Related) == 1 {
			related[e.Related[0]] = true
		}
	}
	if !related[fx.req1Hash] || !related[fx.req2Hash] {
		t.Errorf("expected related set to contain %s and %s, got %v", fx.req1Hash, fx.req2Hash, related)
	}
}

// TestREQ8_ProjectRequirement_Added_RawPreqIDDerivation verifies that derivation
// resolution recognizes module requirements whose preq_id stores the raw project
// requirement ID (as the real repo's module.json files do) rather than the
// TreeBuilder's double-hashed tree key. Regression guard for spexmachina-elv.
func TestREQ8_ProjectRequirement_Added_RawPreqIDDerivation(t *testing.T) {
	dir := t.TempDir()

	rawProjReqID := "0a0f49f9be9b"
	projReqTreeKey := schema.IdentityHash("project", "requirement", rawProjReqID)
	modReqID := schema.IdentityHash("alpha", "requirement", "Derived")
	compID := schema.IdentityHash("alpha", "component", "CompA")

	proj := fmt.Sprintf(`{
		"name": "test-project",
		"requirements": [
			{"id": %q, "type": "functional", "title": "Source"}
		],
		"modules": [
			{"id": %q, "name": "Alpha", "path": "alpha"}
		]
	}`, rawProjReqID, schema.IdentityHash("module", "Alpha"))
	writeFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := fmt.Sprintf(`{
		"name": "alpha",
		"requirements": [
			{"id": %q, "type": "functional", "title": "Derived", "preq_id": %q}
		],
		"components": [
			{"id": %q, "name": "CompA", "content": "arch_comp_a.md", "implements": [%q]}
		]
	}`, modReqID, rawProjReqID, compID, modReqID)
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_comp_a.md", "# CompA\n")

	// Diff carries the TREE KEY (projReqTreeKey) as c.Path for the added project
	// requirement. The module requirement's preq_id stores the RAW ID. The
	// checker must resolve that the module req derives from the project req.
	// With CompA also modified, we expect zero errors.
	changes := []ClassifiedChange{
		reqChange(projReqTreeKey, "", Added),
		compChange(compID, schema.IdentityHash("module", "Alpha"), Modified),
	}

	errs := CheckCompleteness(changes, dir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors (module req derives from project req via raw preq_id), got %d: %v", len(errs), errs)
	}
}
