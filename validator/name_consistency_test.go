package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-10: Module name consistency — project.json names must match module.json
// names exactly. Lowercase convention enforced. Mismatches reported with both
// values and a suggested fix.

func TestREQ10_MatchingNamesPass(t *testing.T) {
	// N1: project.json name "alpha" matches module.json name "alpha" → zero errors.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_consistent"))
	alphaErrs := filterByPath(errs, "alpha")
	if len(alphaErrs) > 0 {
		t.Fatalf("expected no errors for alpha, got %d: %v", len(alphaErrs), alphaErrs)
	}
}

func TestREQ10_AllModulesConsistent(t *testing.T) {
	// N2: Every module in project.json matches its module.json name → empty error slice.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_consistent"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ10_CaseMismatchWithFixSuggestion(t *testing.T) {
	// N3: project.json "alpha" vs module.json "Alpha" → case mismatch with fix suggestion.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_case_mismatch"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "name_consistency" {
		t.Fatalf("expected check=name_consistency, got %q", e.Check)
	}
	if !strings.Contains(e.Message, "alpha") || !strings.Contains(e.Message, "Alpha") {
		t.Fatalf("expected message to contain both names, got: %s", e.Message)
	}
	if !strings.Contains(strings.ToLower(e.Message), "change") {
		t.Fatalf("expected fix suggestion in message, got: %s", e.Message)
	}
}

func TestREQ10_EntirelyDifferentNames(t *testing.T) {
	// N4: project.json "alpha" vs module.json "widget" → name conflict, no fix suggestion.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_conflict"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if !strings.Contains(e.Message, "alpha") || !strings.Contains(e.Message, "widget") {
		t.Fatalf("expected both names in message, got: %s", e.Message)
	}
	// Should NOT contain a fix suggestion since names are entirely different.
	if strings.Contains(strings.ToLower(e.Message), "change module.json name to") {
		t.Fatalf("should not suggest fix for entirely different names, got: %s", e.Message)
	}
}

func TestREQ10_UppercaseNameViolatesConvention(t *testing.T) {
	// N5: Both project.json and module.json have "Alpha" — names match but violate lowercase.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_uppercase_convention"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for uppercase violation, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if !strings.Contains(strings.ToLower(e.Message), "lowercase") {
		t.Fatalf("expected lowercase convention message, got: %s", e.Message)
	}
}

func TestREQ10_MultipleMismatchesAcrossModules(t *testing.T) {
	// N6: alpha has case mismatch, beta has name conflict → two errors.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_multi_mismatch"))
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	paths := errs[0].Path + " " + errs[1].Path
	if !strings.Contains(paths, "alpha") || !strings.Contains(paths, "beta") {
		t.Fatalf("expected errors for both alpha and beta, got paths: %s", paths)
	}
}

func TestREQ10_InvalidModuleJSON(t *testing.T) {
	// N7: module.json is not valid JSON → error about parsing.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_invalid_json"))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "name_consistency" {
		t.Fatalf("expected check=name_consistency, got %q", e.Check)
	}
}

func TestREQ10_HyphenatedNamesMatchExactly(t *testing.T) {
	// N8: project.json "my-module" matches module.json "my-module" → zero errors.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_hyphenated"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ10_TrailingWhitespace(t *testing.T) {
	// N9: module.json has "alpha " (trailing space) vs "alpha" → mismatch.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_trailing_space"))
	if len(errs) < 1 {
		t.Fatalf("expected at least 1 error for trailing space mismatch, got 0")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "alpha ") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error referencing 'alpha ' (with space), got: %v", errs)
	}
}

func TestREQ10_PathDiffersFromName(t *testing.T) {
	// E3: Module path "core" differs from name "core-lib" — comparison is name-to-name, not path.
	errs := CheckNameConsistency(filepath.Join("testdata", "name_path_differs"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors when path differs from name, got %d: %v", len(errs), errs)
	}
}

func TestREQ10_ErrorSeverity(t *testing.T) {
	errs := CheckNameConsistency(filepath.Join("testdata", "name_case_mismatch"))
	for _, e := range errs {
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

func TestREQ10_SelfValidateNameConsistency(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckNameConsistency(specDir)
	for _, e := range errs {
		t.Logf("name consistency issue: %s — %s", e.Path, e.Message)
	}
}

func filterByPath(errs []ValidationError, substr string) []ValidationError {
	var result []ValidationError
	for _, e := range errs {
		if strings.Contains(e.Path, substr) {
			result = append(result, e)
		}
	}
	return result
}
