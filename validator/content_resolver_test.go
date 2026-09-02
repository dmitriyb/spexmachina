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

// S8: missing component content file — the fixture's only mutation is the
// deleted file, so the error set is closed at one, and the path names the
// declaring node, not the missing file.

func TestS8_MissingComponentContentFile(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing_component"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "content" {
		t.Fatalf("expected check=content, got %q", e.Check)
	}
	wantPath := "alpha/module.json:/components/Widget/content"
	if e.Path != wantPath {
		t.Fatalf("expected path %q, got %q", wantPath, e.Path)
	}
	wantMsg := "content file not found: arch_widget.md"
	if e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

// S9/E5: missing data_flow content file — ContentResolver walks data_flows
// content paths the same way it walks components. The fixture's only
// mutation is the deleted file, so the error set is closed at one.

func TestS9_MissingDataFlowContentFile(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing_dataflow"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "content" {
		t.Fatalf("expected check=content, got %q", e.Check)
	}
	wantPath := "alpha/module.json:/data_flows/Widget data/content"
	if e.Path != wantPath {
		t.Fatalf("expected path %q, got %q", wantPath, e.Path)
	}
	wantMsg := "content file not found: flow_widget_data.md"
	if e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

func TestREQ2_MissingContentFile(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing"))
	if len(errs) == 0 {
		t.Fatal("expected errors for missing content files, got none")
	}
	// arch_parser.md and flow_missing.md are missing
	wantMissing := []string{"arch_parser.md", "flow_missing.md"}
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

// REQ-11: Test content path resolution — verify test_sections content paths
// are walked and validated by ContentResolver.

func TestREQ11_ValidTestSectionContent(t *testing.T) {
	// S7 extended: content_valid fixture now includes a test_section with existing file.
	errs := CheckContentPaths(filepath.Join("testdata", "content_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid content paths including test_sections, got %d: %v", len(errs), errs)
	}
}

func TestREQ11_MissingTestSectionContent(t *testing.T) {
	// S10: test_section references test_widget_behavior.md but file is missing.
	// The fixture's only mutation is the deleted file, so the error set is
	// closed at one; the path names the declaring node (the test_section),
	// per the Shared Assertions convention.
	errs := CheckContentPaths(filepath.Join("testdata", "content_missing_test_section"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "content" {
		t.Fatalf("expected check=content, got %q", e.Check)
	}
	wantPath := "alpha/module.json:/test_sections/Widget behavior/content"
	if e.Path != wantPath {
		t.Fatalf("expected path %q, got %q", wantPath, e.Path)
	}
	if !strings.Contains(e.Message, "test_widget_behavior.md") {
		t.Fatalf("expected message mentioning test_widget_behavior.md, got %q", e.Message)
	}
}

func TestREQ11_MultiMissingAcrossSections(t *testing.T) {
	// S13: component, data_flow, and test_section content all missing —
	// exactly three errors, each with the path identifying which section
	// referenced it.
	errs := CheckContentPaths(filepath.Join("testdata", "content_multi_missing"))
	if len(errs) != 3 {
		t.Fatalf("expected exactly 3 errors (component + data_flow + test_section), got %d: %v", len(errs), errs)
	}
	wants := map[string]string{
		"arch_widget.md":          "/components/",
		"flow_widget_data.md":     "/data_flows/",
		"test_widget_behavior.md": "/test_sections/",
	}
	for file, pluralSeg := range wants {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, file) {
				if e.Check != "content" {
					t.Fatalf("expected check=content for %q, got %q", file, e.Check)
				}
				if !strings.Contains(e.Path, pluralSeg) {
					t.Fatalf("expected path for %q to contain %q, got %q", file, pluralSeg, e.Path)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error mentioning %q, got: %v", file, errs)
		}
	}
}

func TestREQ11_TestSectionPathTraversal(t *testing.T) {
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

// S18: ContentResolver walks whichever module-scoped types the resolved
// profile marks content-bearing, not a fixed three.

func TestS18_ProfileDeclaredContentBearingTypeWalked(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_profile_endpoint"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error for the endpoint's missing content, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "content" {
		t.Fatalf("expected check=content, got %q", e.Check)
	}
	wantPath := "core/module.json:/endpoints/Get widget/content"
	if e.Path != wantPath {
		t.Fatalf("expected path %q, got %q", wantPath, e.Path)
	}
	if !strings.Contains(e.Message, "endpoint_get_widget.md") {
		t.Fatalf("expected message mentioning endpoint_get_widget.md, got: %q", e.Message)
	}
}

// E4: a content file that exists but is empty is not an error — ContentResolver
// only checks existence, never opens the file.

func TestE4_EmptyContentFileNotAnError(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_empty_file"))
	if len(errs) != 0 {
		t.Fatalf("expected no errors for an empty but present content file, got %d: %v", len(errs), errs)
	}
}

// E6: Unicode file names resolve correctly.

func TestE6_UnicodeContentFileNameResolves(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_unicode"))
	if len(errs) != 0 {
		t.Fatalf("expected no errors for a unicode content file name, got %d: %v", len(errs), errs)
	}
}

// S15: content checks run across multiple modules — a missing file in one
// module is reported, and no error references the other, fully valid module.

func TestS15_ContentChecksAcrossMultipleModules(t *testing.T) {
	errs := CheckContentPaths(filepath.Join("testdata", "content_multi_module"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "content" {
		t.Fatalf("expected check=content, got %q", e.Check)
	}
	if !strings.Contains(e.Message, "arch_compb.md") {
		t.Fatalf("expected error mentioning arch_compb.md, got: %v", e)
	}
	if !strings.Contains(e.Path, "beta/module.json") {
		t.Fatalf("expected path referencing beta/module.json, got %q", e.Path)
	}
	for _, e := range errs {
		if strings.Contains(e.Path, "alpha") {
			t.Fatalf("expected no error referencing alpha, got: %v", errs)
		}
	}
}

func TestREQ2_SelfValidateContent(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckContentPaths(specDir)
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should have no content errors, got %d errors: %v", len(errs), errs)
	}
}
