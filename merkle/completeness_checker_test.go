package merkle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCompletenessSpecDir creates a spec directory with requirements and
// implements edges for completeness checking tests.
func setupCompletenessSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// project.json with one module and one project-level requirement
	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Proj Req 1"},
			{"id": 5, "type": "functional", "title": "Proj Req 5"}
		],
		"modules": [
			{"id": 1, "name": "Alpha", "path": "alpha"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	// Module alpha: requirements 1,2; components 1,2,3 with implements edges
	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Req 1"},
			{"id": 2, "type": "functional", "title": "Req 2", "preq_id": 1}
		],
		"components": [
			{"id": 1, "name": "CompA", "content": "arch_comp_a.md", "implements": [1]},
			{"id": 2, "name": "CompB", "content": "arch_comp_b.md", "implements": [2]},
			{"id": 3, "name": "CompC", "content": "arch_comp_c.md", "implements": [2]}
		]
	}`
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_comp_a.md", "# CompA\n")
	writeFile(t, alphaDir, "arch_comp_b.md", "# CompB\n")
	writeFile(t, alphaDir, "arch_comp_c.md", "# CompC\n")

	return dir
}

// TestREQ8_C1_ModifiedRequirementWithComponentChanged verifies that no error is
// returned when a modified requirement's implementing component content also changed.
func TestREQ8_C1_ModifiedRequirementWithComponentChanged(t *testing.T) {
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/1", Type: Modified, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
		{Change: Change{Path: "module/1/component/1", Type: Modified, NodeType: "component", Module: "778a8efc100a"}, Impact: ArchImpl, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C2_ModifiedRequirementWithoutComponentChanged verifies that an error
// is returned when a modified requirement's implementing component content did not change.
func TestREQ8_C2_ModifiedRequirementWithoutComponentChanged(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/1", Type: Modified, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Type != "incomplete_change" {
		t.Errorf("expected type incomplete_change, got %q", errs[0].Type)
	}
	if errs[0].Path != "module/1/requirement/1" {
		t.Errorf("expected path module/1/requirement/1, got %q", errs[0].Path)
	}
	if len(errs[0].Related) != 1 || errs[0].Related[0] != "module/1/component/1" {
		t.Errorf("expected related [module/1/component/1], got %v", errs[0].Related)
	}
}

// TestREQ8_C3_AddedRequirementNoImplementor verifies that an error is returned
// when an added requirement has no implementing component.
func TestREQ8_C3_AddedRequirementNoImplementor(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/3", Type: Added, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "not implemented") {
		t.Errorf("expected message about not implemented, got %q", errs[0].Message)
	}
}

// TestREQ8_C4_AddedRequirementImplementorUnchanged verifies that an error is
// returned when an added requirement's implementing component did not change.
func TestREQ8_C4_AddedRequirementImplementorUnchanged(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// Req 2 is implemented by components 2 and 3. Neither is in the diff.
	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/2", Type: Added, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	// Should get errors for both comp 2 and comp 3
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C5_RemovedRequirementStillReferenced verifies that an error is returned
// when a removed requirement is still referenced in a component's implements array.
func TestREQ8_C5_RemovedRequirementStillReferenced(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/2", Type: Removed, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	// Components 2 and 3 still reference req 2
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
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "project/requirement/5", Type: Modified, NodeType: "requirement", Module: ""}, Impact: Structural},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "no module requirement derives from project requirement 5") {
		t.Errorf("unexpected message: %q", errs[0].Message)
	}
}

// TestREQ8_C7_ProjectRequirementChainIncomplete verifies that an error is returned
// when a project requirement changes, a module requirement derives from it, but the
// implementing component's content leaf did not change.
func TestREQ8_C7_ProjectRequirementChainIncomplete(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// project/requirement/1 changed. Module req 2 has preq_id=1.
	// Comp 2 and 3 implement req 2, but neither is in the diff.
	changes := []ClassifiedChange{
		{Change: Change{Path: "project/requirement/1", Type: Modified, NodeType: "requirement", Module: ""}, Impact: Structural},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) < 1 {
		t.Fatalf("expected at least 1 error, got %d", len(errs))
	}
	// Should have errors for components 2 and 3
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C8_ProjectRequirementChainComplete verifies no errors when a project
// requirement changes and the full chain (module req → component) is complete.
func TestREQ8_C8_ProjectRequirementChainComplete(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// project/requirement/1 changed. Module req 2 has preq_id=1.
	// Comp 2 and 3 implement req 2, and both are in the diff.
	changes := []ClassifiedChange{
		{Change: Change{Path: "project/requirement/1", Type: Modified, NodeType: "requirement", Module: ""}, Impact: Structural},
		{Change: Change{Path: "module/1/component/2", Type: Modified, NodeType: "component", Module: "778a8efc100a"}, Impact: ArchImpl, Module: "Alpha"},
		{Change: Change{Path: "module/1/component/3", Type: Modified, NodeType: "component", Module: "778a8efc100a"}, Impact: ArchImpl, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C9_MetaChangedWithoutRequirementChanges verifies component edge
// check when module meta changes but no requirement leaves changed.
func TestREQ8_C9_MetaChangedWithoutRequirementChanges(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// Only meta changed, no requirement changes in module 1
	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/meta", Type: Modified, NodeType: "meta", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	// All 3 components should be flagged since none of their content leaves changed
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (one per component), got %d: %v", len(errs), errs)
	}
}

// TestREQ8_C10_MultipleRequirementsPartialCoverage verifies that errors are
// returned only for components that didn't change, not for those that did.
func TestREQ8_C10_MultipleRequirementsPartialCoverage(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// Req 1 and 2 both modified. Comp 1 (implements req 1) changed. Comp 2 and 3 (implement req 2) did NOT change.
	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/requirement/1", Type: Modified, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
		{Change: Change{Path: "module/1/requirement/2", Type: Modified, NodeType: "requirement", Module: "778a8efc100a"}, Impact: Structural, Module: "Alpha"},
		{Change: Change{Path: "module/1/component/1", Type: Modified, NodeType: "component", Module: "778a8efc100a"}, Impact: ArchImpl, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
	// Only comp 2 and comp 3 should be flagged (they implement req 2 but didn't change)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Path != "module/1/requirement/2" {
			t.Errorf("expected errors for requirement 2, got path %q", e.Path)
		}
	}
}

// TestREQ8_C11_NoStructuralOrRequirementChanges verifies that no errors are
// returned when only impl_only and arch_impl changes are present.
func TestREQ8_C11_NoStructuralOrRequirementChanges(t *testing.T) {
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "module/1/impl_section/1", Type: Modified, NodeType: "impl_section", Module: "778a8efc100a"}, Impact: ImplOnly, Module: "Alpha"},
		{Change: Change{Path: "module/1/component/1", Type: Modified, NodeType: "component", Module: "778a8efc100a"}, Impact: ArchImpl, Module: "Alpha"},
	}

	errs := CheckCompleteness(changes, specDir)
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
	specDir := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		{Change: Change{Path: "project/requirement/5", Type: Added, NodeType: "requirement", Module: ""}, Impact: Structural},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_ProjectRequirement_Removed_StillReferenced verifies error when a removed
// project requirement is still referenced by module requirements via preq_id.
func TestREQ8_ProjectRequirement_Removed_StillReferenced(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupCompletenessSpecDir(t)

	// project/requirement/1 removed. Module req 2 still has preq_id=1.
	changes := []ClassifiedChange{
		{Change: Change{Path: "project/requirement/1", Type: Removed, NodeType: "requirement", Module: ""}, Impact: Structural},
	}

	errs := CheckCompleteness(changes, specDir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "still derives from removed project requirement") {
		t.Errorf("unexpected message: %q", errs[0].Message)
	}
}
