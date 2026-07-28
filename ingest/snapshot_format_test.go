package ingest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// TestSnapshotWritersShareOneFormat pins the invariant that cost this
// repository a duplicated tree walk: ingest writes the snapshot atomically
// and merkle.Save writes it in place, but both must emit the same bytes.
// They did diverge once — ingest carried its own flatten() "mirroring"
// merkle's, and nothing compared them, so either could have drifted while
// both kept passing their own tests.
func TestSnapshotWritersShareOneFormat(t *testing.T) {
	specDir := t.TempDir()
	writeFile(t, specDir, "project.json", `{
		"name": "fmt-fixture",
		"modules": [{"id": "aabbccddee10", "name": "alpha", "path": "alpha"}]
	}`)
	alphaDir := filepath.Join(specDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [{"id": "aabbccddee01", "name": "Widget", "content": "arch_widget.md"}]
	}`)
	writeFile(t, alphaDir, "arch_widget.md", "# Widget\n")

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	stamp := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	atomicPath := filepath.Join(t.TempDir(), "atomic.json")
	if err := writeAtomic(atomicPath, tree, stamp); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	plainPath := filepath.Join(t.TempDir(), "plain.json")
	if err := merkle.Save(tree, plainPath, stamp); err != nil {
		t.Fatalf("merkle.Save: %v", err)
	}

	atomicBytes, err := os.ReadFile(atomicPath)
	if err != nil {
		t.Fatal(err)
	}
	plainBytes, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(atomicBytes) != string(plainBytes) {
		t.Fatalf("the two snapshot writers disagree on the format:\natomic (%d bytes):\n%s\nplain (%d bytes):\n%s",
			len(atomicBytes), atomicBytes, len(plainBytes), plainBytes)
	}
}
