package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-14: Requirement coverage validation — every project requirement must be
// derived into at least one module requirement, and every module requirement
// must be implemented by at least one component.

func TestREQ14_AllCovered(t *testing.T) {
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_all_covered"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_UncoveredProjectRequirement(t *testing.T) {
	// Project req 2 "Feature B" has no module requirement with preq_id=2.
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_project_req"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "requirement_coverage" {
		t.Fatalf("expected check=requirement_coverage, got %q", e.Check)
	}
	if e.Severity != "error" {
		t.Fatalf("expected severity=error, got %q", e.Severity)
	}
	if !strings.Contains(e.Message, "Feature B") {
		t.Fatalf("expected message to contain requirement title, got: %s", e.Message)
	}
	if !strings.Contains(e.Message, "not derived") {
		t.Fatalf("expected 'not derived' in message, got: %s", e.Message)
	}
	if e.Path != "project.json" {
		t.Fatalf("expected path=project.json, got %q", e.Path)
	}
}

func TestREQ14_UncoveredModuleRequirement(t *testing.T) {
	// Module req 2 "Mod Feat B" has no component with implements containing 2.
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_module_req"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if !strings.Contains(e.Message, "Mod Feat B") {
		t.Fatalf("expected message to contain requirement title, got: %s", e.Message)
	}
	if !strings.Contains(e.Message, "not implemented") {
		t.Fatalf("expected 'not implemented' in message, got: %s", e.Message)
	}
	if !strings.Contains(e.Path, "alpha/module.json") {
		t.Fatalf("expected path containing alpha/module.json, got %q", e.Path)
	}
}

func TestREQ14_BothUncovered(t *testing.T) {
	// Project req 2 uncovered + module req 2 uncovered = 2 errors.
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_both_uncovered"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	var codes []string
	for _, e := range errs {
		codes = append(codes, e.Message)
	}
	joined := strings.Join(codes, " | ")
	if !strings.Contains(joined, "not derived") {
		t.Fatalf("expected one 'not derived' error, got: %s", joined)
	}
	if !strings.Contains(joined, "not implemented") {
		t.Fatalf("expected one 'not implemented' error, got: %s", joined)
	}
}

func TestREQ14_MultiModuleCoverage(t *testing.T) {
	// Two modules together cover both project requirements.
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_multi_module"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_NoRequirements(t *testing.T) {
	// No project requirements → nothing to check → zero errors.
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_no_requirements"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_ErrorMessageContainsReqID(t *testing.T) {
	errs, _ := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_project_req"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "2") {
		t.Fatalf("expected requirement ID in message, got: %s", errs[0].Message)
	}
}

func TestREQ14_SelfValidateRequirementCoverage(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs, _ := CheckRequirementCoverage(specDir)
	for _, e := range errs {
		t.Logf("uncovered requirement: %s — %s", e.Path, e.Message)
	}
}

// RC1: an underived project requirement with no derivation field is still an
// error, byte-for-byte the pre-existing message, and produces zero notes.
func TestREQ14_RC1_UnderivedWithoutFieldIsError(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_project_req"))
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `project requirement 000000000002 "Feature B" is not derived into any module requirement`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "project.json" {
		t.Fatalf("expected path=project.json, got %q", errs[0].Path)
	}
}

// RC2: a requirement declaring derivation "pending" that nothing derives
// produces a disclosure note instead of an error.
func TestREQ14_RC2_PendingUnderivedProducesNote(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_pending_note"))
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d: %v", len(notes), notes)
	}
	n := notes[0]
	if n.Type != "pending_derivation" {
		t.Fatalf("expected type=pending_derivation, got %q", n.Type)
	}
	want := `project requirement 000000000002 "Feature B" declares derivation pending and is not derived into any module requirement`
	if n.Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", n.Message, want)
	}
	if len(n.Related) != 1 || n.Related[0] != "000000000002" {
		t.Fatalf("expected related=[000000000002], got %v", n.Related)
	}
}

// RC3: a requirement declaring derivation "pending" that IS derived by a
// module requirement produces neither an error nor a note — the stale field
// is inert once the requirement is covered.
func TestREQ14_RC3_DerivedPendingProducesNeither(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_pending_derived"))
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
}

// RC4: the module-requirement-to-component link admits no pending state —
// an uncovered module requirement is still reported even when every project
// requirement declares derivation "pending".
func TestREQ14_RC4_ModuleLevelLinkAdmitsNoPending(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_pending_module_uncovered"))
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `alpha requirement 000000000001 "Mod Feat A" is not implemented by any component`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "alpha/module.json" {
		t.Fatalf("expected path=alpha/module.json, got %q", errs[0].Path)
	}
}

// RC5: notes are deterministic and ordered — three underived pending
// requirements declared in a fixed order produce the same three notes in
// declaration order across repeated runs.
func TestREQ14_RC5_NotesDeterministicAndOrdered(t *testing.T) {
	wantIDs := []string{"000000000003", "000000000001", "000000000002"}
	for run := 0; run < 2; run++ {
		errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_pending_multiple"))
		if len(errs) != 0 {
			t.Fatalf("run %d: expected 0 errors, got %d: %v", run, len(errs), errs)
		}
		if len(notes) != len(wantIDs) {
			t.Fatalf("run %d: expected %d notes, got %d: %v", run, len(wantIDs), len(notes), notes)
		}
		for i, id := range wantIDs {
			if len(notes[i].Related) != 1 || notes[i].Related[0] != id {
				t.Fatalf("run %d: note %d expected related=[%s], got %v", run, i, id, notes[i].Related)
			}
		}
	}
}

// RC6: a profile renaming the module-level covering type (component ->
// endpoint) drives the scan off the covering type's plural_key and the
// chain's edge instead of the literal components/implements fields; data
// where every module requirement is implemented by an endpoint produces no
// false positive.
func TestREQ14_RC6_ProfileRenamedCoveringTypeFullyCovered(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_profile_renamed_covered"))
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
}

// RC7: under the same renamed profile, a module requirement no endpoint
// implements is still reported, with the renamed covering type's declared
// name interpolated into the same message shape RC4 uses for "component".
func TestREQ14_RC7_ProfileRenamedCoveringTypeUncovered(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_profile_renamed_uncovered"))
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `alpha requirement 000000000002 "Mod Feat B" is not implemented by any endpoint`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "alpha/module.json" {
		t.Fatalf("expected path=alpha/module.json, got %q", errs[0].Path)
	}
}

// RC9: a profile renaming the project-scoped covered type (requirement ->
// objective) still interpolates the renamed node's declared name into the
// message — the generic covered-side path must read the wire's "name" key,
// not the retired "title" key, exactly as RC7 already pins for a renamed
// covering type.
func TestREQ14_RC9_ProfileRenamedCoveredProjectType(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_profile_renamed_covered_project"))
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `project objective 000000000009 "Feature Z" is not derived into any module requirement`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "project.json" {
		t.Fatalf("expected path=project.json, got %q", errs[0].Path)
	}
}

// RC10: a profile renaming the module-scoped covered type (requirement ->
// capability) exercises the same generic covered-side path at module scope
// — its declared name must appear quoted, not empty.
func TestREQ14_RC10_ProfileRenamedCoveredModuleType(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_profile_renamed_covered_module"))
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `alpha capability 000000000002 "Mod Feat B" is not implemented by any endpoint`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "alpha/module.json" {
		t.Fatalf("expected path=alpha/module.json, got %q", errs[0].Path)
	}
}

// RC8: a profile declaring no coverage chains (and no completeness-trigger
// types) drops both checks entirely — a project requirement with no
// deriving module requirement and a module requirement with no implementer
// both produce nothing, since neither chain resolves.
func TestREQ14_RC8_ProfileDroppedChainsSkipsBothChecks(t *testing.T) {
	errs, notes := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_profile_dropped_chains"))
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d: %v", len(notes), notes)
	}
}
