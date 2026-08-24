package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/merkle"
)

// setupSpecDir creates a minimal valid spec directory that BuildTree can
// hash. Mirrors the apply-package fixture but lives here so ingest tests
// stay self-contained.
func setupSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "testmod", "path": "testmod"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "testmod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir testmod: %v", err)
	}
	mod := `{
		"name": "testmod",
		"components": [
			{"id": "aabbccddeeff", "name": "Widget", "content": "arch_widget.md"}
		]
	}`
	writeFile(t, modDir, "module.json", mod)
	writeFile(t, modDir, "arch_widget.md", "# Widget\n")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestSaver_Complete_WritesSnapshot(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := s.Save(adapters.StatusComplete)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !wrote {
		t.Fatalf("wrote: want true on complete, got false")
	}

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	var snap merkle.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("snapshot not valid JSON: %v", err)
	}
	if snap.RootHash == "" {
		t.Fatalf("snapshot has empty root_hash: %s", string(data))
	}
}

func TestSaver_Partial_SkipsWrite(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := s.Save(adapters.StatusPartial)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if wrote {
		t.Fatalf("wrote: want false on partial, got true")
	}
	if _, statErr := os.Stat(snapPath); !os.IsNotExist(statErr) {
		t.Fatalf("snapshot must not be created on partial, stat err: %v", statErr)
	}
}

func TestSaver_Partial_LeavesExistingSnapshotUntouched(t *testing.T) {
	// This is invariant 6 from test_consistency_invariants.md: partial
	// status must leave spec/.snapshot.json byte-for-byte unchanged so
	// the next spex plan diffs against the original baseline.
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	baseline := []byte(`{"baseline":true,"root_hash":"deadbeef"}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := s.Save(adapters.StatusPartial)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if wrote {
		t.Fatalf("wrote: want false on partial, got true")
	}

	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != string(baseline) {
		t.Fatalf("snapshot changed on partial run:\n got %s\nwant %s", got, baseline)
	}
}

func TestSaver_Complete_OverwritesExistingSnapshot(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	if err := os.WriteFile(snapPath, []byte(`{"stale":true}`), 0644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if strings.Contains(string(data), `"stale"`) {
		t.Fatalf("snapshot not overwritten, still contains stale marker: %s", data)
	}
}

func TestSaver_NonCompleteStatuses_Skip(t *testing.T) {
	// Anything other than the literal "complete" must be a no-op write —
	// future status vocabularies must opt in to writing explicitly.
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	for _, status := range []string{"", "partial", "unknown", "Complete", "COMPLETE"} {
		s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
		wrote, err := s.Save(status)
		if err != nil {
			t.Fatalf("Save(%q): %v", status, err)
		}
		if wrote {
			t.Fatalf("Save(%q): want wrote=false, got true", status)
		}
		if _, statErr := os.Stat(snapPath); !os.IsNotExist(statErr) {
			t.Fatalf("Save(%q): snapshot must not exist, stat err: %v", status, statErr)
		}
	}
}

func TestSaver_AtomicWrite_NoLeftoverTmpOnSuccess(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(snapPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file must be cleaned up after rename, stat err: %v", err)
	}

	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read specDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file in specDir: %s", e.Name())
		}
	}
}

func TestSaver_BuildTreeError_ReturnsErrorAndPreservesExisting(t *testing.T) {
	// On a BuildTree failure (malformed module.json), Save must return
	// (false, err) and leave any pre-existing snapshot untouched. This
	// is the partial-style protection: a write failure cannot retroactively
	// invalidate a known-good baseline.
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	baseline := []byte(`{"baseline":true}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	writeFile(t, filepath.Join(specDir, "testmod"), "module.json", "{ not valid json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := s.Save(adapters.StatusComplete)
	if err == nil {
		t.Fatalf("want error on malformed module.json, got nil")
	}
	if wrote {
		t.Fatalf("wrote: want false on error, got true")
	}
	if !strings.HasPrefix(err.Error(), "ingest: snapshot:") {
		t.Fatalf("error must be prefixed %q, got: %v", "ingest: snapshot:", err)
	}

	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != string(baseline) {
		t.Fatalf("snapshot baseline corrupted on error path:\n got %s\nwant %s", got, baseline)
	}
}

func TestSaver_UnsetSnapshotPath_IsCallerError(t *testing.T) {
	// The writer computes no location of its own: an unset SnapshotPath
	// on a complete-status run is a caller error, not a fallback to some
	// default location.
	specDir := setupSpecDir(t)

	s := &Saver{SpecDir: specDir} // SnapshotPath omitted on purpose
	wrote, err := s.Save(adapters.StatusComplete)
	if err == nil {
		t.Fatalf("Save: want error for unset SnapshotPath, got nil")
	}
	if wrote {
		t.Fatalf("wrote: want false on error, got true")
	}
}

func TestSaver_UnsetSpecDir_IsCallerError(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SnapshotPath: snapPath} // SpecDir omitted on purpose
	wrote, err := s.Save(adapters.StatusComplete)
	if err == nil {
		t.Fatalf("Save: want error for unset SpecDir, got nil")
	}
	if wrote {
		t.Fatalf("wrote: want false on error, got true")
	}
	if _, statErr := os.Stat(snapPath); !os.IsNotExist(statErr) {
		t.Fatalf("snapshot must not be created, stat err: %v", statErr)
	}
}

func TestSaver_DeterministicAcrossCalls(t *testing.T) {
	// Two complete saves against unchanged spec must produce snapshots
	// that re-load to the same root hash. Byte-equality is not required
	// (timestamps may differ) but root_hash must match.
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	first, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load first: %v", err)
	}

	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	second, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load second: %v", err)
	}

	if first.Hash != second.Hash {
		t.Fatalf("root hash drift across saves: %s vs %s", first.Hash, second.Hash)
	}
}

func TestSaver_ReflectsSpecChanges(t *testing.T) {
	// After editing a spec file, a complete save must produce a snapshot
	// with a different root hash — this protects against caching the
	// pre-edit tree.
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	before, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}

	writeFile(t, filepath.Join(specDir, "testmod"), "arch_widget.md", "# Widget\n\nedited\n")

	if _, err := s.Save(adapters.StatusComplete); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	after, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}

	if before.Hash == after.Hash {
		t.Fatalf("root hash unchanged after content edit: %s", before.Hash)
	}
}

func TestSaver_WriteFailure_NoPartialSnapshot(t *testing.T) {
	// If rename cannot succeed (snapshot path points into a nonexistent
	// nested directory), Save must return an error and leave nothing
	// behind at the destination. The atomic write contract: either the
	// final file appears whole or nothing appears.
	if runtime.GOOS == "windows" {
		t.Skip("POSIX rename semantics not applicable on Windows")
	}
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, "missing-subdir", ".snapshot.json")

	s := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := s.Save(adapters.StatusComplete)
	if err == nil {
		t.Fatalf("want error when destination dir is missing, got nil")
	}
	if wrote {
		t.Fatalf("wrote: want false on write failure, got true")
	}
	if !strings.HasPrefix(err.Error(), "ingest: snapshot:") {
		t.Fatalf("error must be prefixed %q, got: %v", "ingest: snapshot:", err)
	}
	if _, statErr := os.Stat(snapPath); !os.IsNotExist(statErr) {
		t.Fatalf("partial snapshot left behind, stat err: %v", statErr)
	}
}
