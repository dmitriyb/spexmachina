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
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_all_covered"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_UncoveredProjectRequirement(t *testing.T) {
	// Project req 2 "Feature B" has no module requirement with preq_id=2.
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_project_req"))
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
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_module_req"))
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
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_both_uncovered"))
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
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_multi_module"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_NoRequirements(t *testing.T) {
	// No project requirements → nothing to check → zero errors.
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_no_requirements"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ14_ErrorMessageContainsReqID(t *testing.T) {
	errs := CheckRequirementCoverage(filepath.Join("testdata", "reqcov_uncovered_project_req"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "2") {
		t.Fatalf("expected requirement ID in message, got: %s", errs[0].Message)
	}
}

func TestREQ14_SelfValidateRequirementCoverage(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckRequirementCoverage(specDir)
	for _, e := range errs {
		t.Logf("uncovered requirement: %s — %s", e.Path, e.Message)
	}
}
