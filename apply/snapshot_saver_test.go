package apply

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// fixedSnapshotTime is the deterministic timestamp used across SnapshotSaver
// tests per test_snapshot.md scenarios.
var fixedSnapshotTime = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

// setupSpecDir creates a minimal valid spec directory matching the fixture
// described in spec/apply/test_snapshot.md: one module "testmod" with one
// component and one impl_section.
func setupSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "testmod", "path": "testmod"}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "testmod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir testmod: %v", err)
	}
	mod := `{
		"name": "testmod",
		"components": [
			{"id": "aabbccddeeff", "name": "Widget", "content": "arch_widget.md"}
		],
		"impl_sections": [
			{"id": "112233445566", "name": "WidgetImpl", "content": "impl_widget.md", "describes": ["aabbccddeeff"]}
		]
	}`
	writeTestFile(t, modDir, "module.json", mod)
	writeTestFile(t, modDir, "arch_widget.md", "# Widget architecture\n")
	writeTestFile(t, modDir, "impl_widget.md", "# Widget implementation\n")

	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestSaveSnapshot_S6_WritesToCorrectPath verifies test_snapshot.md S6:
// a file exists at <specDir>/.snapshot.json containing valid JSON with the
// expected created_at timestamp.
func TestSaveSnapshot_S6_WritesToCorrectPath(t *testing.T) {
	specDir := setupSpecDir(t)

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}

	var snap merkle.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("snapshot not valid JSON: %v", err)
	}
	if !snap.CreatedAt.Equal(fixedSnapshotTime) {
		t.Fatalf("created_at: want %v, got %v", fixedSnapshotTime, snap.CreatedAt)
	}
}

// TestSaveSnapshot_S7_ReflectsSpecContent verifies test_snapshot.md S7:
// the snapshot captures actual file content, so modifying arch_widget.md and
// re-saving produces a different root hash.
func TestSaveSnapshot_S7_ReflectsSpecContent(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshotPath := filepath.Join(specDir, ".snapshot.json")

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("first SaveSnapshot: %v", err)
	}
	first, err := merkle.Load(snapshotPath)
	if err != nil {
		t.Fatalf("load first snapshot: %v", err)
	}

	writeTestFile(t, filepath.Join(specDir, "testmod"), "arch_widget.md", "# Widget architecture\n\nNew content.\n")

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("second SaveSnapshot: %v", err)
	}
	second, err := merkle.Load(snapshotPath)
	if err != nil {
		t.Fatalf("load second snapshot: %v", err)
	}

	if first.Hash == second.Hash {
		t.Fatalf("root hash unchanged after content edit: %s", first.Hash)
	}
}

// TestSaveSnapshot_S8_OverwritesPrevious verifies test_snapshot.md S8:
// SaveSnapshot overwrites the existing .snapshot.json rather than creating a
// sibling file, and only one snapshot file remains.
func TestSaveSnapshot_S8_OverwritesPrevious(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshotPath := filepath.Join(specDir, ".snapshot.json")

	if err := os.WriteFile(snapshotPath, []byte(`{"stale":true}`), 0644); err != nil {
		t.Fatalf("write stale snapshot: %v", err)
	}

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if strings.Contains(string(data), `"stale"`) {
		t.Fatalf("snapshot not overwritten, still contains stale content")
	}

	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read specDir: %v", err)
	}
	var snapshots []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".snapshot.json") || strings.HasPrefix(e.Name(), ".snapshot") {
			snapshots = append(snapshots, e.Name())
		}
	}
	if len(snapshots) != 1 {
		t.Fatalf("want exactly 1 snapshot file, got %d: %v", len(snapshots), snapshots)
	}
}

// TestSaveSnapshot_S9_DeterministicOutput verifies test_snapshot.md S9:
// two calls with the same createdAt and unchanged spec produce byte-identical
// snapshot files.
func TestSaveSnapshot_S9_DeterministicOutput(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshotPath := filepath.Join(specDir, ".snapshot.json")

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("first SaveSnapshot: %v", err)
	}
	first, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	if err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime); err != nil {
		t.Fatalf("second SaveSnapshot: %v", err)
	}
	second, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}

	if string(first) != string(second) {
		t.Fatal("snapshot is not deterministic: two saves with same input produced different output")
	}
}

// TestSaveSnapshot_E1_InvalidSpecDir verifies test_snapshot.md E1:
// a nonexistent spec directory produces a "apply: build tree for snapshot:"
// error and no snapshot is written.
func TestSaveSnapshot_E1_InvalidSpecDir(t *testing.T) {
	tmp := t.TempDir()
	bogus := filepath.Join(tmp, "no-such-dir")

	err := SaveSnapshot(context.Background(), bogus, fixedSnapshotTime)
	if err == nil {
		t.Fatal("want error for invalid spec dir, got nil")
	}
	if !strings.HasPrefix(err.Error(), "apply: build tree for snapshot:") {
		t.Fatalf("error should start with %q, got: %v", "apply: build tree for snapshot:", err)
	}

	if _, statErr := os.Stat(filepath.Join(bogus, ".snapshot.json")); !os.IsNotExist(statErr) {
		t.Fatalf("snapshot should not exist, stat err: %v", statErr)
	}
}

// TestSaveSnapshot_E2_MalformedModuleJSON verifies test_snapshot.md E2:
// a malformed module.json causes SaveSnapshot to fail without writing a
// snapshot, so the next run can retry after the user fixes the JSON.
func TestSaveSnapshot_E2_MalformedModuleJSON(t *testing.T) {
	specDir := setupSpecDir(t)
	writeTestFile(t, filepath.Join(specDir, "testmod"), "module.json", "{ not valid json")

	err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime)
	if err == nil {
		t.Fatal("want error for malformed module.json, got nil")
	}
	if !strings.HasPrefix(err.Error(), "apply: build tree for snapshot:") {
		t.Fatalf("error should propagate from BuildTree, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(specDir, ".snapshot.json")); !os.IsNotExist(statErr) {
		t.Fatalf("snapshot should not be written on build failure, stat err: %v", statErr)
	}
}

// TestSaveSnapshot_E6_ReadOnlySpecDir verifies test_snapshot.md E6:
// if .snapshot.json cannot be written due to permissions, the error starts
// with "apply: save snapshot:" so the user gets a clear write-failure message.
func TestSaveSnapshot_E6_ReadOnlySpecDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not applicable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks; skip read-only test")
	}

	specDir := setupSpecDir(t)
	if err := os.Chmod(specDir, 0555); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(specDir, 0755) })

	err := SaveSnapshot(context.Background(), specDir, fixedSnapshotTime)
	if err == nil {
		t.Fatal("want error on read-only spec dir, got nil")
	}
	if !strings.HasPrefix(err.Error(), "apply: save snapshot:") {
		t.Fatalf("error should start with %q, got: %v", "apply: save snapshot:", err)
	}
}
