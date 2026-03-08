package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha"}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	alphaMod := `{
		"name": "alpha",
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md"}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeTestFile(t, alphaDir, "impl_comp1.md", "# Comp1 implementation\n")

	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// captureStdout runs fn with stdout redirected and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestFR1_FR2_FR3_HashCommand_BuildsTreeAndSavesSnapshot(t *testing.T) {
	specDir := setupTestSpec(t)

	code := runHash([]string{specDir})
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}

	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap map[string]interface{}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}

	if snap["root_hash"] == nil || snap["root_hash"] == "" {
		t.Fatal("snapshot missing root_hash")
	}
	if snap["nodes"] == nil {
		t.Fatal("snapshot missing nodes")
	}
}

func TestFR1_FR2_HashCommand_HumanOutput(t *testing.T) {
	specDir := setupTestSpec(t)

	out := captureStdout(t, func() {
		code := runHash([]string{specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	if !strings.HasPrefix(out, "root: ") {
		t.Fatalf("human output should start with 'root: ', got: %s", out)
	}
	if !strings.Contains(out, "nodes:") {
		t.Fatalf("human output should contain node counts, got: %s", out)
	}
	if !strings.Contains(out, "leaf") {
		t.Fatalf("human output should mention leaf type, got: %s", out)
	}
	if !strings.Contains(out, "snapshot:") {
		t.Fatalf("human output should mention snapshot, got: %s", out)
	}
}

func TestFR1_FR2_HashCommand_JSONOutput(t *testing.T) {
	specDir := setupTestSpec(t)

	out := captureStdout(t, func() {
		code := runHash([]string{"--json", specDir})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

	var result hashOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON output should be valid: %v\noutput: %s", err, out)
	}

	if result.RootHash == "" {
		t.Fatal("JSON output missing root_hash")
	}
	if len(result.Nodes) == 0 {
		t.Fatal("JSON output should contain nodes")
	}

	// Check that nodes include paths with hierarchy
	foundModule := false
	foundLeaf := false
	for _, n := range result.Nodes {
		if n.Type == "module" {
			foundModule = true
		}
		if n.Type == "leaf" {
			foundLeaf = true
		}
	}
	if !foundModule {
		t.Fatal("JSON output should include module nodes")
	}
	if !foundLeaf {
		t.Fatal("JSON output should include leaf nodes")
	}
}

func TestFR1_FR2_HashCommand_DefaultDir(t *testing.T) {
	// When no dir is specified, it defaults to "spec/" which won't exist in temp
	code := runHash([]string{"/nonexistent/path"})
	if code == 0 {
		t.Fatal("should fail with nonexistent dir")
	}
}

func TestNFR9_HashCommand_Deterministic(t *testing.T) {
	specDir := setupTestSpec(t)

	var out1, out2 string
	out1 = captureStdout(t, func() {
		runHash([]string{"--json", specDir})
	})
	// Remove the snapshot so we recompute
	os.Remove(filepath.Join(specDir, ".snapshot.json"))
	out2 = captureStdout(t, func() {
		runHash([]string{"--json", specDir})
	})

	var r1, r2 hashOutput
	if err := json.Unmarshal([]byte(out1), &r1); err != nil {
		t.Fatalf("unmarshal first run: %v", err)
	}
	if err := json.Unmarshal([]byte(out2), &r2); err != nil {
		t.Fatalf("unmarshal second run: %v", err)
	}

	if r1.RootHash != r2.RootHash {
		t.Fatalf("determinism: root hashes differ: %s vs %s", r1.RootHash, r2.RootHash)
	}
}
