package impact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFR1_ReadBeadsExtractsRecordID(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{
		{
			ID:     "abc-123",
			Status: "open",
			Labels: []string{"spex:42"},
		},
		{
			ID:     "def-456",
			Status: "in_progress",
			Labels: []string{"spex:7"},
		},
	})

	beads, err := ReadBeads(context.Background(), stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 2 {
		t.Fatalf("want 2 beads, got %d", len(beads))
	}

	b := beads[0]
	if b.ID != "abc-123" {
		t.Errorf("want ID abc-123, got %s", b.ID)
	}
	if b.Status != "open" {
		t.Errorf("want status open, got %s", b.Status)
	}
	if b.RecordID != 42 {
		t.Errorf("want RecordID 42, got %d", b.RecordID)
	}

	b2 := beads[1]
	if b2.ID != "def-456" {
		t.Errorf("want ID def-456, got %s", b2.ID)
	}
	if b2.RecordID != 7 {
		t.Errorf("want RecordID 7, got %d", b2.RecordID)
	}
}

func TestFR1_ReadBeadsIgnoresNonSpecBeads(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{
		{
			ID:     "spec-bead",
			Status: "open",
			Labels: []string{"spex:1"},
		},
		{
			ID:     "plain-bead",
			Status: "open",
			Labels: []string{"priority:high"},
		},
		{
			ID:     "no-labels",
			Status: "open",
			Labels: []string{},
		},
	})

	beads, err := ReadBeads(context.Background(), stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 1 {
		t.Fatalf("want 1 bead, got %d", len(beads))
	}
	if beads[0].ID != "spec-bead" {
		t.Errorf("want spec-bead, got %s", beads[0].ID)
	}
}

func TestFR1_ReadBeadsEmptyList(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{})

	beads, err := ReadBeads(context.Background(), stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 0 {
		t.Errorf("want 0 beads, got %d", len(beads))
	}
}

func TestFR1_ReadBeadsCLINotFound(t *testing.T) {
	_, err := ReadBeads(context.Background(), "nonexistent-bead-cli-xyz")
	if err == nil {
		t.Fatal("want error for missing CLI, got nil")
	}
	if !strings.Contains(err.Error(), "impact: read beads:") {
		t.Errorf("want wrapped error, got: %v", err)
	}
}

func TestFR1_ReadBeadsCLIBadJSON(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "br-stub")
	script := "#!/bin/sh\necho 'not json'\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ReadBeads(context.Background(), stub)
	if err == nil {
		t.Fatal("want error for bad JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse JSON") {
		t.Errorf("want parse JSON error, got: %v", err)
	}
}

func TestFR1_ReadBeadsNoSpecLabelsReturnsEmpty(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{
		{
			ID:     "task-1",
			Status: "open",
			Labels: []string{"team:backend", "priority:high"},
		},
	})

	beads, err := ReadBeads(context.Background(), stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 0 {
		t.Errorf("want 0 beads (no spex labels), got %d", len(beads))
	}
}

func TestFR1_ExtractRecordID(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		wantID int
		wantOK bool
	}{
		{
			name:   "valid spex label",
			labels: []string{"spex:42"},
			wantID: 42,
			wantOK: true,
		},
		{
			name:   "spex label among others",
			labels: []string{"team:backend", "spex:7", "priority:high"},
			wantID: 7,
			wantOK: true,
		},
		{
			name:   "no spex label",
			labels: []string{"priority:high"},
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "empty labels",
			labels: []string{},
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "nil labels",
			labels: nil,
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "spex label with non-numeric value",
			labels: []string{"spex:abc"},
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "spex label with zero",
			labels: []string{"spex:0"},
			wantID: 0,
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := extractRecordID(tt.labels)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("extractRecordID(%v) = (%d, %v), want (%d, %v)",
					tt.labels, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

// writeStubCLI creates a shell script that outputs the given beads as JSON
// when called with "list --json". Returns the path to the script.
func writeStubCLI(t *testing.T, beads []rawBead) string {
	t.Helper()
	data, err := json.Marshal(beads)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	stub := filepath.Join(dir, "br-stub")

	// Write JSON to a separate file and cat it to avoid quoting issues
	jsonFile := filepath.Join(dir, "data.json")
	if err := os.WriteFile(jsonFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat " + jsonFile + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return stub
}
