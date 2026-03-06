package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-4: Orphan detection — find requirements not implemented by any component
// and components not described by any impl_section.

func TestREQ4_NoOrphansReturnsEmpty(t *testing.T) {
	errs := DetectOrphans(filepath.Join("testdata", "orphan_none"))
	if len(errs) > 0 {
		t.Fatalf("expected no orphans, got %d: %v", len(errs), errs)
	}
}

func TestREQ4_OrphanRequirementsDetected(t *testing.T) {
	errs := DetectOrphans(filepath.Join("testdata", "orphan_reqs"))
	if len(errs) == 0 {
		t.Fatal("expected orphan requirement warnings, got none")
	}

	var orphanReqs []ValidationError
	for _, e := range errs {
		if strings.Contains(e.Message, "not implemented by any component") {
			orphanReqs = append(orphanReqs, e)
		}
	}
	if len(orphanReqs) != 2 {
		t.Fatalf("expected 2 orphan requirements, got %d: %v", len(orphanReqs), orphanReqs)
	}

	// Verify specific orphans are found.
	msgs := orphanReqs[0].Message + " " + orphanReqs[1].Message
	for _, title := range []string{"Orphan feature", "Another orphan"} {
		if !strings.Contains(msgs, title) {
			t.Fatalf("expected orphan %q in messages, got: %s", title, msgs)
		}
	}
}

func TestREQ4_OrphanComponentsDetected(t *testing.T) {
	errs := DetectOrphans(filepath.Join("testdata", "orphan_comps"))
	if len(errs) == 0 {
		t.Fatal("expected orphan component warnings, got none")
	}

	var orphanComps []ValidationError
	for _, e := range errs {
		if strings.Contains(e.Message, "not described by any impl_section") {
			orphanComps = append(orphanComps, e)
		}
	}
	if len(orphanComps) != 2 {
		t.Fatalf("expected 2 orphan components, got %d: %v", len(orphanComps), orphanComps)
	}

	msgs := orphanComps[0].Message + " " + orphanComps[1].Message
	for _, name := range []string{"Orphan Widget", "Lonely Service"} {
		if !strings.Contains(msgs, name) {
			t.Fatalf("expected orphan %q in messages, got: %s", name, msgs)
		}
	}
}

func TestREQ4_OrphansAreWarnings(t *testing.T) {
	dirs := []string{"orphan_reqs", "orphan_comps"}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			errs := DetectOrphans(filepath.Join("testdata", dir))
			for _, e := range errs {
				if e.Check != "orphan" {
					t.Fatalf("expected check=orphan, got %q", e.Check)
				}
				if e.Severity != "warning" {
					t.Fatalf("expected severity=warning, got %q", e.Severity)
				}
			}
		})
	}
}

func TestREQ4_OrphanPathIncludesModule(t *testing.T) {
	errs := DetectOrphans(filepath.Join("testdata", "orphan_reqs"))
	for _, e := range errs {
		if !strings.Contains(e.Path, "core/module.json") {
			t.Fatalf("expected path to include module reference, got: %s", e.Path)
		}
	}
}

func TestREQ4_SelfValidateOrphans(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := DetectOrphans(specDir)
	// Orphans are warnings — log them but don't fail.
	for _, e := range errs {
		if e.Severity == "error" {
			t.Fatalf("unexpected error in own spec: %v", e)
		}
		t.Logf("orphan warning: %s — %s", e.Path, e.Message)
	}
}
