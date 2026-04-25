package proposal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestHistoryViewer_GroupsBeadsBySpecProposalLabel(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-spex-machina.md", projContent)
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-def", Status: "in_progress", Title: "validator: SchemaChecker",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-ghi", Status: "closed", Title: "merkle: DiffCommand",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
		// Bead without spec_proposal label is skipped.
		{ID: "spexmachina-pqr", Status: "open", Title: "unrelated"},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(beads); err != nil {
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
		t.Errorf("missing beads under first proposal:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-ghi") {
		t.Errorf("missing bead under second proposal:\n%s", out)
	}
	if strings.Contains(out, "spexmachina-pqr") {
		t.Errorf("unrelated bead leaked into output:\n%s", out)
	}
}

func TestHistoryViewer_RendersProposalTypeAndStatus(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-02-23-spex-machina.md", projContent)
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-02-23-spex-machina"}},
		{ID: "spexmachina-old", Status: "closed", Title: "apply: BeadCreator",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(beads); err != nil {
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
		t.Errorf("want Created action for open bead, got:\n%s", out)
	}
	if !strings.Contains(out, "Closed:") || !strings.Contains(out, "spexmachina-old") {
		t.Errorf("want Closed action for closed bead, got:\n%s", out)
	}
	if !strings.Contains(out, "(open)") || !strings.Contains(out, "(closed)") {
		t.Errorf("want bead status in parentheses, got:\n%s", out)
	}
}

func TestHistoryViewer_MissingProposalFile(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	// Create the proposals directory but no files.
	if err := os.MkdirAll(filepath.Join(specDir, "proposals"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{"spec_proposal:2026-99-99-ghost"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(beads); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "proposal file missing") {
		t.Errorf("want 'proposal file missing' label, got:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") {
		t.Errorf("want bead listed under missing-proposal entry, got:\n%s", out)
	}
}

func TestHistoryViewer_JSONMode(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "emit: ChangesetBuilder",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
		{ID: "spexmachina-old", Status: "closed", Title: "apply: BeadCreator",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(beads); err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Title    string `json:"title"`
			Beads    []struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Action  string `json:"action"`
				Summary string `json:"summary"`
			} `json:"beads"`
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
	if len(p.Beads) != 2 {
		t.Fatalf("want 2 beads, got %d", len(p.Beads))
	}
	if p.Beads[0].ID != "spexmachina-abc" || p.Beads[0].Action != "created" || p.Beads[0].Status != "open" {
		t.Errorf("first bead: %+v", p.Beads[0])
	}
	if p.Beads[0].Summary != "emit: ChangesetBuilder" {
		t.Errorf("want summary, got %q", p.Beads[0].Summary)
	}
	if p.Beads[1].Action != "closed" {
		t.Errorf("want closed action for closed bead, got %q", p.Beads[1].Action)
	}
}

func TestHistoryViewer_NoProposalsNoBeads(t *testing.T) {
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

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "schema: ProjectSchema",
			Labels: []string{
				"spec_proposal:2026-02-23-first",
				"spec_proposal:2026-03-01-second",
			}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf, JSON: true}
	if err := hv.ShowHistory(beads); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	var payload struct {
		Proposals []struct {
			Filename string `json:"filename"`
			Beads    []struct {
				ID string `json:"id"`
			} `json:"beads"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	var firstHasBead, secondHasBead bool
	for _, p := range payload.Proposals {
		for _, b := range p.Beads {
			if b.ID != "spexmachina-abc" {
				continue
			}
			if p.Filename == "2026-02-23-first.md" {
				firstHasBead = true
			}
			if p.Filename == "2026-03-01-second.md" {
				secondHasBead = true
			}
		}
	}
	if !firstHasBead {
		t.Errorf("bead should be grouped under first label")
	}
	if secondHasBead {
		t.Errorf("bead should NOT appear under second label")
	}
}

func TestHistoryViewer_NoSubprocessUsage(t *testing.T) {
	// HistoryViewer accepts parsed []BeadRecord and must not invoke any
	// external subprocess. This test does not assert on output; it merely
	// constructs a viewer with an empty PATH — if any exec.Command call
	// existed, it would fail. With a pure pipeline, this succeeds.
	t.Setenv("PATH", "")

	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-04-18-decouple.md", changeContent)

	beads := []BeadRecord{
		{ID: "spexmachina-abc", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-18-decouple"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(beads); err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}
}

func TestHistoryViewer_ProposalsSortedByFilename(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	writeProposal(t, specDir, "2026-03-01-b.md", changeContent)
	writeProposal(t, specDir, "2026-02-23-a.md", projContent)
	writeProposal(t, specDir, "2026-04-12-c.md", changeContent)

	beads := []BeadRecord{
		{ID: "id1", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-02-23-a"}},
		{ID: "id2", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-03-01-b"}},
		{ID: "id3", Status: "open", Title: "x",
			Labels: []string{"spec_proposal:2026-04-12-c"}},
	}

	var buf bytes.Buffer
	hv := &HistoryViewer{SpecDir: specDir, Out: &buf}
	if err := hv.ShowHistory(beads); err != nil {
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
