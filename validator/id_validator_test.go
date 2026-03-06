package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-5: ID uniqueness — no duplicate IDs within any array.
// REQ-6: Cross-reference integrity — all reference targets exist.

func TestREQ5_ValidIDsReturnsEmpty(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ5_DuplicateIDsDetected(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	if len(errs) == 0 {
		t.Fatal("expected duplicate ID errors, got none")
	}

	for _, e := range errs {
		if !strings.Contains(e.Message, "duplicate ID") {
			t.Fatalf("expected duplicate ID error, got: %s", e.Message)
		}
	}

	// Should find duplicates in project requirements, module requirements, and module components.
	paths := make(map[string]bool)
	for _, e := range errs {
		paths[e.Path] = true
	}

	wantPaths := []string{
		"project.json:/requirements",
		"core/module.json:/requirements",
		"core/module.json:/components",
	}
	for _, p := range wantPaths {
		if !paths[p] {
			t.Fatalf("expected duplicate in %q, got paths: %v", p, paths)
		}
	}
}

func TestREQ5_DuplicateIDsAreErrors(t *testing.T) {
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

func TestREQ5_DuplicatesBlockRefChecks(t *testing.T) {
	// The id_dup fixture has no cross-ref errors — only duplicates.
	// Verify that only duplicate errors are returned (ref checks skipped).
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	for _, e := range errs {
		if !strings.Contains(e.Message, "duplicate ID") {
			t.Fatalf("expected only duplicate errors when duplicates exist, got: %s", e.Message)
		}
	}
}

func TestREQ6_DanglingRefsDetected(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	if len(errs) == 0 {
		t.Fatal("expected dangling reference errors, got none")
	}

	// Collect all error messages.
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}
	allMsgs := strings.Join(msgs, " | ")

	// Verify each type of dangling reference is caught.
	wantFragments := []string{
		"requires_module references non-existent module 99",
		"groups references non-existent module 99",
		"implements references non-existent requirement 99",
		"uses references non-existent component 99",
		"describes references non-existent component 99",
		"depends_on references non-existent requirement 99",
		"preq_id references non-existent project requirement 99",
	}
	for _, frag := range wantFragments {
		if !strings.Contains(allMsgs, frag) {
			t.Fatalf("expected error containing %q, got: %s", frag, allMsgs)
		}
	}

	// Verify project-level depends_on is exercised (not just module-level).
	found := false
	for _, e := range errs {
		if strings.HasPrefix(e.Path, "project.json:/requirements") &&
			strings.Contains(e.Message, "depends_on") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a project-level depends_on error with project.json:/requirements path")
	}
}

func TestREQ6_DanglingRefsAreErrors(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
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

	// Project-level errors should reference project.json.
	// Module-level errors should reference the module path.
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
