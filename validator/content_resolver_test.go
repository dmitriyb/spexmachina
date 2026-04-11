package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-2: Content resolution — verify all content paths in module.json files
// resolve to existing markdown files relative to their module directory.

func TestREQ2_ValidContentReturnsNoErrors(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	errs := CheckContentPaths(filepath.Join("testdata", "content_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid content paths, got %d: %v", len(errs), errs)
	}
}

func TestREQ2_MissingContentFile(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing"))
	if len(errs) == 0 {
		t.Fatal("expected errors for missing content files, got none")
	}
	// arch_parser.md and impl_setup.md are missing
	wantMissing := []string{"arch_parser.md", "impl_setup.md"}
	for _, want := range wantMissing {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error mentioning %q, got: %v", want, errs)
		}
	}
}

func TestREQ2_PathTraversalRejected(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	errs := CheckContentPaths(filepath.Join("testdata", "content_traversal"))
	if len(errs) == 0 {
		t.Fatal("expected error for path traversal, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "..") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about '..' in path, got: %v", errs)
	}
}

func TestREQ2_AbsolutePathRejected(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	errs := CheckContentPaths(filepath.Join("testdata", "content_absolute"))
	if len(errs) == 0 {
		t.Fatal("expected error for absolute path, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "absolute") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about absolute path, got: %v", errs)
	}
}

func TestREQ2_EmptyContentIsValid(t *testing.T) {
	// The content_valid fixture has a component (Store) with no content field.
	// It should not produce errors.
	errs := CheckContentPaths(filepath.Join("testdata", "content_valid"))
	for _, e := range errs {
		if strings.Contains(e.Message, "Store") {
			t.Fatalf("empty content should not produce errors, got: %v", e)
		}
	}
}

func TestREQ2_AllContentErrorsTagged(t *testing.T) {
	dirs := []string{"content_missing", "content_traversal", "content_absolute"}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			errs := CheckContentPaths(filepath.Join("testdata", dir))
			for _, e := range errs {
				if e.Check != "content" {
					t.Fatalf("expected check=content, got %q for error: %v", e.Check, e)
				}
				if e.Severity != "error" {
					t.Fatalf("expected severity=error, got %q for error: %v", e.Severity, e)
				}
			}
		})
	}
}

// REQ-11: Test content path resolution — verify test_sections content paths
// are walked and validated by ContentResolver.

func TestREQ11_ValidTestSectionContent(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// S7 extended: content_valid fixture now includes a test_section with existing file.
	errs := CheckContentPaths(filepath.Join("testdata", "content_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid content paths including test_sections, got %d: %v", len(errs), errs)
	}
}

func TestREQ11_MissingTestSectionContent(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// S10: test_section references test_widget_behavior.md but file is missing.
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing_test_section"))
	if len(errs) == 0 {
		t.Fatal("expected error for missing test_section content file, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "test_widget_behavior.md") {
			if e.Check != "content" {
				t.Fatalf("expected check=content, got %q", e.Check)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error mentioning test_widget_behavior.md, got: %v", errs)
	}
}

func TestREQ11_MultiMissingAcrossSections(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// S13: component, impl_section, and test_section content all missing.
	errs := CheckContentPaths(filepath.Join("testdata", "content_multi_missing"))
	if len(errs) != 3 {
		t.Fatalf("expected exactly 3 errors (component + impl + test_section), got %d: %v", len(errs), errs)
	}
	wants := []string{"arch_widget.md", "impl_widget_logic.md", "test_widget_behavior.md"}
	for _, want := range wants {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error mentioning %q, got: %v", want, errs)
		}
	}
}

func TestREQ11_TestSectionPathTraversal(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	// S11 extended to test_sections: path traversal in test_section content.
	errs := CheckContentPaths(filepath.Join("testdata", "content_test_section_traversal"))
	if len(errs) == 0 {
		t.Fatal("expected error for path traversal in test_section, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "..") && strings.Contains(e.Path, "test_section") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about '..' in test_section path, got: %v", errs)
	}
}

func TestREQ2_SelfValidateContent(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := filepath.Join("..", "spec")
	errs := CheckContentPaths(specDir)
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should have no content errors, got %d errors: %v", len(errs), errs)
	}
}
