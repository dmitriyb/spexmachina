package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-2: Content resolution — verify all content paths in module.json files
// resolve to existing markdown files relative to their module directory.

func TestREQ2_ValidContentReturnsNoErrors(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid content paths, got %d: %v", len(errs), errs)
	}
}

func TestREQ2_MissingContentFile(t *testing.T) {
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

func TestREQ2_SelfValidateContent(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckContentPaths(specDir)
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should have no content errors, got %d errors: %v", len(errs), errs)
	}
}
