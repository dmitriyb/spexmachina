package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
)

func TestFR4_ImpactCommand_FromDiffFile(t *testing.T) {
	// Create a diff JSON file as input.
	diffJSON := `{
		"changes": [
			{"path": "project/alpha/arch/arch_comp1.md", "type": "added", "impact": "arch_impl", "module": "alpha", "new_hash": "abc123"}
		],
		"summary": {"total": 1, "by_type": {"added": 1}, "by_impact": {"arch_impl": 1}}
	}`
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Use a nonexistent bead CLI — ReadBeads should fail, but we can test
	// with a script that returns empty beads.
	beadScript := writeFakeBeadCLI(t, "[]")

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", beadScript})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if report.Summary.CreateCount != 1 {
		t.Fatalf("want 1 create, got %d", report.Summary.CreateCount)
	}
	if report.Creates[0].Module != "alpha" {
		t.Fatalf("want module 'alpha', got %q", report.Creates[0].Module)
	}
}

func TestFR4_ImpactCommand_WithMatchingBeads(t *testing.T) {
	diffJSON := `{
		"changes": [
			{"path": "project/alpha/arch/arch_comp1.md", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "old", "new_hash": "new"}
		],
		"summary": {"total": 1}
	}`
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	beadJSON := `[{"id": "bead-42", "labels": {"spec_module": "alpha", "spec_component": "Comp1"}}]`
	beadScript := writeFakeBeadCLI(t, beadJSON)

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", beadScript})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if report.Summary.ReviewCount != 1 {
		t.Fatalf("want 1 review, got %d", report.Summary.ReviewCount)
	}
	if report.Reviews[0].BeadID != "bead-42" {
		t.Fatalf("want bead_id 'bead-42', got %q", report.Reviews[0].BeadID)
	}
}

func TestFR4_ImpactCommand_EmptyDiff(t *testing.T) {
	diffJSON := `{"changes": [], "summary": {"total": 0}}`
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	beadScript := writeFakeBeadCLI(t, "[]")

	out := captureStdout(t, func() {
		code := runImpact([]string{"--diff", diffFile, "--bead-cli", beadScript})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if report.Summary.CreateCount != 0 || report.Summary.CloseCount != 0 || report.Summary.ReviewCount != 0 {
		t.Fatalf("empty diff should produce zero counts, got %+v", report.Summary)
	}
}

func TestFR4_ImpactCommand_BadDiffFile(t *testing.T) {
	code := runImpact([]string{"--diff", "/nonexistent/diff.json"})
	if code == 0 {
		t.Fatal("should fail with nonexistent diff file")
	}
}

func TestFR4_ImpactCommand_InvalidJSON(t *testing.T) {
	diffFile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(diffFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	code := runImpact([]string{"--diff", diffFile})
	if code == 0 {
		t.Fatal("should fail with invalid JSON")
	}
}

func TestNFR5_ImpactCommand_Deterministic(t *testing.T) {
	diffJSON := `{
		"changes": [
			{"path": "project/b/arch/arch_z.md", "type": "added", "impact": "arch_impl", "module": "b"},
			{"path": "project/a/arch/arch_y.md", "type": "added", "impact": "arch_impl", "module": "a"}
		],
		"summary": {"total": 2}
	}`
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	beadScript := writeFakeBeadCLI(t, "[]")

	run := func() string {
		return captureStdout(t, func() {
			code := runImpact([]string{"--diff", diffFile, "--bead-cli", beadScript})
			if code != 0 {
				t.Fatalf("want exit 0, got %d", code)
			}
		})
	}

	out1 := run()
	out2 := run()
	if out1 != out2 {
		t.Fatalf("non-deterministic output:\nrun1: %s\nrun2: %s", out1, out2)
	}
}

// writeFakeBeadCLI creates a shell script that outputs the given JSON
// when called with "list --json". Returns the path to the script.
func writeFakeBeadCLI(t *testing.T, jsonOutput string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-bead-cli")
	content := "#!/bin/sh\necho '" + jsonOutput + "'\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("write fake bead CLI: %v", err)
	}
	return script
}
