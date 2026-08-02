package proposal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFR1_DetectType_Project(t *testing.T) {
	content := "# My Proposal\n\n## Vision\n\nSome vision.\n"
	ptype, err := detectType(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ptype != "project" {
		t.Errorf("want type %q, got %q", "project", ptype)
	}
}

func TestFR1_DetectType_Change(t *testing.T) {
	content := "# Change\n\n## Proposed change\n\nSome change.\n"
	ptype, err := detectType(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ptype != "change" {
		t.Errorf("want type %q, got %q", "change", ptype)
	}
}

func TestFR1_DetectType_CaseInsensitive(t *testing.T) {
	// H2 text matching is case-insensitive via extractH2Headings + detectType
	content := "# Proposal\n\n## VISION\n\nsome vision\n"
	ptype, err := detectType(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ptype != "project" {
		t.Errorf("want type %q, got %q", "project", ptype)
	}
}

func TestFR1_DetectType_Unknown(t *testing.T) {
	content := "# Something\n\n## Background\n\n## Plan\n"
	_, err := detectType(content)
	if err == nil {
		t.Fatal("want error for undetectable type, got nil")
	}
	if !strings.Contains(err.Error(), "cannot detect type from headings") {
		t.Errorf("want error about headings, got: %v", err)
	}
}

func TestFR1_ValidateSections_ProjectComplete(t *testing.T) {
	content := "# P\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	err := validateSections(content, "project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFR1_ValidateSections_ProjectMissing(t *testing.T) {
	content := "# P\n\n## Vision\n\nx\n"
	err := validateSections(content, "project")
	if err == nil {
		t.Fatal("want error for missing sections, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "modules") {
		t.Errorf("want error about modules, got: %v", err)
	}
	if !strings.Contains(msg, "key requirements") {
		t.Errorf("want error about key requirements, got: %v", err)
	}
	if !strings.Contains(msg, "design decisions") {
		t.Errorf("want error about design decisions, got: %v", err)
	}
}

func TestFR1_ValidateSections_ChangeComplete(t *testing.T) {
	content := "# C\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	err := validateSections(content, "change")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFR1_ValidateSections_ChangeMissing(t *testing.T) {
	content := "# C\n\n## Context\n\nx\n"
	err := validateSections(content, "change")
	if err == nil {
		t.Fatal("want error for missing sections, got nil")
	}
	if !strings.Contains(err.Error(), "proposed change") {
		t.Errorf("want error about proposed change, got: %v", err)
	}
}

func TestFR1_ExtractH2Headings(t *testing.T) {
	content := "# Title\n\n## First\n\ntext\n\n## Second\n\nmore text\n\n### Not H2\n"
	headings := extractH2Headings(content)
	if len(headings) != 2 {
		t.Fatalf("want 2 headings, got %d: %v", len(headings), headings)
	}
	if headings[0] != "First" {
		t.Errorf("want %q, got %q", "First", headings[0])
	}
	if headings[1] != "Second" {
		t.Errorf("want %q, got %q", "Second", headings[1])
	}
}

func TestFR1_SlugFromHeading(t *testing.T) {
	tests := []struct {
		name    string
		content string
		file    string
		want    string
	}{
		{"from H1", "# Change Proposal: My Cool Feature\n\n## Context\n", "input.md", "my-cool-feature"},
		{"fallback to filename", "No heading here\n", "my-input.md", "my-input"},
		{"strips special chars", "# Project Proposal: Spex Machina (v2)\n", "x.md", "spex-machina-v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugFromHeading(tt.content, tt.file)
			if got != tt.want {
				t.Errorf("slugFromHeading = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFR1_ToSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Special!@#Characters", "special-characters"},
		{"trailing-dash-", "trailing-dash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSlug(tt.input)
			if got != tt.want {
				t.Errorf("toSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFR1_Register_ValidChange(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	inputDir := filepath.Join(tmp, "input")
	os.MkdirAll(inputDir, 0755)

	content := "# Change Proposal: Test Change\n\n## Context\n\nSome context.\n\n## Proposed change\n\nSome change.\n\n## Impact expectation\n\nSome impact.\n"
	inputFile := filepath.Join(inputDir, "test-change.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	filename, err := Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !strings.HasSuffix(filename, "-test-change.md") {
		t.Errorf("want filename ending with -test-change.md, got %q", filename)
	}

	destPath := filepath.Join(specDir, "proposals", filename)
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Errorf("registered file does not exist: %s", destPath)
	}
}

func TestFR1_Register_ValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Bad Proposal\n\n## Background\n\n## Plan\n"
	inputFile := filepath.Join(tmp, "bad.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	_, err := Register(inputFile, specDir)
	if err == nil {
		t.Fatal("want error for invalid proposal, got nil")
	}
	if !strings.Contains(err.Error(), "cannot detect type from headings") {
		t.Errorf("want heading detection error, got: %v", err)
	}
}

func TestFR1_Register_NonexistentFile(t *testing.T) {
	tmp := t.TempDir()
	_, err := Register(filepath.Join(tmp, "ghost.md"), tmp)
	if err == nil {
		t.Fatal("want error for nonexistent file, got nil")
	}
}

func TestFR1_Register_Idempotency(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: Idempotent\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "idempotent.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	_, err := Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err = Register(inputFile, specDir)
	if err == nil {
		t.Fatal("want error on second register, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("want 'already registered' error, got: %v", err)
	}
}

// TestFR30_S6_Register_PreservesDatedFilename covers S6: a source already
// following YYYY-MM-DD-<name>.md keeps its own name. Re-dating it would file
// the proposal under a date it was not written on and break the
// spec_proposal:<stem> label every bead filed against it already carries.
func TestFR30_S6_Register_PreservesDatedFilename(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	// The H1 slug is deliberately unrelated to the filename: if the registrar
	// re-derived the name, the result would be today-caching-layer.md.
	content := "# Change Proposal: Caching Layer\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "2026-05-10-caching.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	filename, err := Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if filename != "2026-05-10-caching.md" {
		t.Fatalf("want the original dated name preserved, got %q", filename)
	}
	if _, err := os.Stat(filepath.Join(specDir, "proposals", "2026-05-10-caching.md")); err != nil {
		t.Fatalf("copy not written under the preserved name: %v", err)
	}
}

// TestFR30_S7_Register_DatesAnUndatedFilename is S6's other side: a source
// that does not follow the convention is renamed to today plus a slug from
// its first heading.
func TestFR30_S7_Register_DatesAnUndatedFilename(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Project Proposal: Add Caching Layer\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	inputFile := filepath.Join(tmp, "valid-project.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	filename, err := Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	want := time.Now().Format("2006-01-02") + "-add-caching-layer.md"
	if filename != want {
		t.Fatalf("want %q, got %q", want, filename)
	}
}

func TestNFR4_Register_PreservesFile(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: Preserve\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "preserve.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	filename, err := Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	destContent, err := os.ReadFile(filepath.Join(specDir, "proposals", filename))
	if err != nil {
		t.Fatalf("read registered file: %v", err)
	}
	if string(destContent) != content {
		t.Error("registered file content does not match original")
	}

	// Original file should still exist.
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		t.Error("original file was removed")
	}
}
