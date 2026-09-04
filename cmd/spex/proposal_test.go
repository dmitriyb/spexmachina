package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

// --- spex register ---

func TestREQ30_S1_RegisterValidProposal(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	content := "# Change Proposal: New Change\n\n## Context\n\nSome context.\n\n## Proposed change\n\nSome change.\n\n## Impact expectation\n\nSome impact.\n"
	inputDir := filepath.Join(tmp, "input")
	os.MkdirAll(inputDir, 0755)
	inputFile := filepath.Join(inputDir, "new-change.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	var out strings.Builder
	root := buildTestCmd(specDir)
	registerCmd := findSubcommand(root, "register")
	registerCmd.SetOut(&out)

	root.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if !strings.Contains(out.String(), "registered: "+filepath.Join(specDir, "proposals")+"/") {
		t.Errorf("want registered message, got %q", out.String())
	}

	// Verify file was created in proposals dir.
	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	var slug string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-new-change.md") {
			slug = strings.TrimSuffix(e.Name(), ".md")
		}
	}
	if slug == "" {
		t.Fatal("registered proposal file not found in spec/proposals/")
	}

	// The flag's head feeds the registered event's eid — spex itself never
	// calls git.
	events, err := mapping.NewMappingStore(filepath.Join(projectStateDir(specDir), lifecycle.JournalFileName)).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	wantEID := "cafe1234:" + slug
	found := false
	for _, ev := range events {
		if ev.Event == "registered" && ev.EID == wantEID {
			found = true
		}
	}
	if !found {
		t.Errorf("want registered event with eid %q, got events: %+v", wantEID, events)
	}
}

func TestREQ30_S2_RegisterWithExplicitSpecDir(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "myspec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	content := "# Change Proposal: Explicit Dir\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "explicit.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) == 0 {
		t.Error("no file created in custom spec dir proposals/")
	}

	// The journal follows the lifecycle pre-flight's resolved location,
	// itself keyed off --spec-dir, exactly as the proposals directory
	// follows --spec-dir.
	events, err := mapping.NewMappingStore(filepath.Join(projectStateDir(specDir), lifecycle.JournalFileName)).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	found := false
	for _, ev := range events {
		if ev.Event == "registered" && ev.GitHead == "cafe1234" {
			found = true
		}
	}
	if !found {
		t.Errorf("want registered event in the resolved journal, got events: %+v", events)
	}
}

func TestREQ30_S3_RegisterValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	content := "# Bad\n\n## Background\n\n## Plan\n"
	inputFile := filepath.Join(tmp, "invalid-proposal.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
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

// TestREQ30_S4_RegisterMissingArgument covers the barest invocation: no
// proposal path and no --git-head. The argument count is checked before any
// flag is, so the missing-path message wins, not the flag's own (see E7).
func TestREQ30_S4_RegisterMissingArgument(t *testing.T) {
	root := buildTestCmd("/tmp/spec")
	root.SetArgs([]string{"register"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for missing argument, got nil")
	}
	if strings.Contains(err.Error(), "git-head") {
		t.Errorf("want the missing-path message, not the missing-flag message, got: %v", err)
	}
}

// TestREQ30_E7_RegisterMissingGitHead covers the flag omitted entirely (path
// present). The pre-flight refuses the run before Registrar is reached, so
// neither the file nor the journal event is written.
func TestREQ30_E7_RegisterMissingGitHead(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	content := "# Change Proposal: New Change\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "new-change.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for missing --git-head, got nil")
	}
	if !strings.Contains(err.Error(), "git-head") {
		t.Errorf("want the missing flag named in the error, got: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
	if len(entries) != 0 {
		t.Error("no file should be created when --git-head is missing")
	}
	events, _ := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl")).Parse()
	if len(events) != 0 {
		t.Errorf("no journal event should be appended when --git-head is missing, got: %+v", events)
	}
}

// TestREQ30_E8_RegisterMalformedGitHead covers a --git-head value that fails
// the ^[0-9a-f]{7,40}$ pre-flight: non-hex characters, empty, and too short.
func TestREQ30_E8_RegisterMalformedGitHead(t *testing.T) {
	cases := []struct {
		name string
		head string
	}{
		{"non-hex", "zznothex"},
		{"empty", ""},
		{"too short", "cafe12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			specDir := filepath.Join(tmp, "spec")
			os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

			content := "# Change Proposal: New Change\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
			inputFile := filepath.Join(tmp, "new-change.md")
			os.WriteFile(inputFile, []byte(content), 0644)

			root := buildTestCmd(specDir)
			root.SetArgs([]string{"register", inputFile, "--git-head", tc.head})
			err := root.Execute()
			if err == nil {
				t.Fatalf("want error for malformed --git-head %q, got nil", tc.head)
			}
			if !strings.Contains(err.Error(), "git-head") {
				t.Errorf("want the pre-flight message naming --git-head, got: %v", err)
			}

			entries, _ := os.ReadDir(filepath.Join(specDir, "proposals"))
			if len(entries) != 0 {
				t.Error("no file should be created for a malformed --git-head")
			}
			events, _ := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl")).Parse()
			if len(events) != 0 {
				t.Errorf("no journal event should be appended for a malformed --git-head, got: %+v", events)
			}
		})
	}
}

func TestREQ30_S5_RegisterNonexistentFile(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", filepath.Join(tmp, "input", "ghost.md"), "--git-head", "cafe1234"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for nonexistent file, got nil")
	}
}

func TestREQ30_E6_RegisterIdempotency(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	content := "# Change Proposal: Repeat\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	inputFile := filepath.Join(tmp, "repeat.md")
	os.WriteFile(inputFile, []byte(content), 0644)

	// First register.
	root1 := buildTestCmd(specDir)
	root1.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	if err := root1.Execute(); err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Second register should fail.
	root2 := buildTestCmd(specDir)
	root2.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
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
		t.Errorf("want task id in output, got:\n%s", out)
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
			Tasks    []struct {
				ID     string `json:"id"`
				Action string `json:"action"`
				Status string `json:"status"`
			} `json:"tasks"`
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
	if len(p.Tasks) != 1 || p.Tasks[0].ID != "spexmachina-xyz" || p.Tasks[0].Action != "closed" {
		t.Errorf("unexpected task entry: %+v", p.Tasks)
	}
}

func TestREQ30_S8_LogEmptyProposals(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}

	// Human-readable: empty task array → empty stdout.
	out, err := runLogWith(t, specDir, `[]`)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if out != "" {
		t.Errorf("want empty stdout, got %q", out)
	}

	// JSON: empty task array → {"proposals":[]} envelope.
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
	if !strings.Contains(err.Error(), "no task data on stdin") {
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
	if !strings.Contains(err.Error(), "spex log: parse task JSON:") {
		t.Errorf("want documented parse-failure error, got: %v", err)
	}
}

func TestREQ30_S16_RegisterThenLogRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	inputFile := filepath.Join(tmp, "new.md")
	if err := os.WriteFile(inputFile, []byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 1: register.
	root1 := buildTestCmd(specDir)
	root1.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	if err := root1.Execute(); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Step 2: log --json with empty task list — registered proposal should
	// not appear (HistoryViewer keys off task labels, not directory listing).
	out, err := runLogWith(t, specDir, `[]`, "--json")
	if err != nil {
		t.Fatalf("log --json: %v", err)
	}
	var envelope struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Tasks    []any  `json:"tasks"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if len(envelope.Proposals) != 0 {
		t.Errorf("empty task input must produce empty proposals array, got %d", len(envelope.Proposals))
	}
}

// TestREQ30_LogProposalFilter verifies the --proposal flag scopes the output
// to a single proposal stem, dropping tasks tagged for any other proposal.
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
		t.Errorf("want filtered task, got:\n%s", out)
	}
	if strings.Contains(out, "spexmachina-abc") {
		t.Errorf("filtered task leaked into output:\n%s", out)
	}
}

// --- end-to-end pipeline ---

// TestREQ30_S15_TemplateOutputPipeable covers stdout redirected to a file:
// the bytes landing there must match the template verbatim, with nothing
// else mixed in, and any informational output must go to stderr instead.
func TestREQ30_S15_TemplateOutputPipeable(t *testing.T) {
	binPath := buildSpexBinary(t)
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "my-proposal.md")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	cmd := exec.Command(binPath, "template", "project")
	cmd.Stdout = outFile
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	outFile.Close()
	if runErr != nil {
		t.Fatalf("template project: %v\nstderr: %s", runErr, stderr.String())
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	var want bytes.Buffer
	if err := proposal.Template("project", &want); err != nil {
		t.Fatal(err)
	}
	if string(got) != want.String() {
		t.Errorf("redirected stdout does not match the template verbatim (extra output mixed in?)\ngot:\n%s\nwant:\n%s", got, want.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("want no stderr output, got %q", stderr.String())
	}
}

// TestREQ30_S17_RegisterComposability checks the composability contract
// itself: a file in, a file out, exit 0, no interactive prompt (so no
// process hang when stdin is unattached), and no side effects outside the
// spec directory.
func TestREQ30_S17_RegisterComposability(t *testing.T) {
	binPath := buildSpexBinary(t)
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	inputFile := filepath.Join(tmp, "new-change.md")
	if err := os.WriteFile(inputFile, []byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath, "register", "--spec-dir", specDir, inputFile, "--git-head", "cafe1234")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Deliberately leave cmd.Stdin nil (no pipe attached) — a prompt reading
	// from it would block, which the timeout below turns into a failure
	// instead of a hung test.
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("register: %v\nstderr: %s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("register blocked without stdin attached — not suitable for shell scripts or CI pipelines")
	}

	entries, err := os.ReadDir(filepath.Join(specDir, "proposals"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("want file written to spec/proposals/, entries=%v err=%v", entries, err)
	}

	rootEntries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	// "spec" is the spec directory itself; ".spex" is the lifecycle state
	// dir seedProjectState pre-seeded as a sibling before register ran; the
	// input file is the one thing this run reads. Nothing else may appear.
	allowed := map[string]bool{"spec": true, ".spex": true, filepath.Base(inputFile): true}
	for _, e := range rootEntries {
		if !allowed[e.Name()] {
			t.Errorf("unexpected side effect outside the spec directory: %s", e.Name())
		}
	}
}

// --- edge cases ---

// TestREQ30_E1_RegisterProposalsSymlink covers spec/proposals being a
// symlink to another directory: the file must land through the symlink, at
// the target.
func TestREQ30_E1_RegisterProposalsSymlink(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "shared-proposals")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(specDir, "proposals")); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	inputFile := filepath.Join(tmp, "new-change.md")
	if err := os.WriteFile(inputFile, []byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	if err := root.Execute(); err != nil {
		t.Fatalf("register: %v", err)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("want the file written through the symlink, at its target")
	}
}

// TestREQ30_E2_RegisterReadOnlyProposalsDir covers a spec/proposals/ with no
// write permission: the copy must fail with a permission error, the
// original proposal must be left untouched, and no file should land.
func TestREQ30_E2_RegisterReadOnlyProposalsDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits")
	}

	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	if err := os.MkdirAll(proposalsDir, 0755); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())
	if err := os.Chmod(proposalsDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(proposalsDir, 0755) })

	content := changeProposalContent
	inputFile := filepath.Join(tmp, "new-change.md")
	if err := os.WriteFile(inputFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	root := buildTestCmd(specDir)
	root.SetArgs([]string{"register", inputFile, "--git-head", "cafe1234"})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error for read-only proposals dir, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("want a permission-denied error, got: %v", err)
	}

	got, err := os.ReadFile(inputFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Error("original proposal file must not be modified")
	}
}

// TestREQ30_E3_LogNoANSIWhenPiped covers spex log with stdout piped (never a
// terminal in a subprocess pipe): the human-readable rendering must carry no
// ANSI escape sequences.
func TestREQ30_E3_LogNoANSIWhenPiped(t *testing.T) {
	binPath := buildSpexBinary(t)
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

	cmd := exec.Command(binPath, "log", "--spec-dir", specDir)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if strings.ContainsRune(string(out), 0x1b) {
		t.Errorf("want no ANSI escape sequences in piped output, got:\n%q", out)
	}
	if !strings.Contains(string(out), "2026-02-23-spex-machina.md") {
		t.Errorf("want plain-text history still parseable, got:\n%s", out)
	}
}

// TestREQ30_E4_DefaultSpecDirSharedAcrossSubcommands covers all three
// subcommands defaulting to ./spec relative to the working directory when
// --spec-dir is not passed, and staying consistent with each other: a
// proposal registered by one is visible to another.
func TestREQ30_E4_DefaultSpecDirSharedAcrossSubcommands(t *testing.T) {
	binPath, err := filepath.Abs(buildSpexBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	inputFile := filepath.Join(tmp, "new-change.md")
	if err := os.WriteFile(inputFile, []byte(changeProposalContent), 0644); err != nil {
		t.Fatal(err)
	}

	registerCmd := exec.Command(binPath, "register", inputFile, "--git-head", "cafe1234")
	registerCmd.Dir = tmp
	if out, err := registerCmd.CombinedOutput(); err != nil {
		t.Fatalf("register (default spec dir): %v\n%s", err, out)
	}

	entries, err := os.ReadDir(filepath.Join(specDir, "proposals"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("want file registered under the default ./spec, entries=%v err=%v", entries, err)
	}
	stem := strings.TrimSuffix(entries[0].Name(), ".md")

	stdin := `[{"id":"spexmachina-abc","status":"open","title":"x","labels":["spec_proposal:` + stem + `"]}]`
	logCmd := exec.Command(binPath, "log", "--json")
	logCmd.Dir = tmp
	logCmd.Stdin = strings.NewReader(stdin)
	out, err := logCmd.Output()
	if err != nil {
		t.Fatalf("log (default spec dir): %v", err)
	}
	if !strings.Contains(string(out), stem+".md") {
		t.Errorf("want the proposal registered under the default spec dir visible to log, got:\n%s", out)
	}

	templateCmd := exec.Command(binPath, "template", "project")
	templateCmd.Dir = tmp
	if _, err := templateCmd.Output(); err != nil {
		t.Fatalf("template (default spec dir): %v", err)
	}
}

// TestREQ30_E5_ConcurrentRegisterDistinctProposals covers two register calls
// for distinct proposals racing each other: both must succeed and both
// files must land, since their target names never collide.
func TestREQ30_E5_ConcurrentRegisterDistinctProposals(t *testing.T) {
	binPath := buildSpexBinary(t)
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	fileA := filepath.Join(tmp, "a.md")
	fileB := filepath.Join(tmp, "b.md")
	if err := os.WriteFile(fileA, []byte("# Change Proposal: A\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("# Change Proposal: B\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	outs := make([]string, 2)
	for i, f := range []string{fileA, fileB} {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			cmd := exec.Command(binPath, "register", "--spec-dir", specDir, f, "--git-head", "cafe1234")
			out, err := cmd.CombinedOutput()
			outs[i] = string(out)
			errs[i] = err
		}(i, f)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("register %d: %v\n%s", i, err, outs[i])
		}
	}

	entries, err := os.ReadDir(filepath.Join(specDir, "proposals"))
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !hasSuffix(names, "-a.md") || !hasSuffix(names, "-b.md") {
		t.Errorf("want both distinct proposals registered without a race, got: %v", names)
	}
}

// hasSuffix reports whether any name in names ends with suffix.
func hasSuffix(names map[string]bool, suffix string) bool {
	for n := range names {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
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
