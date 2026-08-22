package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/schema"
)

// REQ-12: Test coverage checking — every component must be described by at
// least one test_section.

func TestREQ12_AllComponentsCovered(t *testing.T) {
	// T1: All components covered → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_all"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_OneUncoveredComponent(t *testing.T) {
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
	// T4: Module with no test_sections → one error per component.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_no_test_sections"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (one per component), got %d: %v", len(errs), errs)
	}
}

func TestREQ12_NoComponents(t *testing.T) {
	// T5: Module with no components → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_no_components"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for empty components, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_EmptyDescribesArray(t *testing.T) {
	// T6: test_section with empty describes → all components uncovered.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_empty_describes"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_ComponentCoveredByMultipleTestSections(t *testing.T) {
	// T7: Component covered by two test_sections → zero errors.
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_multi_covered"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ12_SingleTestSectionCoversAll(t *testing.T) {
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
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_one_uncovered"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "id:000000000002") {
		t.Fatalf("expected component ID in message, got: %s", errs[0].Message)
	}
}

func TestREQ12_LargeModuleCountPerformance(t *testing.T) {
	// E5: 100 modules, each with 10 fully-covered components, completes
	// within the 1-second performance budget and returns zero errors.
	const numModules = 100
	const numComponents = 10

	dir := t.TempDir()
	proj := schema.Project{Name: "perf-test"}

	for m := 0; m < numModules; m++ {
		modName := fmt.Sprintf("module%03d", m)
		proj.Modules = append(proj.Modules, schema.Module{
			ID:   schema.IdentityHash("module", modName),
			Name: modName,
			Path: modName,
		})

		mod := schema.ModuleSpec{Name: modName}
		var describes []string
		for c := 0; c < numComponents; c++ {
			compName := fmt.Sprintf("comp%03d", c)
			id := schema.IdentityHash(modName, "component", compName)
			mod.Components = append(mod.Components, schema.Component{
				ID:      id,
				Name:    compName,
				Content: "arch_" + compName + ".md",
			})
			describes = append(describes, id)
		}
		mod.TestSections = []schema.TestSection{{
			ID:        schema.IdentityHash(modName, "test_section", "all"),
			Name:      "all tests",
			Content:   "test_all.md",
			Describes: describes,
		}}

		modDir := filepath.Join(dir, modName)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		modData, err := json.Marshal(mod)
		if err != nil {
			t.Fatalf("marshal module: %v", err)
		}
		writeFile(t, modDir, "module.json", string(modData))
	}

	projData, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}
	writeProject(t, dir, string(projData))

	start := time.Now()
	errs := CheckTestCoverage(dir)
	elapsed := time.Since(start)

	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
	if elapsed > time.Second {
		t.Fatalf("expected completion within 1 second, took %s", elapsed)
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
