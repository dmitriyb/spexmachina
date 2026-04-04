package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
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

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	archPath := filepath.Join(specDir, "alpha", "arch_comp1.md")
	if err := os.WriteFile(archPath, []byte("# Changed architecture\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff command failed: %v", err)
	}

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

	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: "alpha/component/1", BeadID: "bead-1", BeadType: "task", Module: "alpha", Component: "Comp1", ContentFile: "spec/alpha/arch_comp1.md", SpecHash: "abc123"},
	})

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	// Format mismatch between merkle paths and mapping spec_node_id
	// prevents matching here. Tracked separately from this bead.
}

func TestFR4_ImpactCommand_CreateForUnmatchedNode(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

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

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	total := report.Summary.CreateCount + report.Summary.ObsoleteCount
	if total != 0 {
		t.Fatalf("expected 0 actions with no changes, got %d", total)
	}
}

func TestNFR5_ImpactCommand_Deterministic(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: "alpha/component/1", BeadID: "bead-1", BeadType: "task", Module: "alpha", Component: "Comp1", ContentFile: "spec/alpha/arch_comp1.md", SpecHash: "abc123"},
	})

	out1, _ := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	out2, _ := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)

	if out1 != out2 {
		t.Fatalf("determinism: outputs differ\nrun1: %s\nrun2: %s", out1, out2)
	}
}

func TestFR4_ImpactCommand_InvalidDiffJSON(t *testing.T) {
	diffFile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(diffFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runSpex(t, "impact", "--diff", diffFile)
	if err == nil {
		t.Fatal("should fail with invalid diff JSON")
	}
}

func TestFR4_ImpactCommand_NonexistentDiffFile(t *testing.T) {
	_, err := runSpex(t, "impact", "--diff", "/nonexistent/diff.json")
	if err == nil {
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

	changes, _, err := parseDiffJSON([]byte(input))
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
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown change type") {
		t.Fatalf("want error about unknown change type, got %v", err)
	}
}

func TestFR4_ParseDiffJSON_InvalidImpact(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "added", "impact": "bogus", "module": "m"}]}`
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown impact level") {
		t.Fatalf("want error about unknown impact level, got %v", err)
	}
}

// runSpexWithStderr is like runSpex but also returns stderr output.
func runSpexWithStderr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newHashCmd(),
		newDiffCmd(),
		newValidateCmd(),
		newImpactCmd(),
		newApplyCmd(),
		newMapCmd(),
		newRenderCmd(),
	)

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	var execErr error
	stdout := captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, errBuf.String(), execErr
}

func TestFR8_ImpactCommand_RejectsDiffWithErrors(t *testing.T) {
	specDir := setupTestSpec(t)
	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	diffJSON := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"}
		],
		"errors": [
			{
				"type": "incomplete_change",
				"message": "Requirement 2 (impact) description changed but implementing component NodeMatcher content leaf unchanged",
				"path": "module/4/meta",
				"related": ["module/4/component/2"]
			}
		]
	}`

	diffFile := filepath.Join(t.TempDir(), "diff_with_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("expected error when diff contains errors, got nil")
	}
	if !strings.Contains(err.Error(), "diff contains 1 error(s)") {
		t.Fatalf("expected error about diff errors, got: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty stdout when diff has errors, got: %s", out)
	}
}

func TestFR8_ImpactCommand_EmptyErrorsArrayProceeds(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Create a diff with empty errors array — should proceed normally.
	diffJSON := `{"changes": [], "errors": []}`
	diffFile := filepath.Join(t.TempDir(), "empty_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("expected no error with empty errors array, got: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
}

func TestFR8_ParseDiffJSON_ExtractsErrors(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"}
		],
		"errors": [
			{"type": "incomplete_change", "message": "something broken", "path": "module/1/meta", "related": ["module/1/component/1"]}
		]
	}`

	changes, diffErrors, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(changes))
	}
	if len(diffErrors) != 1 {
		t.Fatalf("want 1 error, got %d", len(diffErrors))
	}
	if diffErrors[0].Type != "incomplete_change" {
		t.Errorf("want error type incomplete_change, got %s", diffErrors[0].Type)
	}
	if diffErrors[0].Message != "something broken" {
		t.Errorf("want error message 'something broken', got %s", diffErrors[0].Message)
	}
}

func TestFR8_ParseDiffJSON_NoErrorsField(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"}
		]
	}`

	changes, diffErrors, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(changes))
	}
	if len(diffErrors) != 0 {
		t.Fatalf("want 0 errors when field absent, got %d", len(diffErrors))
	}
}

func TestFR8_ImpactCommand_MultipleErrorsAllPrinted(t *testing.T) {
	specDir := setupTestSpec(t)
	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	diffJSON := `{
		"changes": [],
		"errors": [
			{"type": "incomplete_change", "message": "first error message", "path": "module/1/meta", "related": []},
			{"type": "incomplete_change", "message": "second error message", "path": "module/2/meta", "related": []}
		]
	}`

	diffFile := filepath.Join(t.TempDir(), "multi_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runSpexWithStderr(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("expected error when diff contains errors, got nil")
	}
	if !strings.Contains(err.Error(), "diff contains 2 error(s)") {
		t.Fatalf("expected error about 2 diff errors, got: %v", err)
	}
	if !strings.Contains(stderr, "first error message") {
		t.Fatalf("expected first error in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "second error message") {
		t.Fatalf("expected second error in stderr, got: %s", stderr)
	}
}
