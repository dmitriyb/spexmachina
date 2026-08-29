package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/internal/perf"
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

	var errs []ValidationError
	perf.Within(t, time.Second, func() {
		errs = CheckTestCoverage(dir)
	})

	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// CC1: a profile identical to the default except the component-to-
// test_section coverage link is absent drops the check entirely — the same
// two-component, one-covered fixture that yields the T2 error under the
// default profile (coverage_one_uncovered) yields zero errors here, because
// the checker enforces only the chains the profile declares.
func TestREQ12_CC1_DroppedChainSkipsCheck(t *testing.T) {
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_profile_dropped"))
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors under profile dropping the link, got %d: %v", len(errs), errs)
	}

	defaultErrs := CheckTestCoverage(filepath.Join("testdata", "coverage_one_uncovered"))
	if len(defaultErrs) != 1 {
		t.Fatalf("expected the same fixture shape to still yield 1 error under the default profile, got %d: %v", len(defaultErrs), defaultErrs)
	}
}

// CC2: a profile that renames "component" to "endpoint" (same shape,
// different name) is still enforced — the error carries T2's shape with the
// declared type names interpolated, naming the uncovered node an "endpoint"
// rather than the literal word "component".
func TestREQ12_CC2_RenamedCoveredTypeInterpolated(t *testing.T) {
	errs := CheckTestCoverage(filepath.Join("testdata", "coverage_profile_renamed"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	want := `endpoint PostWidget (id:000000000002) has no test_section coverage`
	if errs[0].Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", errs[0].Message, want)
	}
	if errs[0].Path != "alpha/module.json:/endpoints/000000000002" {
		t.Fatalf("unexpected path: %s", errs[0].Path)
	}
}

// CC3: the acceptance test that the profile is load-bearing, not decorative.
// A fixture authored against a profile that both renames the covered type
// and drops the coverage chain validates cleanly under that profile under
// the full ten-check pipeline, and fails schema conformance once the profile
// file is removed and the spec is judged against the default profile's
// "components" array instead.
func TestREQ12_CC3_ProfileIsLoadBearing(t *testing.T) {
	dir := filepath.Join("testdata", "coverage_profile_load_bearing")

	withProfile := runValidationPipeline(dir)
	if len(withProfile) != 0 {
		t.Fatalf("expected 0 errors with profile.json in place, got %d: %v", len(withProfile), withProfile)
	}

	tmp := t.TempDir()
	copyDirExceptProfile(t, dir, tmp)

	withoutProfile := runValidationPipeline(tmp)
	if len(withoutProfile) == 0 {
		t.Fatalf("expected errors once profile.json is removed and the default profile applies")
	}

	found := false
	for _, e := range withoutProfile {
		if e.Check == "schema" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a schema-conformance error for the renamed type's array, got: %v", withoutProfile)
	}
}

// runValidationPipeline runs the full ten-check validation pipeline
// flow_validation_pipeline.md declares, in the same order cmd/spex's
// validate command does, discarding disclosure notes — CC3's concern is
// only whether the entry list is empty.
func runValidationPipeline(specDir string) []ValidationError {
	var errs []ValidationError
	errs = append(errs, CheckSchema(specDir)...)
	errs = append(errs, CheckContentPaths(specDir)...)
	errs = append(errs, CheckLinks(specDir)...)
	errs = append(errs, CheckIDs(specDir)...)
	errs = append(errs, CheckIDDerivation(specDir)...)
	errs = append(errs, CheckDAG(specDir)...)
	errs = append(errs, CheckNameConsistency(specDir)...)
	errs = append(errs, CheckTestCoverage(specDir)...)
	reqCovErrs, _ := CheckRequirementCoverage(specDir)
	errs = append(errs, reqCovErrs...)
	errs = append(errs, CheckCoupledSections(specDir)...)
	return errs
}

// copyDirExceptProfile copies src into dst, skipping profile.json — used by
// CC3 to reproduce a fixture without its profile file so the default
// profile applies instead.
func copyDirExceptProfile(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read dir %s: %v", src, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				t.Fatalf("mkdir %s: %v", dstPath, err)
			}
			copyDirExceptProfile(t, srcPath, dstPath)
			continue
		}
		if entry.Name() == "profile.json" {
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			t.Fatalf("write %s: %v", dstPath, err)
		}
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
