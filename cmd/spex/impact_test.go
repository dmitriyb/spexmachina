package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/impact"
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

// fakeBR writes a fake bead CLI script that returns the given JSON.
func fakeBR(t *testing.T, beadsJSON string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-br")
	content := "#!/bin/sh\necho '" + beadsJSON + "'\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestFR4_ImpactCommand_ProducesReport(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	beads := []struct {
		ID     string   `json:"id"`
		Status string   `json:"status"`
		Labels []string `json:"labels"`
	}{
		{
			ID:     "bead-1",
			Status: "open",
			Labels: []string{"spec_module:alpha", "spec_component:Comp1"},
		},
	}
	beadsJSON, err := json.Marshal(beads)
	if err != nil {
		t.Fatal(err)
	}
	brScript := fakeBR(t, string(beadsJSON))

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", brScript, "--spec", specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if report.Summary.ReviewCount == 0 {
		t.Fatal("expected at least one review action for modified arch file with matching bead")
	}
	foundComp1 := false
	for _, r := range report.Reviews {
		if r.BeadID == "bead-1" {
			foundComp1 = true
		}
	}
	if !foundComp1 {
		t.Fatalf("expected review for bead-1, got reviews: %+v", report.Reviews)
	}
}

func TestFR4_ImpactCommand_CreateForUnmatchedNode(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	// No beads at all — should produce create actions.
	brScript := fakeBR(t, "[]")

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", brScript, "--spec", specDir})
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

	brScript := fakeBR(t, "[]")

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", brScript, "--spec", specDir})
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

	beads := []struct {
		ID     string   `json:"id"`
		Status string   `json:"status"`
		Labels []string `json:"labels"`
	}{
		{
			ID:     "bead-1",
			Status: "open",
			Labels: []string{"spec_module:alpha", "spec_component:Comp1"},
		},
	}
	beadsJSON, err := json.Marshal(beads)
	if err != nil {
		t.Fatal(err)
	}
	brScript := fakeBR(t, string(beadsJSON))

	args := []string{"--diff", diffFile, "--bead-cli", brScript, "--spec", specDir}

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

	code := runImpact([]string{"--diff", diffFile, "--bead-cli", "echo", "--spec", t.TempDir()})
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
			{"path": "project/alpha/arch/arch_comp1.md", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"},
			{"path": "project/alpha/impl/impl_comp1.md", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"},
			{"path": "project/alpha/arch/arch_removed.md", "type": "removed", "impact": "arch_impl", "module": "alpha", "old_hash": "ddd"}
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

func TestFR4_BuildNodeMaps(t *testing.T) {
	specDir := setupTestSpec(t)

	modules, err := buildNodeMaps(specDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// project.json and module.json both use "alpha".
	nm, ok := modules["alpha"]
	if !ok {
		t.Fatalf("expected NodeMap for module alpha, got keys: %v", mapKeys(modules))
	}

	if nm["arch_comp1.md"] != "Comp1" {
		t.Errorf("want arch_comp1.md → Comp1, got %q", nm["arch_comp1.md"])
	}
	if nm["impl_comp1.md"] != "Impl1" {
		t.Errorf("want impl_comp1.md → Impl1, got %q", nm["impl_comp1.md"])
	}
}

func mapKeys(m map[string]impact.NodeMap) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestFR4_BuildNodeMaps_NoModules(t *testing.T) {
	dir := t.TempDir()
	// Write a project.json with no modules.
	writeTestFile(t, dir, "project.json", `{"name":"empty","modules":[]}`)

	modules, err := buildNodeMaps(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(modules) != 0 {
		t.Fatalf("expected empty NodeMaps, got %d", len(modules))
	}
}
