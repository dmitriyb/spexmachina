package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

// stubBeadLister is a test double for proposal.BeadLister.
type stubBeadLister struct {
	beads []proposal.BeadRecord
}

func (s *stubBeadLister) ListBeads(_ context.Context) ([]proposal.BeadRecord, error) {
	return s.beads, nil
}

// --- spex register ---

func TestREQ30_S1_RegisterValidProposal(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: New Change\n\n## Context\n\nSome context.\n\n## Proposed change\n\nSome change.\n\n## Impact expectation\n\nSome impact.\n"
	inputDir := filepath.Join(tmp, "input")
	os.MkdirAll(inputDir, 0755)
	inputFile := filepath.Join(inputDir, "new-change.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	var out strings.Builder
	root := buildTestCmd(specDir)
	registerCmd := findSubcommand(root, "register")
	registerCmd.SetOut(&out)

	root.SetArgs([]string{"register", inputFile})
	err := root.Execute()
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if !strings.Contains(out.String(), "registered: spec/proposals/") {
		t.Errorf("want registered message, got %q", out.String())
	}

	// Verify file was created in proposals dir.
	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-new-change.md") {
			found = true
		}
	}
	if !found {
		t.Error("registered proposal file not found in spec/proposals/")
	}
}

func TestREQ30_S2_RegisterWithExplicitSpecDir(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "myspec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: Explicit Dir\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "explicit.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile})
	err := root.Execute()
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) == 0 {
		t.Error("no file created in custom spec dir proposals/")
	}
}

func TestREQ30_S3_RegisterValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Bad\n\n## Background\n\n## Plan\n"
	inputFile := filepath.Join(tmp, "invalid-proposal.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for invalid proposal, got nil")
	}
	if !strings.Contains(err.Error(), "cannot detect type from headings") {
		t.Errorf("want heading detection error, got: %v", err)
	}

	// No file created.
	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) > 0 {
		t.Error("file should not be created on validation failure")
	}
}

func TestREQ30_S4_RegisterMissingArgument(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"register"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for missing argument, got nil")
	}
}

func TestREQ30_S5_RegisterNonexistentFile(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", filepath.Join(tmp, "input", "ghost.md")})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for nonexistent file, got nil")
	}
}

func TestREQ30_E6_RegisterIdempotency(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: Repeat\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "repeat.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	// First register.
	root1 := buildTestCmd(specDir)
	root1.SetArgs([]string{"register", inputFile})
	if err := root1.Execute(); err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Second register should fail.
	root2 := buildTestCmd(specDir)
	root2.SetArgs([]string{"register", inputFile})
	err := root2.Execute()
	if err == nil {
		t.Fatal("want error on second register, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("want 'already registered' error, got: %v", err)
	}
}

// --- spex template ---

func TestREQ30_S11_TemplateProject(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"template", "project"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("template project: %v", err)
	}
}

func TestREQ30_S12_TemplateChange(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"template", "change"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("template change: %v", err)
	}
}

func TestREQ30_S13_TemplateInvalidType(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"template", "unknown"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for unknown template type, got nil")
	}
	if !strings.Contains(err.Error(), `unknown template type: "unknown"`) {
		t.Errorf("want unknown type error, got: %v", err)
	}
}

func TestREQ30_S14_TemplateMissingArgument(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"template"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for missing argument, got nil")
	}
}

// --- spex log ---

func TestREQ30_S6_LogHumanReadable(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	os.MkdirAll(proposalsDir, 0755)

	projContent := "# Project Proposal: Test\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	os.WriteFile(filepath.Join(proposalsDir, "2026-02-23-test.md"), []byte(projContent), 0644)

	lister := &stubBeadLister{
		beads: []proposal.BeadRecord{
			{
				ID:       "spexmachina-abc",
				Title:    "schema: ProjectSchema",
				Metadata: map[string]string{"spec_proposal": "2026-02-23-test"},
			},
		},
	}

	var out strings.Builder
	err := proposal.ShowHistory(t.Context(), specDir, lister, &out, false)
	if err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	result := out.String()
	if !strings.Contains(result, "2026-02-23-test.md (project proposal)") {
		t.Errorf("want proposal header in output, got:\n%s", result)
	}
	if !strings.Contains(result, "spexmachina-abc") {
		t.Errorf("want bead ID in output, got:\n%s", result)
	}
	if !strings.Contains(result, "schema: ProjectSchema") {
		t.Errorf("want bead details in output, got:\n%s", result)
	}
}

func TestREQ30_S7_LogJSON(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	os.MkdirAll(proposalsDir, 0755)

	projContent := "# Project Proposal: Test\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	os.WriteFile(filepath.Join(proposalsDir, "2026-02-23-test.md"), []byte(projContent), 0644)

	lister := &stubBeadLister{
		beads: []proposal.BeadRecord{
			{
				ID:       "spexmachina-abc",
				Title:    "schema: ProjectSchema",
				Metadata: map[string]string{"spec_proposal": "2026-02-23-test"},
			},
		},
	}

	var out strings.Builder
	err := proposal.ShowHistory(t.Context(), specDir, lister, &out, true)
	if err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}

	var result []proposal.ProposalEntry
	if err := json.Unmarshal([]byte(out.String()), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out.String())
	}
	if len(result) != 1 {
		t.Fatalf("want 1 entry, got %d", len(result))
	}
	if result[0].Proposal != "2026-02-23-test.md" {
		t.Errorf("want proposal name, got %q", result[0].Proposal)
	}
	if result[0].Type != "project" {
		t.Errorf("want type project, got %q", result[0].Type)
	}
	if result[0].Date != "2026-02-23" {
		t.Errorf("want date, got %q", result[0].Date)
	}
	if len(result[0].Beads) != 1 {
		t.Fatalf("want 1 bead, got %d", len(result[0].Beads))
	}
}

func TestREQ30_S8_LogEmptyProposals(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	lister := &stubBeadLister{beads: []proposal.BeadRecord{}}

	// Human-readable.
	var out strings.Builder
	err := proposal.ShowHistory(t.Context(), specDir, lister, &out, false)
	if err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}
	if out.String() != "" {
		t.Errorf("want empty output, got %q", out.String())
	}

	// JSON.
	out.Reset()
	err = proposal.ShowHistory(t.Context(), specDir, lister, &out, true)
	if err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("want '[]', got %q", out.String())
	}
}

func TestREQ30_S16_RegisterThenLogRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	// Register a change proposal.
	content := "# Change Proposal: Round Trip\n\n## Context\n\nTesting round trip.\n\n## Proposed change\n\nSome change.\n\n## Impact expectation\n\nSome impact.\n"
	inputFile := filepath.Join(tmp, "round-trip.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	filename, err := proposal.Register(inputFile, specDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Log (JSON mode, no beads tagged yet).
	lister := &stubBeadLister{beads: []proposal.BeadRecord{}}
	var out strings.Builder
	err = proposal.ShowHistory(t.Context(), specDir, lister, &out, true)
	if err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	var result []proposal.ProposalEntry
	if err := json.Unmarshal([]byte(out.String()), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	found := false
	for _, entry := range result {
		if entry.Proposal == filename {
			found = true
			if len(entry.Beads) != 0 {
				t.Errorf("want empty beads array, got %d", len(entry.Beads))
			}
		}
	}
	if !found {
		t.Errorf("registered proposal %q not found in log output", filename)
	}
}

// --- helpers ---

// buildTestCmd creates a root command with all proposal subcommands and a
// preset --spec-dir flag, suitable for testing.
func buildTestCmd(specDir string) *cobra.Command {
	root := &cobra.Command{
		Use:           "spex",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().String("spec-dir", specDir, "spec directory")
	root.AddCommand(
		newRegisterCmd(),
		newLogCmd(),
		newTemplateCmd(),
	)
	return root
}

// findSubcommand returns the named subcommand from root.
func findSubcommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
