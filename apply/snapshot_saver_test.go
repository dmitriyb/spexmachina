package apply

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// setupSpecDir creates a minimal spec directory for testing.
func setupSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "Alpha", "path": "alpha"}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
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

func TestREQ5_SaveSnapshot_WritesFile(t *testing.T) {
	specDir := setupSpecDir(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := SaveSnapshot(ctx, specDir, now); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}
}

func TestREQ5_SaveSnapshot_Loadable(t *testing.T) {
	specDir := setupSpecDir(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := SaveSnapshot(ctx, specDir, now); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	loaded, err := merkle.Load(snapshotPath)
	if err != nil {
		t.Fatalf("Load snapshot: %v", err)
	}

	// The loaded tree should match a fresh build
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if loaded.Hash != tree.Hash {
		t.Fatalf("snapshot hash mismatch: saved=%s rebuilt=%s", loaded.Hash, tree.Hash)
	}
}

func TestREQ5_SaveSnapshot_Deterministic(t *testing.T) {
	specDir := setupSpecDir(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := SaveSnapshot(ctx, specDir, now); err != nil {
		t.Fatalf("first SaveSnapshot: %v", err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	first, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}

	if err := SaveSnapshot(ctx, specDir, now); err != nil {
		t.Fatalf("second SaveSnapshot: %v", err)
	}
	second, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read second snapshot: %v", err)
	}

	if string(first) != string(second) {
		t.Fatal("snapshot is not deterministic: two saves with same input produced different output")
	}
}

func TestREQ5_SaveSnapshot_InvalidSpecDir(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	err := SaveSnapshot(ctx, "/nonexistent/spec/dir", now)
	if err == nil {
		t.Fatal("want error for invalid spec dir, got nil")
	}
	if !strings.Contains(err.Error(), "apply: build tree for snapshot") {
		t.Fatalf("error should mention build tree, got: %v", err)
	}
}
