package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// setupImpactDiffFile creates a spec dir, builds a tree, snapshots it,
// modifies a file, runs diff, and writes the diff JSON to a temp file.
// Returns the spec dir path and the diff file path.
func setupImpactDiffFile(t *testing.T) (string, string) {
	t.Helper()
	specDir := setupTestSpec(t)

	// Snapshot the initial state.
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify a file to produce changes.
	archPath := filepath.Join(specDir, "alpha", "arch_comp1.md")
	if err := os.WriteFile(archPath, []byte("# Changed architecture\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture diff JSON output.
	diffJSON := captureStdout(t, func() {
		code := runDiff([]string{"--json", specDir})
		if code != 0 {
			t.Fatalf("diff command failed with exit code %d", code)
		}
	})

	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	return specDir, diffFile
}

// setupMappingFile creates a .bead-map.json with the given records.
func setupMappingFile(t *testing.T, dir string, records []mapping.Record) string {
	t.Helper()
	mapPath := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(mapPath)
	for _, r := range records {
		if _, err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	return mapPath
}

func TestFR4_ImpactCommand_ProducesReport(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	// Create a mapping record that matches the changed arch file.
	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp1"},
	})

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--map", mapPath, specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if report.Summary.ReviewCount == 0 {
		t.Fatal("expected at least one review action for changed arch file with matching record")
	}
}

func TestFR4_ImpactCommand_CreateForUnmatchedNode(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	// Empty mapping file — no records → changed nodes produce create actions.
	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--map", mapPath, specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if report.Summary.CreateCount == 0 {
		t.Fatal("expected create actions for unmatched nodes")
	}
}

func TestFR4_ImpactCommand_NoChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	// Snapshot, then diff with no modifications → empty changes.
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	diffJSON := captureStdout(t, func() {
		code := runDiff([]string{"--json", specDir})
		if code != 0 {
			t.Fatalf("diff failed: %d", code)
		}
	})

	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--map", mapPath, specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	total := report.Summary.CreateCount + report.Summary.CloseCount + report.Summary.ReviewCount
	if total != 0 {
		t.Fatalf("expected 0 actions with no changes, got %d", total)
	}
}

func TestNFR5_ImpactCommand_Deterministic(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp1"},
	})

	args := []string{"--diff", diffFile, "--map", mapPath, specDir}

	out1 := captureStdout(t, func() {
		runImpact(args)
	})
	out2 := captureStdout(t, func() {
		runImpact(args)
	})

	if out1 != out2 {
		t.Fatalf("determinism: outputs differ\nrun1: %s\nrun2: %s", out1, out2)
	}
}

func TestFR4_ImpactCommand_InvalidDiffJSON(t *testing.T) {
	diffFile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(diffFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	code := runImpact([]string{"--diff", diffFile})
	if code == 0 {
		t.Fatal("should fail with invalid diff JSON")
	}
}

func TestFR4_ImpactCommand_NonexistentDiffFile(t *testing.T) {
	code := runImpact([]string{"--diff", "/nonexistent/diff.json"})
	if code == 0 {
		t.Fatal("should fail with nonexistent diff file")
	}
}

func TestFR4_ParseDiffJSON(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"},
			{"path": "module/1/impl_section/1", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"},
			{"path": "module/1/component/2", "type": "removed", "impact": "arch_impl", "module": "alpha", "old_hash": "ddd"}
		]
	}`

	changes, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("want 3 changes, got %d", len(changes))
	}

	if changes[0].Type != merkle.Modified {
		t.Errorf("change 0: want Modified, got %v", changes[0].Type)
	}
	if changes[0].Impact != merkle.ArchImpl {
		t.Errorf("change 0: want ArchImpl, got %v", changes[0].Impact)
	}
	if changes[0].Module != "alpha" {
		t.Errorf("change 0: want module alpha, got %s", changes[0].Module)
	}
	if changes[1].Type != merkle.Added {
		t.Errorf("change 1: want Added, got %v", changes[1].Type)
	}
	if changes[2].Type != merkle.Removed {
		t.Errorf("change 2: want Removed, got %v", changes[2].Type)
	}
}

func TestFR4_ParseDiffJSON_InvalidType(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "bogus", "impact": "impl_only", "module": "m"}]}`
	_, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown change type") {
		t.Fatalf("want error about unknown change type, got %v", err)
	}
}

func TestFR4_ParseDiffJSON_InvalidImpact(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "added", "impact": "bogus", "module": "m"}]}`
	_, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown impact level") {
		t.Fatalf("want error about unknown impact level, got %v", err)
	}
}
