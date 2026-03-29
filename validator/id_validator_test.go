package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-5: ID uniqueness — no duplicate IDs within any array.
// REQ-6: Cross-reference integrity — all reference targets exist.
// REQ-13: Priority on project requirements.

// --- I1: All IDs unique and references valid ---

func TestREQ5_ValidIDsReturnsEmpty(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// --- I2: Duplicate requirement IDs within a module ---

func TestREQ5_DuplicateReqIDsInModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/requirements") &&
			strings.Contains(e.Message, "duplicate ID 1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate requirement ID error in module, got none")
	}
}

// --- I3: Duplicate component IDs within a module ---

func TestREQ5_DuplicateCompIDsInModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/components") &&
			strings.Contains(e.Message, "duplicate ID 1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate component ID error in module, got none")
	}
}

// --- I4: Duplicate impl_section IDs within a module ---

func TestREQ5_DuplicateImplSectionIDs(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup_impl"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/impl_sections") &&
			strings.Contains(e.Message, "duplicate ID 1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate impl_section ID error, got none")
	}
}

// --- I5: Duplicate module IDs in project.json ---

func TestREQ5_DuplicateModuleIDs(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup_module"))
	found := false
	for _, e := range errs {
		if e.Path == "project.json:/modules" &&
			strings.Contains(e.Message, "duplicate ID 1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate module ID error in project.json, got none")
	}
}

// --- I6: Component implements references non-existent requirement ---

func TestREQ6_DanglingImplements(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "implements references non-existent requirement 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling implements error, got none")
	}
}

// --- I7: Component uses references non-existent component ---

func TestREQ6_DanglingUses(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "components") &&
			strings.Contains(e.Message, "uses references non-existent component 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling component uses error, got none")
	}
}

// --- I8: impl_section describes references non-existent component ---

func TestREQ6_DanglingDescribes(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "impl_sections") &&
			strings.Contains(e.Message, "describes references non-existent component 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling impl_section describes error, got none")
	}
}

// --- I9: Requirement depends_on references non-existent requirement ---

func TestREQ6_DanglingDependsOn(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "depends_on references non-existent requirement 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling depends_on error, got none")
	}
}

// --- I10: Module requires_module references non-existent module ---

func TestREQ6_DanglingRequiresModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "requires_module references non-existent module 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling requires_module error, got none")
	}
}

// --- I11: Milestone groups references non-existent module ---

func TestREQ6_DanglingMilestoneGroups(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "groups references non-existent module 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling milestone groups error, got none")
	}
}

// --- I12: Requirement preq_id references non-existent project requirement ---

func TestREQ6_DanglingPreqID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "preq_id references non-existent project requirement 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling preq_id error, got none")
	}
}

// --- I13: Multiple dangling references in one module ---

func TestREQ6_MultipleDanglingRefsReported(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))

	// Should have at least 3 module-level errors: implements, uses, describes.
	moduleErrs := 0
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:") {
			moduleErrs++
		}
	}
	if moduleErrs < 3 {
		t.Fatalf("expected at least 3 module-level errors, got %d", moduleErrs)
	}
}

// --- I14: data_flow uses references non-existent component ---

func TestREQ6_DanglingDataFlowUses(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "data_flows") &&
			strings.Contains(e.Message, "uses references non-existent component 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling data_flow uses error, got none")
	}
}

// --- I15: Module requirement missing preq_id fails validation ---

func TestREQ6_MissingPreqID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_missing_preq"))
	if len(errs) == 0 {
		t.Fatal("expected error for missing preq_id, got none")
	}

	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "missing preq_id") {
			found = true
			if e.Check != "id" {
				t.Fatalf("expected check=id, got %q", e.Check)
			}
			if e.Severity != "error" {
				t.Fatalf("expected severity=error, got %q", e.Severity)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'missing preq_id' error, got: %v", errs)
	}
}

// --- I16: Project requirement missing priority field ---

func TestREQ13_MissingPriority(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_missing_priority"))
	if len(errs) == 0 {
		t.Fatal("expected error for missing priority, got none")
	}

	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "missing priority") {
			found = true
			if e.Check != "id" {
				t.Fatalf("expected check=id, got %q", e.Check)
			}
			if e.Severity != "error" {
				t.Fatalf("expected severity=error, got %q", e.Severity)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'missing priority' error, got: %v", errs)
	}
}

// --- I17: Project requirement with out-of-range priority ---

func TestREQ13_PriorityOutOfRange(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_priority_range"))
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 priority range errors (5 and -1), got %d: %v", len(errs), errs)
	}

	for _, e := range errs {
		if !strings.Contains(e.Message, "out of range") {
			t.Fatalf("expected 'out of range' in message, got: %s", e.Message)
		}
		if e.Check != "id" {
			t.Fatalf("expected check=id, got %q", e.Check)
		}
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

// --- E2: Same numeric ID reused across different array types ---

func TestREQ5_SameIDacrossTypesIsValid(t *testing.T) {
	// The id_valid fixture already uses ID 1 for requirements, components,
	// impl_sections, data_flows, and test_sections. This is valid because
	// IDs only need to be unique within their own array type.
	errs := CheckIDs(filepath.Join("testdata", "id_valid"))
	if len(errs) > 0 {
		t.Fatalf("same ID across different array types should be valid, got errors: %v", errs)
	}
}

// --- E5: test_section describes references non-existent component ---

func TestREQ6_DanglingTestSectionDescribes(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "test_sections") &&
			strings.Contains(e.Message, "describes references non-existent component 99") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling test_section describes error, got none")
	}
}

// --- Structural tests ---

func TestREQ5_DuplicatesBlockRefChecks(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	for _, e := range errs {
		if !strings.Contains(e.Message, "duplicate ID") {
			t.Fatalf("expected only duplicate errors when duplicates exist, got: %s", e.Message)
		}
	}
}

func TestREQ5_AllDuplicateErrorsTaggedID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	for _, e := range errs {
		if e.Check != "id" {
			t.Fatalf("expected check=id, got %q", e.Check)
		}
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

func TestREQ6_DanglingRefPathIncludesSource(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))

	hasProject := false
	hasModule := false
	for _, e := range errs {
		if strings.HasPrefix(e.Path, "project.json:") {
			hasProject = true
		}
		if strings.HasPrefix(e.Path, "core/module.json:") {
			hasModule = true
		}
	}
	if !hasProject {
		t.Fatal("expected at least one error with project.json path")
	}
	if !hasModule {
		t.Fatal("expected at least one error with module path")
	}
}

func TestREQ5_SelfValidateIDs(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckIDs(specDir)
	for _, e := range errs {
		t.Fatalf("unexpected error in own spec: %v", e)
	}
}
