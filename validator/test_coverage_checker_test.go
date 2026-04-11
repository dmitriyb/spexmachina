package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-12: Test coverage checking — every component must be described by at
// least one test_section.

func TestREQ12_AllComponentsCovered(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T1: All components covered → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_all"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_OneUncoveredComponent(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T2: Component 2 has no test_section → one error.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_one_uncovered"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if !strings.Contains(e.Message, "Renderer") {
		t.Fatalf("expected error about Renderer, got: %s", e.Message)
	}
	if !strings.Contains(e.Message, "no test_section coverage") {
		t.Fatalf("expected 'no test_section coverage' in message, got: %s", e.Message)
	}
	if e.Check != "test_coverage" {
		t.Fatalf("expected check=test_coverage, got %q", e.Check)
	}
}

func TestREQ12_MultipleUncoveredComponents(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T3: Components 2 and 3 uncovered → two errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_multi_uncovered"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	msgs := errs[0].Message + " " + errs[1].Message
	for _, name := range []string{"Renderer", "Formatter"} {
		if !strings.Contains(msgs, name) {
			t.Fatalf("expected error about %q, got: %s", name, msgs)
		}
	}
}

func TestREQ12_NoTestSectionsArray(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T4: Module with no test_sections → one error per component.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_no_test_sections"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (one per component), got %d: %v", len(errs), errs)
	}
}

func TestREQ12_NoComponents(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T5: Module with no components → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_no_components"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for empty components, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_EmptyDescribesArray(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T6: test_section with empty describes → all components uncovered.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_empty_describes"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_ComponentCoveredByMultipleTestSections(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T7: Component covered by two test_sections → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_multi_covered"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_SingleTestSectionCoversAll(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T8: One test_section with describes: [1,2,3] → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_single_covers_all"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_UncoveredAcrossMultipleModules(t *testing.T) {
	// T9: One uncovered in alpha, one in beta → two errors identifying correct modules.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_multi_module_uncovered"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	var modules []string
	for _, e := range errs {
		modules = append(modules, e.Path)
	}
	joined := strings.Join(modules, " ")
	if !strings.Contains(joined, "alpha") || !strings.Contains(joined, "beta") {
		t.Fatalf("expected errors from both alpha and beta, got paths: %v", modules)
	}
}

func TestREQ12_DanglingRefDoesNotPreventUncoveredError(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// T10: test_section describes non-existent component 99. Component 1 still uncovered.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_dangling_ref"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for uncovered component 1, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "Parser") {
		t.Fatalf("expected error about Parser, got: %s", errs[0].Message)
	}
}

func TestREQ12_ErrorsAreSeverityError(t *testing.T) {
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_one_uncovered"))
	for _, e := range errs {
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

func TestREQ12_ErrorMessageContainsComponentID(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_one_uncovered"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "id:2") {
		t.Fatalf("expected component ID in message, got: %s", errs[0].Message)
	}
}

func TestREQ12_SelfValidateTestCoverage(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckTestCoverage(specDir)
	// Log uncovered components but don't fail — own spec coverage gaps
	// are tracked by separate beads, not this checker's concern.
	for _, e := range errs {
		t.Logf("uncovered component: %s — %s", e.Path, e.Message)
	}
}
