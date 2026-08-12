package proposal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/mapping"
)

// --- fixture content ---

const validProjectContent = "# Project Proposal: Add Caching Layer\n\n## Vision\n\nSome vision.\n\n## Modules\n\nSome modules.\n\n## Key requirements\n\nSome requirements.\n\n## Design decisions\n\nSome decisions.\n"

const validChangeContent = "# Change Proposal: New Change\n\n## Context\n\nSome context.\n\n## Proposed change\n\nSome change.\n\n## Impact expectation\n\nSome impact.\n"

const partialProjectContent = "# Project Proposal: Partial\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n"

const partialChangeContent = "# Change Proposal: Partial\n\n## Context\n\nx\n\n## Proposed change\n\nx\n"

// fixtureHead is the caller-supplied git head every Register call in this
// file carries, matching the fixture test_registration.md pins.
const fixtureHead = "cafe1234"

// --- helpers ---

// newSpecDir creates a temp spec/ directory with an empty proposals/
// subdirectory (matching the Setup fixture) and returns its path.
func newSpecDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatalf("mkdir spec/proposals: %v", err)
	}
	return specDir
}

// writeInput writes content to <tmp>/input/<name> and returns its path.
func writeInput(t *testing.T, specDir, name, content string) string {
	t.Helper()
	inputDir := filepath.Join(filepath.Dir(specDir), "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}
	path := filepath.Join(inputDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// journalEvents parses spec/.history.jsonl into events, failing the test on
// any parse error.
func journalEvents(t *testing.T, specDir string) []mapping.Event {
	t.Helper()
	events, err := mapping.NewMappingStore(specDir).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	return events
}

// --- S1: Register a valid project proposal ---

func TestREQ_7885ad439bb9_S1_RegisterValidProjectProposal(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "valid-project.md", validProjectContent)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	want := today + "-add-caching-layer.md"
	if filename != want {
		t.Fatalf("filename: want %q, got %q", want, filename)
	}

	destPath := filepath.Join(specDir, "proposals", filename)
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(destContent) != validProjectContent {
		t.Fatal("copy is not byte-for-byte identical to source")
	}

	events := journalEvents(t, specDir)
	if len(events) != 1 {
		t.Fatalf("want 1 journal line, got %d: %+v", len(events), events)
	}
	stem := strings.TrimSuffix(filename, ".md")
	ev := events[0]
	if ev.Event != "registered" {
		t.Fatalf("event: want registered, got %q", ev.Event)
	}
	if ev.EID != fixtureHead+":"+stem {
		t.Fatalf("eid: want %q, got %q", fixtureHead+":"+stem, ev.EID)
	}
	if ev.Proposal != stem {
		t.Fatalf("proposal: want %q, got %q", stem, ev.Proposal)
	}
	if ev.GitHead != fixtureHead {
		t.Fatalf("git_head: want %q, got %q", fixtureHead, ev.GitHead)
	}
}

// --- S2: Register a valid change proposal ---

func TestREQ_7885ad439bb9_S2_RegisterValidChangeProposal(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "valid-change.md", validChangeContent)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !strings.HasSuffix(filename, "-new-change.md") {
		t.Fatalf("want filename ending -new-change.md, got %q", filename)
	}
	if _, err := os.Stat(filepath.Join(specDir, "proposals", filename)); err != nil {
		t.Fatalf("copy not written: %v", err)
	}
}

// --- S3: Reject project proposal with missing sections ---

func TestREQ_7885ad439bb9_S3_RejectProjectMissingSections(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "partial-project.md", partialProjectContent)

	_, err := Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error for missing sections, got nil")
	}
	if !strings.Contains(err.Error(), "design decisions") {
		t.Errorf("want error naming design decisions, got: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) != 0 {
		t.Errorf("refusal must leave no file, found %d entries", len(entries))
	}
	if events := journalEvents(t, specDir); len(events) != 0 {
		t.Errorf("refusal must leave no journal line, found %d", len(events))
	}
}

// --- S4: Reject change proposal with missing sections ---

func TestREQ_7885ad439bb9_S4_RejectChangeMissingSections(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "partial-change.md", partialChangeContent)

	_, err := Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error for missing sections, got nil")
	}
	if !strings.Contains(err.Error(), "impact expectation") {
		t.Errorf("want error naming impact expectation, got: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) != 0 {
		t.Errorf("refusal must leave no file, found %d entries", len(entries))
	}
}

// --- S5: Report all missing sections, not just the first ---

func TestREQ_7885ad439bb9_S5_ReportAllMissingSections(t *testing.T) {
	specDir := newSpecDir(t)
	content := "# Project Proposal: One Section\n\n## Vision\n\nx\n"
	input := writeInput(t, specDir, "one-section.md", content)

	_, err := Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	for _, want := range []string{"modules", "key requirements", "design decisions"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want error listing %q, got: %v", want, err)
		}
	}
}

// --- S6: Preserve existing date-prefixed filename ---

func TestREQ_7885ad439bb9_S6_PreserveExistingDatePrefixedFilename(t *testing.T) {
	specDir := newSpecDir(t)
	// H1 slug is deliberately unrelated to the filename: if the registrar
	// re-derived the name from the heading, the result would differ.
	content := "# Change Proposal: Caching Layer\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	input := writeInput(t, specDir, "2026-05-10-caching.md", content)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if filename != "2026-05-10-caching.md" {
		t.Fatalf("want preserved name, got %q", filename)
	}
}

// --- S7: Generate slug from H1 heading when filename lacks date prefix ---

func TestREQ_7885ad439bb9_S7_GenerateSlugFromH1Heading(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "valid-project.md", validProjectContent)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	want := time.Now().Format("2006-01-02") + "-add-caching-layer.md"
	if filename != want {
		t.Fatalf("want %q, got %q", want, filename)
	}
}

// --- S8: Case-insensitive section matching ---

func TestREQ_7885ad439bb9_S8_CaseInsensitiveSectionMatching(t *testing.T) {
	specDir := newSpecDir(t)
	content := "# Project Proposal: Mixed Case\n\n## VISION\n\nx\n\n## modules\n\nx\n\n## key Requirements\n\nx\n\n## design Decisions\n\nx\n"
	input := writeInput(t, specDir, "mixed-case.md", content)

	if _, err := Register(input, specDir, fixtureHead); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// --- S9: Source file does not exist ---

func TestREQ_7885ad439bb9_S9_SourceFileDoesNotExist(t *testing.T) {
	specDir := newSpecDir(t)
	ghost := filepath.Join(filepath.Dir(specDir), "input", "nonexistent.md")

	_, err := Register(ghost, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error for nonexistent source, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want wrapped fs not-exist error, got: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) != 0 {
		t.Errorf("want no file written, found %d entries", len(entries))
	}
}

// --- S10: Target proposals directory does not exist ---

func TestREQ_7885ad439bb9_S10_TargetProposalsDirectoryDoesNotExist(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir spec: %v", err)
	}
	// Deliberately no spec/proposals/ subdirectory.
	input := writeInput(t, specDir, "valid-project.md", validProjectContent)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	info, err := os.Stat(filepath.Join(specDir, "proposals"))
	if err != nil || !info.IsDir() {
		t.Fatalf("proposals dir was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(specDir, "proposals", filename)); err != nil {
		t.Fatalf("copy not written: %v", err)
	}
}

// --- E1: Empty file ---

func TestREQ_7885ad439bb9_E1_EmptyFile(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "empty.md", "")

	_, err := Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error for empty file, got nil")
	}
	if !strings.Contains(err.Error(), "cannot detect type from headings") {
		t.Errorf("want descriptive detection error, got: %v", err)
	}
}

// --- E2: File with H2 headings but no recognizable type ---

func TestREQ_7885ad439bb9_E2_UnrecognizableHeadings(t *testing.T) {
	specDir := newSpecDir(t)
	content := "# Something\n\n## Introduction\n\nx\n\n## Conclusion\n\nx\n"
	input := writeInput(t, specDir, "no-headings.md", content)

	_, err := Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot detect type from headings") {
		t.Errorf("want descriptive detection error, got: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) != 0 {
		t.Errorf("want no file written, found %d entries", len(entries))
	}
}

// --- E3: Duplicate registration ---

func TestREQ_7885ad439bb9_E3_DuplicateRegistration(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "2026-03-10-some-change.md", validChangeContent)

	first, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}
	destPath := filepath.Join(specDir, "proposals", first)
	before, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read first copy: %v", err)
	}

	_, err = Register(input, specDir, fixtureHead)
	if err == nil {
		t.Fatal("want error on second registration, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("want 'already registered' error, got: %v", err)
	}

	after, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read after second attempt: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("existing file was overwritten")
	}
}

// --- E3b: Crash recovery — event appended, file not copied ---

func TestREQ_7885ad439bb9_E3b_CrashRecoveryEventAppendedFileNotCopied(t *testing.T) {
	specDir := newSpecDir(t)
	input := writeInput(t, specDir, "2026-03-10-some-change.md", validChangeContent)
	stem := "2026-03-10-some-change"
	eid := fixtureHead + ":" + stem

	store := mapping.NewMappingStore(specDir)
	if err := store.Append([]mapping.Event{{
		Event: "registered", EID: eid, Proposal: stem, GitHead: fixtureHead,
	}}); err != nil {
		t.Fatalf("seed journal: %v", err)
	}

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if filename != "2026-03-10-some-change.md" {
		t.Fatalf("want preserved name, got %q", filename)
	}

	events := journalEvents(t, specDir)
	count := 0
	for _, ev := range events {
		if ev.Event == "registered" && ev.EID == eid {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want exactly 1 registered line for %q, got %d", eid, count)
	}

	if _, err := os.Stat(filepath.Join(specDir, "proposals", filename)); err != nil {
		t.Fatalf("copy did not land: %v", err)
	}
}

// --- E4: Proposal containing both project and change markers ---

func TestREQ_7885ad439bb9_E4_BothProjectAndChangeMarkers(t *testing.T) {
	specDir := newSpecDir(t)
	content := "# Combo\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n\n## Proposed change\n\nx\n"
	input := writeInput(t, specDir, "combo.md", content)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("want project-type validation to pass (all project sections present), got: %v", err)
	}
	if filename == "" {
		t.Fatal("want a filename")
	}
}

// --- E5: Very large proposal file ---

func TestREQ_7885ad439bb9_E5_VeryLargeProposalFile(t *testing.T) {
	specDir := newSpecDir(t)
	padding := strings.Repeat("x", 10*1024*1024)
	content := "# Project Proposal: Big\n\n## Vision\n\n" + padding + "\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	input := writeInput(t, specDir, "big.md", content)

	filename, err := Register(input, specDir, fixtureHead)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	destContent, err := os.ReadFile(filepath.Join(specDir, "proposals", filename))
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if len(destContent) != len(content) {
		t.Fatalf("copy size mismatch: want %d, got %d", len(content), len(destContent))
	}
}

// --- E6: Proposal with extra sections beyond required ones ---

func TestREQ_7885ad439bb9_E6_ExtraSectionsIgnored(t *testing.T) {
	specDir := newSpecDir(t)
	content := "# Project Proposal: Extra\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n\n## Timeline\n\nx\n\n## Open questions\n\nx\n\n## Appendix\n\nx\n"
	input := writeInput(t, specDir, "extra.md", content)

	if _, err := Register(input, specDir, fixtureHead); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// --- FR1 unit tests: internal helpers ---

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
	if err := validateSections(content, "project"); err != nil {
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
	for _, want := range []string{"modules", "key requirements", "design decisions"} {
		if !strings.Contains(msg, want) {
			t.Errorf("want error about %s, got: %v", want, err)
		}
	}
}

func TestFR1_ValidateSections_ChangeComplete(t *testing.T) {
	content := "# C\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	if err := validateSections(content, "change"); err != nil {
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
	if headings[0] != "First" || headings[1] != "Second" {
		t.Errorf("want [First Second], got %v", headings)
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
