package proposal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// writeProposal writes a proposal markdown file under specDir/proposals.
func writeProposal(t *testing.T, specDir, name, content string) {
	t.Helper()
	dir := filepath.Join(specDir, "proposals")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir proposals: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

const projContent = "# Project Proposal: Spex Machina\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"

const changeContent = "# Change Proposal: Decouple spex from br\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"

func TestHistoryViewer_GroupsTasksBySpecProposalLabel(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-spex-machina.md", projContent)
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-def", Status: "in_progress", Title: "validator: SchemaChecker",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-ghi", Status: "closed", Title: "merkle: DiffCommand",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
		// Task without spec_proposal label is skipped.
		{ID: "spexmachina-pqr", Status: "open", Title: "unrelated"},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-02-23-spex-machina.md") {
		t.Errorf("missing first proposal header:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-18-decouple.md") {
		t.Errorf("missing second proposal header:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") || !strings.Contains(out, "spexmachina-def") {
		t.Errorf("missing tasks under first proposal:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-ghi") {
		t.Errorf("missing task under second proposal:\n%s", out)
	}
	if strings.Contains(out, "spexmachina-pqr") {
		t.Errorf("unrelated task leaked into output:\n%s", out)
	}
}

func TestHistoryViewer_RendersProposalTypeAndStatus(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-spex-machina.md", projContent)
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-old", Status: "closed", Title: "emit: ChangesetBuilder",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "(project proposal)") {
		t.Errorf("want project-proposal type label, got:\n%s", out)
	}
	if !strings.Contains(out, "(change proposal)") {
		t.Errorf("want change-proposal type label, got:\n%s", out)
	}
	if !strings.Contains(out, "Created: spexmachina-abc") {
		t.Errorf("want Created action for open task, got:\n%s", out)
	}
	if !strings.Contains(out, "Closed:") || !strings.Contains(out, "spexmachina-old") {
		t.Errorf("want Closed action for closed task, got:\n%s", out)
	}
	if !strings.Contains(out, "(open)") || !strings.Contains(out, "(closed)") {
		t.Errorf("want task status in parentheses, got:\n%s", out)
	}
}

func TestHistoryViewer_MissingProposalFile(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	// Create the proposals directory but no files.
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-99-99-ghost"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "proposal file missing") {
		t.Errorf("want 'proposal file missing' label, got:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") {
		t.Errorf("want task listed under missing-proposal entry, got:\n%s", out)
	}
}

// TestHistoryViewer_ProposalWithNoLinkedTasks covers S3: a proposal file
// exists on disk but no supplied task labels it, so it must not appear in
// the output — the listing is driven by task labels, never a directory scan.
func TestHistoryViewer_ProposalWithNoLinkedTasks(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-03-08-future-idea.md", changeContent)
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "future-idea") {
		t.Errorf("unlabelled proposal must not appear:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-18-decouple.md") {
		t.Errorf("want labelled proposal present:\n%s", out)
	}
}

func TestHistoryViewer_JSONMode(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "emit: ChangesetBuilder",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
		{ID: "spexmachina-old", Status: "closed", Title: "emit: Resolver",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Title    string `json:"title"`
			Tasks    []struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Action  string `json:"action"`
				Summary string `json:"summary"`
			} `json:"tasks"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json unmarshal: %v\nraw: %s", err, buf.String())
	}
	if len(payload.Proposals) != 1 {
		t.Fatalf("want 1 proposal entry, got %d", len(payload.Proposals))
	}
	p := payload.Proposals[0]
	if p.Filename != "2026-04-18-decouple.md" {
		t.Errorf("want filename, got %q", p.Filename)
	}
	if p.Title != "Change Proposal: Decouple spex from br" {
		t.Errorf("want H1 title, got %q", p.Title)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(p.Tasks))
	}
	if p.Tasks[0].ID != "spexmachina-abc" || p.Tasks[0].Action != "created" || p.Tasks[0].Status != "open" {
		t.Errorf("first task: %+v", p.Tasks[0])
	}
	if p.Tasks[0].Summary != "emit: ChangesetBuilder" {
		t.Errorf("want summary, got %q", p.Tasks[0].Summary)
	}
	if p.Tasks[1].Action != "closed" {
		t.Errorf("want closed action for closed task, got %q", p.Tasks[1].Action)
	}

	// The envelope speaks the corpus vocabulary: "tasks", never "beads".
	if strings.Contains(buf.String(), `"beads"`) {
		t.Errorf("retired key 'beads' must not appear in envelope:\n%s", buf.String())
	}
}

func TestHistoryViewer_NoProposalsNoTasks(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")

	// Default (text) mode produces empty output.
	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(nil); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("want empty output, got %q", buf.String())
	}

	// JSON mode produces an empty proposals array.
	buf.Reset()
	hv.JSON = true
	if err := hv.ShowHistory(nil); err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}
	var envelope struct {
		Proposals []any `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("json unmarshal: %v\nraw: %s", err, buf.String())
	}
	if len(envelope.Proposals) != 0 {
		t.Errorf("want empty proposals array, got %d entries", len(envelope.Proposals))
	}
}

func TestHistoryViewer_MultipleSpecProposalLabelsUsesFirst(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-first.md", projContent)
	writeProposal(t, specDir, "2026-03-01-second.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{
				"spec_proposal:2026-02-23-first",
				"spec_proposal:2026-03-01-second",
			}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Tasks    []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	var firstHasTask, secondHasTask bool
	for _, p := range payload.Proposals {
		for _, tk := range p.Tasks {
			if tk.ID != "spexmachina-abc" {
				continue
			}
			if p.Filename == "2026-02-23-first.md" {
				firstHasTask = true
			}
			if p.Filename == "2026-03-01-second.md" {
				secondHasTask = true
			}
		}
	}
	if !firstHasTask {
		t.Errorf("task should be grouped under first label")
	}
	if secondHasTask {
		t.Errorf("task should NOT appear under second label")
	}
}

func TestHistoryViewer_NoSubprocessUsage(t *testing.T) {
	// HistoryViewer accepts parsed []TaskRecord and must not invoke any
	// external subprocess. This test does not assert on output; it merely
	// constructs a viewer with an empty PATH — if any exec.Command call
	// existed, it would fail. With a pure pipeline, this succeeds.
	t.Setenv("PATH", "")

	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}
}

func TestHistoryViewer_ProposalsSortedByFilename(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-03-01-b.md", changeContent)
	writeProposal(t, specDir, "2026-02-23-a.md", projContent)
	writeProposal(t, specDir, "2026-04-12-c.md", changeContent)

	tasks := []TaskRecord{
		{ID: "id1", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-02-23-a"}},
		{ID: "id2", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-03-01-b"}},
		{ID: "id3", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-12-c"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	idxA := strings.Index(out, "2026-02-23-a.md")
	idxB := strings.Index(out, "2026-03-01-b.md")
	idxC := strings.Index(out, "2026-04-12-c.md")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing proposal entries:\n%s", out)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("proposals not sorted by filename:\n%s", out)
	}
}

// TestHistoryViewer_NonMarkdownFilesIgnored covers E1: non-.md files under
// spec/proposals/ never surface. HistoryViewer never scans the directory —
// it only resolves the specific stems named by task labels — so a stray
// notes.txt or diagram.png simply has no path that could ever name it.
func TestHistoryViewer_NonMarkdownFilesIgnored(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-spex-machina.md", projContent)
	writeProposal(t, specDir, "notes.txt", "not a proposal")
	writeProposal(t, specDir, "diagram.png", "\x89PNG")

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(payload.Proposals) != 1 {
		t.Fatalf("want exactly 1 proposal entry, got %d: %+v", len(payload.Proposals), payload.Proposals)
	}
}

// TestHistoryViewer_NonDatedFilenameStillResolves covers E2: a proposal
// filename with no YYYY-MM-DD- prefix is still resolved by its full stem.
func TestHistoryViewer_NonDatedFilenameStillResolves(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "random-notes.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:random-notes"}},
		// Trailing .md on the label is tolerated too.
		{ID: "spexmachina-def", Status: "closed", Title: "y",
			Labels: []string{"spec_proposal:random-notes.md"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "random-notes.md") {
		t.Errorf("want non-dated proposal listed:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") || !strings.Contains(out, "spexmachina-def") {
		t.Errorf("want both tasks grouped under the same stem:\n%s", out)
	}
}

// TestHistoryViewer_LargeInput covers E5: hundreds of proposals and
// thousands of tasks complete correctly with in-memory filtering.
func TestHistoryViewer_LargeInput(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")

	const numProposals = 500
	const tasksPerProposal = 4

	var tasks []TaskRecord
	for i := 0; i < numProposals; i++ {
		stem := fmt.Sprintf("2026-01-%03d-proposal", i)
		writeProposal(t, specDir, stem+".md", changeContent)
		for j := 0; j < tasksPerProposal; j++ {
			tasks = append(tasks, TaskRecord{
				ID:     fmt.Sprintf("spexmachina-%d-%d", i, j),
				Status: "open",
				Title:  "x",
				Labels: []string{"spec_proposal:" + stem},
			})
		}
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(tasks); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(payload.Proposals) != numProposals {
		t.Fatalf("want %d proposals, got %d", numProposals, len(payload.Proposals))
	}
	for _, p := range payload.Proposals {
		if len(p.Tasks) != tasksPerProposal {
			t.Fatalf("want %d tasks per proposal, got %d", tasksPerProposal, len(p.Tasks))
		}
	}
}

// TestHistoryViewer_ConcurrentCalls covers E7: ShowHistory carries no shared
// mutable state, so concurrent calls against the same spec directory must
// each produce correct, independent output.
func TestHistoryViewer_ConcurrentCalls(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	tasks := []TaskRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	outs := make([]bytes.Buffer, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hv := &HistoryViewer{SpecDir: specDir, Out: &outs[idx], JSON: true}
			errs[idx] = hv.ShowHistory(tasks)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: ShowHistory: %v", i, err)
		}
		if !strings.Contains(outs[i].String(), "2026-04-18-decouple.md") {
			t.Errorf("goroutine %d: missing proposal in output:\n%s", i, outs[i].String())
		}
	}
}
