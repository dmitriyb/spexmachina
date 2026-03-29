package proposal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubLister is a test double for BeadLister.
type stubLister struct {
	beads []BeadRecord
	err   error
}

func (s *stubLister) ListBeads(_ context.Context) ([]BeadRecord, error) {
	return s.beads, s.err
}

func TestFR2_ShowHistory_HumanReadable(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	os.MkdirAll(proposalsDir, 0755)

	// Create a project proposal file.
	projContent := "# Project Proposal: Spex Machina\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	os.WriteFile(filepath.Join(proposalsDir, "2026-02-23-spex-machina.md"), []byte(projContent), 0644)

	lister := &stubLister{
		beads: []BeadRecord{
			{
				ID:       "spexmachina-abc",
				Title:    "schema: ProjectSchema",
				Metadata: map[string]string{"spec_proposal": "2026-02-23-spex-machina"},
			},
		},
	}

	var buf bytes.Buffer
	err := ShowHistory(context.Background(), specDir, lister, &buf, false)
	if err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-02-23-spex-machina.md (project proposal)") {
		t.Errorf("want proposal header, got:\n%s", out)
	}
	if !strings.Contains(out, "spexmachina-abc") {
		t.Errorf("want bead ID in output, got:\n%s", out)
	}
}

func TestFR2_ShowHistory_JSON(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	os.MkdirAll(proposalsDir, 0755)

	projContent := "# Project Proposal: Test\n\n## Vision\n\nx\n\n## Modules\n\nx\n\n## Key requirements\n\nx\n\n## Design decisions\n\nx\n"
	os.WriteFile(filepath.Join(proposalsDir, "2026-02-23-test.md"), []byte(projContent), 0644)

	lister := &stubLister{beads: []BeadRecord{}}

	var buf bytes.Buffer
	err := ShowHistory(context.Background(), specDir, lister, &buf, true)
	if err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}

	var result []ProposalEntry
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	if len(result) != 1 {
		t.Fatalf("want 1 proposal, got %d", len(result))
	}
	if result[0].Proposal != "2026-02-23-test.md" {
		t.Errorf("want proposal 2026-02-23-test.md, got %q", result[0].Proposal)
	}
	if result[0].Type != "project" {
		t.Errorf("want type project, got %q", result[0].Type)
	}
	if result[0].Date != "2026-02-23" {
		t.Errorf("want date 2026-02-23, got %q", result[0].Date)
	}
}

func TestFR2_ShowHistory_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	os.MkdirAll(filepath.Join(specDir, "proposals"), 0755)

	lister := &stubLister{beads: []BeadRecord{}}

	// Human-readable: empty output.
	var buf bytes.Buffer
	err := ShowHistory(context.Background(), specDir, lister, &buf, false)
	if err != nil {
		t.Fatalf("ShowHistory: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("want empty output, got %q", buf.String())
	}

	// JSON: empty array.
	buf.Reset()
	err = ShowHistory(context.Background(), specDir, lister, &buf, true)
	if err != nil {
		t.Fatalf("ShowHistory JSON: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("want '[]', got %q", buf.String())
	}
}

func TestFR2_ShowHistory_BeadListerError(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	proposalsDir := filepath.Join(specDir, "proposals")
	os.MkdirAll(proposalsDir, 0755)

	content := "# Change Proposal: X\n\n## Context\n\nx\n\n## Proposed change\n\nx\n\n## Impact expectation\n\nx\n"
	os.WriteFile(filepath.Join(proposalsDir, "2026-03-01-x.md"), []byte(content), 0644)

	lister := &stubLister{err: fmt.Errorf("proposal: bead CLI unavailable: br not found")}

	var buf bytes.Buffer
	err := ShowHistory(context.Background(), specDir, lister, &buf, false)
	if err == nil {
		t.Fatal("want error when bead lister fails, got nil")
	}
	if !strings.Contains(err.Error(), "bead CLI unavailable") {
		t.Errorf("want bead CLI error, got: %v", err)
	}
}

func TestFR2_ExtractDate(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"2026-02-23-spex-machina.md", "2026-02-23"},
		{"2026-03-09-skills.md", "2026-03-09"},
		{"short.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractDate(tt.filename)
			if got != tt.want {
				t.Errorf("extractDate(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestFR2_ParseBeadTitle(t *testing.T) {
	tests := []struct {
		title      string
		wantModule string
		wantNode   string
	}{
		{"schema: ProjectSchema", "schema", "ProjectSchema"},
		{"NoColonHere", "", "NoColonHere"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			m, n := parseBeadTitle(tt.title)
			if m != tt.wantModule || n != tt.wantNode {
				t.Errorf("parseBeadTitle(%q) = (%q, %q), want (%q, %q)", tt.title, m, n, tt.wantModule, tt.wantNode)
			}
		})
	}
}
