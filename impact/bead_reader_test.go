package impact

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFR1_ReadBeadsExtractsSpecMetadata(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{
		{
			ID:     "abc-123",
			Status: "open",
			Labels: []string{
				"spec_module:validator",
				"spec_component:SchemaChecker",
				"spec_hash:deadbeef",
			},
		},
		{
			ID:     "def-456",
			Status: "in_progress",
			Labels: []string{
				"spec_module:merkle",
				"spec_impl_section:Hashing algorithm",
				"spec_hash:cafebabe",
			},
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
	if b.Module != "validator" {
		t.Errorf("want module validator, got %s", b.Module)
	}
	if b.Component != "SchemaChecker" {
		t.Errorf("want component SchemaChecker, got %s", b.Component)
	}
	if b.SpecHash != "deadbeef" {
		t.Errorf("want spec_hash deadbeef, got %s", b.SpecHash)
	}

	b2 := beads[1]
	if b2.ImplSection != "Hashing algorithm" {
		t.Errorf("want impl_section 'Hashing algorithm', got %s", b2.ImplSection)
	}
}

func TestFR1_ReadBeadsIgnoresNonSpecBeads(t *testing.T) {
	stub := writeStubCLI(t, []rawBead{
		{
			ID:     "spec-bead",
			Status: "open",
			Labels: []string{"spec_module:validator", "spec_component:X"},
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

func TestFR1_ParseLabels(t *testing.T) {
	tests := []struct {
		name string
		raw  rawBead
		want BeadSpec
	}{
		{
			name: "all spec labels",
			raw: rawBead{
				ID:     "x",
				Status: "open",
				Labels: []string{
					"spec_module:impact",
					"spec_component:BeadReader",
					"spec_impl_section:Bead metadata reading",
					"spec_hash:abc123",
				},
			},
			want: BeadSpec{
				ID:          "x",
				Status:      "open",
				Module:      "impact",
				Component:   "BeadReader",
				ImplSection: "Bead metadata reading",
				SpecHash:    "abc123",
			},
		},
		{
			name: "no spec labels",
			raw:  rawBead{ID: "y", Status: "open", Labels: []string{"priority:high"}},
			want: BeadSpec{ID: "y", Status: "open"},
		},
		{
			name: "label without colon",
			raw:  rawBead{ID: "z", Status: "open", Labels: []string{"nocolon"}},
			want: BeadSpec{ID: "z", Status: "open"},
		},
		{
			name: "colon in value",
			raw:  rawBead{ID: "w", Status: "open", Labels: []string{"spec_module:a:b"}},
			want: BeadSpec{ID: "w", Status: "open", Module: "a:b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabels(tt.raw)
			if got != tt.want {
				t.Errorf("parseLabels() = %+v, want %+v", got, tt.want)
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

	// The stub script echoes the JSON when called with "list --json"
	script := "#!/bin/sh\necho '" + string(data) + "'\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Verify the stub is executable
	if _, err := exec.LookPath(stub); err != nil {
		// LookPath won't find it without full path, but CommandContext with full path works
		_ = err
	}

	return stub
}
