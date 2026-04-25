package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TODO(bead:spexmachina-0lk.21): the stubBeadLister + ShowHistory tests below
// were removed because spexmachina-0lk.20 retired BeadLister/CLIBeadLister and
// reshaped HistoryViewer to consume a pre-parsed []BeadRecord. The
// ProposalCommands bead owns rewiring `spex log` to read bead JSON from stdin
// and the corresponding integration tests.

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

// TODO(bead:spexmachina-0lk.21): TestREQ30_S6_LogHumanReadable,
// TestREQ30_S7_LogJSON, TestREQ30_S8_LogEmptyProposals, and
// TestREQ30_S16_RegisterThenLogRoundTrip were removed here.
// They asserted on proposal.ShowHistory(ctx, specDir, lister, ...) and
// proposal.BeadRecord.Metadata, both of which spexmachina-0lk.20 retired.
// Re-introduce equivalent tests when ProposalCommands wires `spex log` to read
// bead JSON from stdin and parse it into the new BeadRecord shape (Labels-based
// grouping, HistoryViewer struct).

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
