package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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

const projProposalContent = "# Project Proposal: Spex Machina\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"

const changeProposalContent = "# Change Proposal: Decouple\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"

// runLogWith feeds stdin to the log subcommand and returns stdout.
func runLogWith(t *testing.T, specDir, stdin string, args ...string) (string, error) {
	t.Helper()
	root := buildTestCmd(specDir)
	logCmd := findSubcommand(root, "log")
	var out bytes.Buffer
	logCmd.SetOut(&out)
	logCmd.SetErr(&bytes.Buffer{})
	logCmd.SetIn(strings.NewReader(stdin))

	root.SetArgs(append([]string{"log"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestREQ30_S6_LogHumanReadable(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "proposals", "2026-02-23-spex-machina.md"),
		[]byte(projProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdin := `[{"id":"spexmachina-abc","status":"open","title":"schema: ProjectSchema","labels":["spec_proposal:2026-02-23-spex-machina"]}]`

	out, err := runLogWith(t, specDir, stdin)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(out, "2026-02-23-spex-machina.md") {
		t.Errorf("want proposal filename in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(project proposal)") {
		t.Errorf("want project-proposal type label, got:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") {
		t.Errorf("want bead id in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Created:") {
		t.Errorf("want Created action label, got:\n%s", out)
	}
}

func TestREQ30_S7_LogJSON(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "proposals", "2026-04-18-decouple.md"),
		[]byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdin := `{"issues":[{"id":"spexmachina-xyz","status":"closed","title":"emit: ChangesetBuilder","labels":["spec_proposal:2026-04-18-decouple"]}]}`

	out, err := runLogWith(t, specDir, stdin, "--json")
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Beads    []struct {
				ID     string `json:"id"`
				Action string `json:"action"`
				Status string `json:"status"`
			} `json:"beads"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("not valid JSON: %v\nraw: %s", err, out)
	}
	if len(payload.Proposals) != 1 {
		t.Fatalf("want 1 proposal entry, got %d", len(payload.Proposals))
	}
	p := payload.Proposals[0]
	if p.Filename != "2026-04-18-decouple.md" {
		t.Errorf("want filename, got %q", p.Filename)
	}
	if len(p.Beads) != 1 || p.Beads[0].ID != "spexmachina-xyz" || p.Beads[0].Action != "closed" {
		t.Errorf("unexpected bead entry: %+v", p.Beads)
	}
}

func TestREQ30_S8_LogEmptyProposals(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}

	// Human-readable: empty bead array → empty stdout.
	out, err := runLogWith(t, specDir, `[]`)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if out != "" {
		t.Errorf("want empty stdout, got %q", out)
	}

	// JSON: empty bead array → {"proposals":[]} envelope.
	out, err = runLogWith(t, specDir, `[]`, "--json")
	if err != nil {
		t.Fatalf("log --json: %v", err)
	}
	var envelope struct {
		Proposals []any `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if len(envelope.Proposals) != 0 {
		t.Errorf("want empty proposals array, got %d entries", len(envelope.Proposals))
	}
}

func TestREQ30_S9_LogExplicitSpecDir(t *testing.T) {
	tmp := t.TempDir()
	otherSpec := filepath.Join(tmp, "otherspec")
	if err := os.MkdirAll(filepath.Join(otherSpec, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherSpec, "proposals", "2026-04-18-decouple.md"),
		[]byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdin := `[{"id":"spexmachina-abc","status":"open","title":"x","labels":["spec_proposal:2026-04-18-decouple"]}]`

	// buildTestCmd already wires --spec-dir; we just point it at otherSpec.
	out, err := runLogWith(t, otherSpec, stdin)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(out, "2026-04-18-decouple.md") {
		t.Errorf("want history from explicit spec dir, got:\n%s", out)
	}
}

// TestREQ30_S10_LogEmptyStdin replaces the original S10 "br not on PATH"
// scenario: the new architecture (arch_proposal_commands.md) requires that
// spex log never invoke a tracker subprocess. Empty stdin must therefore exit
// non-zero with the documented message.
func TestREQ30_S10_LogEmptyStdin(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := runLogWith(t, specDir, "")
	if err == nil {
		t.Fatal("want error on empty stdin, got nil")
	}
	if !strings.Contains(err.Error(), "no bead data on stdin") {
		t.Errorf("want documented empty-stdin error, got: %v", err)
	}
}

func TestREQ30_S10b_LogMalformedStdin(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := runLogWith(t, specDir, "{ not json")
	if err == nil {
		t.Fatal("want error on malformed JSON, got nil")
	}
}

func TestREQ30_S16_RegisterThenLogRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}

	inputFile := filepath.Join(tmp, "new.md")
	if err := os.WriteFile(inputFile, []byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 1: register.
	root1 := buildTestCmd(specDir)
	root1.SetArgs([]string{"register", inputFile})
	if err := root1.Execute(); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Step 2: log --json with empty bead list — registered proposal should
	// not appear (HistoryViewer keys off bead labels, not directory listing).
	out, err := runLogWith(t, specDir, `[]`, "--json")
	if err != nil {
		t.Fatalf("log --json: %v", err)
	}
	var envelope struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Beads    []any  `json:"beads"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if len(envelope.Proposals) != 0 {
		t.Errorf("empty bead input must produce empty proposals array, got %d", len(envelope.Proposals))
	}
}

// TestREQ30_LogProposalFilter verifies the --proposal flag scopes the output
// to a single proposal stem, dropping beads tagged for any other proposal.
func TestREQ30_LogProposalFilter(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "proposals", "2026-02-23-spex-machina.md"),
		[]byte(projProposalContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "proposals", "2026-04-18-decouple.md"),
		[]byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdin := `[
{"id":"spexmachina-abc","status":"open","title":"a","labels":["spec_proposal:2026-02-23-spex-machina"]},
{"id":"spexmachina-xyz","status":"open","title":"b","labels":["spec_proposal:2026-04-18-decouple"]}
]`

	out, err := runLogWith(t, specDir, stdin, "--proposal", "2026-04-18-decouple")
	if err != nil {
		t.Fatalf("log --proposal: %v", err)
	}
	if !strings.Contains(out, "spexmachina-xyz") {
		t.Errorf("want filtered bead, got:\n%s", out)
	}
	if strings.Contains(out, "spexmachina-abc") {
		t.Errorf("filtered bead leaked into output:\n%s", out)
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
