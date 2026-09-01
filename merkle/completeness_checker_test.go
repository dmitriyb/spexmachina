package merkle

import (
	"encoding/json"
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
	specDir   string
	alphaHash string
	req1Hash  string
	req2Hash  string
	req99Hash string
	comp1Hash string
	comp2Hash string
	comp3Hash string
	// projReq1ID/projReq5ID are the project requirement ids stored in
	// project.json (and used as module preq_id values, matching real specs).
	// They are also the tree keys the diff engine emits as Change.Key for
	// project-level requirement leaves.
	projReq1ID string
	projReq5ID string
}

// setupCompletenessSpecDir creates a spec directory with requirements and
// implements edges keyed by identity hashes. All module-level IDs match the
// canonical IdentityHash derivation so that ClassifiedChange.Key values
// used in tests line up with what the spec files declare.
func setupCompletenessSpecDir(t *testing.T) completenessFixture {
	t.Helper()
	dir := t.TempDir()

	fx := completenessFixture{
		specDir:     dir,
		alphaHash:   schema.IdentityHash("module", "Alpha"),
		req1Hash:    schema.IdentityHash("alpha", "requirement", "Req 1"),
		req2Hash:    schema.IdentityHash("alpha", "requirement", "Req 2"),
		comp1Hash:   schema.IdentityHash("alpha", "component", "CompA"),
		comp2Hash:   schema.IdentityHash("alpha", "component", "CompB"),
		comp3Hash:   schema.IdentityHash("alpha", "component", "CompC"),
		projReq1ID:  schema.IdentityHash("project", "requirement", "Proj Req 1"),
		projReq5ID:  schema.IdentityHash("project", "requirement", "Proj Req 5"),
	}

	proj := fmt.Sprintf(`{
		"name": "test-project",
		"requirements": [
			{"id": %q, "type": "functional", "name": "Proj Req 1"},
			{"id": %q, "type": "functional", "name": "Proj Req 5"}
		],
		"modules": [
			{"id": %q, "name": "Alpha", "path": "alpha"}
		]
	}`, fx.projReq1ID, fx.projReq5ID, fx.alphaHash)
	writeFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	req99Hash := schema.IdentityHash("alpha", "requirement", "Req 99")
	fx.req99Hash = req99Hash
	alphaMod := fmt.Sprintf(`{
		"name": "alpha",
		"requirements": [
			{"id": %q, "type": "functional", "title": "Req 1", "preq_id": %q},
			{"id": %q, "type": "functional", "title": "Req 2", "preq_id": %q},
			{"id": %q, "type": "functional", "title": "Req 99", "preq_id": %q}
		],
		"components": [
			{"id": %q, "name": "CompA", "content": "arch_comp_a.md", "implements": [%q]},
			{"id": %q, "name": "CompB", "content": "arch_comp_b.md", "implements": [%q]},
			{"id": %q, "name": "CompC", "content": "arch_comp_c.md", "implements": [%q]}
		]
	}`,
		fx.req1Hash, fx.projReq1ID,
		fx.req2Hash, fx.projReq1ID,
		req99Hash, fx.projReq1ID,
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
		Change: Change{Key: path, Type: t, NodeType: "requirement", Module: module},
		Impact: Structural,
	}
}

func compChange(path, module string, t ChangeType) ClassifiedChange {
	return ClassifiedChange{
		Change: Change{Key: path, Type: t, NodeType: "component", Module: module},
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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
		reqChange(fx.projReq5ID, "", Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := "no module requirement derives from project requirement 'Proj Req 5'"
	if !strings.Contains(errs[0].Message, want) {
		t.Errorf("expected message containing %q, got %q", want, errs[0].Message)
	}
	if errs[0].Path != fx.projReq5ID {
		t.Errorf("expected path %s (identity hash), got %q", fx.projReq5ID, errs[0].Path)
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
		reqChange(fx.projReq1ID, "", Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (CompA, CompB, CompC), got %d: %v", len(errs), errs)
	}
	related := map[string]bool{}
	for _, e := range errs {
		if len(e.Related) == 1 {
			related[e.Related[0]] = true
		}
		if e.Path != fx.projReq1ID {
			t.Errorf("expected path %s (identity hash), got %q", fx.projReq1ID, e.Path)
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
		reqChange(fx.projReq1ID, "", Modified),
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
		compChange(fx.comp2Hash, fx.alphaHash, Modified),
		compChange(fx.comp3Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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
			Change: Change{Key: "meta/" + fx.alphaHash, Type: Modified, NodeType: "meta", Module: fx.alphaHash},
			Impact: Structural,
		},
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
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
			Key:      schema.IdentityHash("alpha", "test_section", "Test1"),
			Type:     Modified,
			NodeType: "test_section",
			Module:   fx.alphaHash,
		},
		Impact: ImplOnly,
	}

	changes := []ClassifiedChange{
		implOnly,
		compChange(fx.comp1Hash, fx.alphaHash, Modified),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ8_S15_CompletenessTriggerFollowsProfile covers S15: which node type
// triggers the requirement-leaf rules is read from the resolved profile's
// per-type CompletenessTrigger flag, not compiled in as the literal
// "requirement". A profile that turns the trigger off for module-scoped
// "requirement" and on for a project-declared "objective" type must let a
// requirement change pass unchecked while treating an objective change the
// same way C2 treats a modified requirement.
func TestREQ8_S15_CompletenessTriggerFollowsProfile(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	profile := schema.DefaultProfile()
	for i := range profile.NodeTypes {
		if profile.NodeTypes[i].Name == "requirement" && profile.NodeTypes[i].Scope == "module" {
			profile.NodeTypes[i].CompletenessTrigger = false
		}
	}
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name: "objective", PluralKey: "objectives", Scope: "module", CompletenessTrigger: true,
	})

	// A modified requirement no longer triggers anything under this profile,
	// even though CompA (which implements it) never changed.
	reqOnly := []ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified),
	}
	if errs := CheckCompleteness(reqOnly, fx.specDir, profile); errs != nil {
		t.Fatalf("expected no errors for requirement change under a profile without the trigger, got %v", errs)
	}

	// The same identity hash, reported as an "objective" change instead,
	// triggers the rule the profile now assigns to that type.
	objChange := []ClassifiedChange{
		{
			Change: Change{Key: fx.req1Hash, Type: Modified, NodeType: "objective", Module: fx.alphaHash},
			Impact: Structural,
		},
	}
	errs := CheckCompleteness(objChange, fx.specDir, profile)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for objective-type completeness trigger, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != fx.req1Hash {
		t.Errorf("expected path %s, got %q", fx.req1Hash, errs[0].Path)
	}
	if len(errs[0].Related) != 1 || errs[0].Related[0] != fx.comp1Hash {
		t.Errorf("expected related [%s], got %v", fx.comp1Hash, errs[0].Related)
	}
}

// TestREQ8_CheckCompleteness_NilChanges verifies that nil input returns nil.
func TestREQ8_CheckCompleteness_NilChanges(t *testing.T) {
	errs := CheckCompleteness(nil, "", schema.DefaultProfile())
	if errs != nil {
		t.Fatalf("expected nil for nil input, got %v", errs)
	}
}

// TestREQ8_CheckCompleteness_EmptyChanges verifies that empty input returns nil.
func TestREQ8_CheckCompleteness_EmptyChanges(t *testing.T) {
	errs := CheckCompleteness([]ClassifiedChange{}, "", schema.DefaultProfile())
	if errs != nil {
		t.Fatalf("expected nil for empty input, got %v", errs)
	}
}

// TestREQ8_ProjectRequirement_Added_NoDerivation verifies error when an added
// project requirement has no module requirement deriving from it.
func TestREQ8_ProjectRequirement_Added_NoDerivation(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	changes := []ClassifiedChange{
		reqChange(fx.projReq5ID, "", Added),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != fx.projReq5ID {
		t.Errorf("expected path %s (identity hash), got %q", fx.projReq5ID, errs[0].Path)
	}
}

// TestREQ8_ProjectRequirement_Removed_StillReferenced verifies error when a removed
// project requirement is still referenced by module requirements via preq_id.
func TestREQ8_ProjectRequirement_Removed_StillReferenced(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// projReq1 removed. Module reqs 1, 2, and 99 still have preq_id = projReq1ID.
	changes := []ClassifiedChange{
		reqChange(fx.projReq1ID, "", Removed),
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (one per derived module requirement), got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "still derives from removed project requirement") {
			t.Errorf("unexpected message: %q", e.Message)
		}
		if e.Path != fx.projReq1ID {
			t.Errorf("expected path %s (identity hash), got %q", fx.projReq1ID, e.Path)
		}
	}
	related := map[string]bool{}
	for _, e := range errs {
		if len(e.Related) == 1 {
			related[e.Related[0]] = true
		}
	}
	for _, want := range []string{fx.req1Hash, fx.req2Hash, fx.req99Hash} {
		if !related[want] {
			t.Errorf("expected related set to contain %s, got %v", want, related)
		}
	}
}

// TestREQ8_ProjectRequirement_Added_RawPreqIDDerivation verifies that derivation
// resolution recognizes module requirements whose preq_id stores the project
// requirement id (as the real repo's module.json files do). The diff engine
// emits that same id as Change.Key — no re-derived tree key exists anymore.
// Regression guard for spexmachina-elv, updated for spexmachina-f7re.
func TestREQ8_ProjectRequirement_Added_RawPreqIDDerivation(t *testing.T) {
	dir := t.TempDir()

	rawProjReqID := "0a0f49f9be9b"
	modReqID := schema.IdentityHash("alpha", "requirement", "Derived")
	compID := schema.IdentityHash("alpha", "component", "CompA")

	proj := fmt.Sprintf(`{
		"name": "test-project",
		"requirements": [
			{"id": %q, "type": "functional", "name": "Source"}
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

	// Diff carries the requirement's id as c.Key for the added project
	// requirement, and the module requirement's preq_id stores that same id.
	// The checker must resolve that the module req derives from the project
	// req. With CompA also modified, we expect zero errors.
	changes := []ClassifiedChange{
		reqChange(rawProjReqID, "", Added),
		compChange(compID, schema.IdentityHash("module", "Alpha"), Modified),
	}

	errs := CheckCompleteness(changes, dir, schema.DefaultProfile())
	if len(errs) != 0 {
		t.Fatalf("expected no errors (module req derives from project req via raw preq_id), got %d: %v", len(errs), errs)
	}
}

// TestREQ8_ErrorFields_AreIdentityHashesInSpec asserts that every Path and
// Related value produced by CheckCompleteness is an identity hash that
// actually appears as an id in project.json or the relevant module.json.
// Regression guard for spexmachina-d7w: path must never be the TreeBuilder
// double-hashed key, which does not appear in any spec file.
func TestREQ8_ErrorFields_AreIdentityHashesInSpec(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// Mix module-req, added-orphan, removed, and project-req changes so every
	// error branch contributes at least one DiffError.
	changes := []ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified), // C2 branch
		reqChange(fx.req99Hash, fx.alphaHash, Added),   // C3 branch (req in module.json, no implementor)
		reqChange(fx.req2Hash, fx.alphaHash, Removed),  // C5 branch
		reqChange(fx.projReq1ID, "", Modified),        // project modified branch
		reqChange(fx.projReq5ID, "", Added),           // project no-derivation branch
		{ // meta-only branch
			Change: Change{Key: "meta/" + fx.alphaHash, Type: Modified, NodeType: "meta", Module: fx.alphaHash},
			Impact: Structural,
		},
	}

	errs := CheckCompleteness(changes, fx.specDir, schema.DefaultProfile())
	if len(errs) == 0 {
		t.Fatalf("expected errors across every branch, got 0")
	}

	valid := collectIdentityHashes(t, fx.specDir)

	for _, e := range errs {
		// Meta-envelope leaves use a synthetic "meta/<hash>" path; accept
		// that form but verify the hash portion is a known module.
		if hash, ok := stripMetaPrefix(e.Path); ok {
			if !valid[hash] {
				t.Errorf("meta path %q references unknown module hash", e.Path)
			}
		} else if !valid[e.Path] {
			t.Errorf("Path %q is not an identity hash present in project.json or module.json (message: %q)", e.Path, e.Message)
		}
		for _, r := range e.Related {
			if !valid[r] {
				t.Errorf("Related %q is not an identity hash present in project.json or module.json (message: %q)", r, e.Message)
			}
		}
	}
}

// TestREQ8_FindingsAreTypedErrors locks the output-semantics clarification from
// the 2026-04-27-pipeline-cleanup-and-refresh-mode proposal: CompletenessChecker
// findings are errors, not warnings. Per arch_completeness_checker.md the return
// value is []DiffError, and per arch_diff_command.md each entry's `type` is a
// specific error kind (e.g. incomplete_change) — never empty and never an
// advisory "warning" label. This guards against any regression that would let a
// finding leak out without a typed error kind or message.
func TestREQ8_FindingsAreTypedErrors(t *testing.T) {
	fx := setupCompletenessSpecDir(t)

	// assertTyped verifies the contract on a set of errors: every finding is a
	// typed incomplete_change error with a non-empty message, never a blank or
	// advisory "warning" label.
	assertTyped := func(t *testing.T, branch string, errs []DiffError) {
		t.Helper()
		if len(errs) == 0 {
			t.Fatalf("%s: expected at least one DiffError, got 0", branch)
		}
		for i, e := range errs {
			if e.Type == "" {
				t.Errorf("%s: errs[%d]: error kind (Type) is empty; findings must carry a specific error kind, not an advisory blank/warning (message: %q)", branch, i, e.Message)
			}
			if e.Type != "incomplete_change" {
				t.Errorf("%s: errs[%d]: Type = %q, want the documented error kind %q", branch, i, e.Type, "incomplete_change")
			}
			if e.Message == "" {
				t.Errorf("%s: errs[%d]: Message is empty; every error must explain the incomplete edit", branch, i)
			}
		}
	}

	// Drive the requirement-leaf branches (modified, added-orphan, removed,
	// project-no-derivation). The return type is []DiffError by construction;
	// binding it here makes the "findings are DiffError, not a warning type"
	// contract explicit at compile time.
	var reqErrs []DiffError = CheckCompleteness([]ClassifiedChange{
		reqChange(fx.req1Hash, fx.alphaHash, Modified), // modified req, component unchanged
		reqChange(fx.req99Hash, fx.alphaHash, Added),   // added req, no implementor
		reqChange(fx.req2Hash, fx.alphaHash, Removed),  // removed req, still referenced
		reqChange(fx.projReq5ID, "", Modified),        // project req, no derivation
	}, fx.specDir, schema.DefaultProfile())
	assertTyped(t, "requirement branches", reqErrs)

	// Drive the meta/component-edge branch in isolation. Per
	// impl_completeness_algorithm.md step 9 it only fires when no requirement
	// leaf in the same module changed, so it must run on its own module-meta
	// change rather than alongside the alpha requirement changes above (which
	// would suppress it). This is the only test that locks the typed-error
	// invariant for this branch.
	var metaErrs []DiffError = CheckCompleteness([]ClassifiedChange{
		{
			Change: Change{Key: "meta/" + fx.alphaHash, Type: Modified, NodeType: "meta", Module: fx.alphaHash},
			Impact: Structural,
		},
	}, fx.specDir, schema.DefaultProfile())
	assertTyped(t, "meta/component-edge branch", metaErrs)
}

// collectIdentityHashes returns the set of every raw identity hash stored in
// project.json and every module.json — the values any Path/Related field is
// allowed to take.
func collectIdentityHashes(t *testing.T, specDir string) map[string]bool {
	t.Helper()
	set := map[string]bool{}

	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	must(t, err)
	var proj schema.Project
	must(t, json.Unmarshal(projData, &proj))
	for _, req := range proj.Requirements {
		set[req.ID] = true
	}
	for _, mod := range proj.Modules {
		set[schema.IdentityHash("module", mod.Name)] = true
		data, err := os.ReadFile(filepath.Join(specDir, mod.Path, "module.json"))
		must(t, err)
		var ms schema.ModuleSpec
		must(t, json.Unmarshal(data, &ms))
		for _, r := range ms.Requirements {
			set[r.ID] = true
		}
		for _, c := range ms.Components {
			set[c.ID] = true
		}
		for _, f := range ms.DataFlows {
			set[f.ID] = true
		}
		for _, ts := range ms.TestSections {
			set[ts.ID] = true
		}
	}
	return set
}

func stripMetaPrefix(path string) (string, bool) {
	const prefix = "meta/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):], true
	}
	return "", false
}
