package merkle

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestREQ4_Diff_NoSnapshot_AllAdded(t *testing.T) {
	specDir := setupSpecDir(t)
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, nil)
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	for _, c := range changes {
		if c.Type != Added {
			t.Errorf("expected type 'added' for %s, got %q", c.Path, c.Type)
		}
		if c.NewHash == "" {
			t.Errorf("expected non-empty NewHash for %s", c.Path)
		}
		if c.OldHash != "" {
			t.Errorf("expected empty OldHash for added %s, got %q", c.Path, c.OldHash)
		}
	}
}

func TestREQ4_Diff_IdenticalTrees_NoChanges(t *testing.T) {
	specDir := setupSpecDir(t)
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestREQ4_Diff_ModifiedLeaf(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Modify a leaf file
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Modified content\n")
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)

	var modified []Change
	for _, c := range changes {
		if c.Type == Modified {
			modified = append(modified, c)
		}
	}

	if len(modified) != 1 {
		t.Fatalf("expected 1 modified change, got %d: %v", len(modified), changes)
	}
	if modified[0].Path != "module/1/component/1" {
		t.Errorf("expected modified path module/1/component/1, got %s", modified[0].Path)
	}
	if modified[0].OldHash == "" || modified[0].NewHash == "" {
		t.Error("expected both OldHash and NewHash to be non-empty for modified change")
	}
	if modified[0].OldHash == modified[0].NewHash {
		t.Error("OldHash and NewHash should differ for modified change")
	}
}

func TestREQ4_Diff_AddedLeaf(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Add a new impl section to alpha's module.json and create the file
	alphaMod := `{
		"name": "alpha",
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md"},
			{"id": 2, "name": "Comp2", "content": "arch_comp2.md"}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md"},
			{"id": 2, "name": "Impl2", "content": "impl_comp2.md"}
		]
	}`
	alphaDir := filepath.Join(specDir, "alpha")
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "impl_comp2.md", "# New impl\n")

	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)

	var added []Change
	for _, c := range changes {
		if c.Type == Added {
			added = append(added, c)
		}
	}

	// The new impl_section/2 is added; module meta is modified (its hash changed)
	foundNewImpl := false
	for _, c := range added {
		if c.Path == "module/1/impl_section/2" {
			foundNewImpl = true
		}
	}
	if !foundNewImpl {
		t.Errorf("expected added change for module/1/impl_section/2, changes: %v", changes)
	}
}

func TestREQ4_Diff_RemovedLeaf(t *testing.T) {
	specDir := setupSpecDir(t)

	// Build snapshot with both modules
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Remove beta module from project.json
	proj := `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "Alpha", "path": "alpha"}
		]
	}`
	writeFile(t, specDir, "project.json", proj)

	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)

	var removed []Change
	for _, c := range changes {
		if c.Type == Removed {
			removed = append(removed, c)
		}
	}

	if len(removed) == 0 {
		t.Fatalf("expected removed changes, got none. all changes: %v", changes)
	}

	foundBeta := false
	for _, c := range removed {
		if strings.HasPrefix(c.Path, "module/2") {
			foundBeta = true
		}
	}
	if !foundBeta {
		t.Errorf("expected removed change for module/2, changes: %v", changes)
	}
}

func TestREQ4_Diff_LeafOnlyReporting(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Modify a leaf to cause interior hash changes
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Changed\n")
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	changes := Diff(current, snapshot)

	// Verify all changes have valid ChangeType values (not zero-value or unexpected)
	validTypes := map[ChangeType]bool{Added: true, Removed: true, Modified: true}
	for _, c := range changes {
		if !validTypes[c.Type] {
			t.Errorf("unexpected change type %d for path: %s", c.Type, c.Path)
		}
	}

	// Only the one modified leaf should appear — not its ancestor interior nodes
	if len(changes) != 1 {
		t.Fatalf("expected 1 leaf change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != Modified {
		t.Errorf("expected Modified, got %v", changes[0].Type)
	}
}

func TestREQ4_Diff_Deterministic(t *testing.T) {
	specDir := setupSpecDir(t)

	// Modify a file to create some changes
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# V2\n")
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Build a "previous" tree from a fresh spec dir
	snapshotDir := setupSpecDir(t)
	snapshot, err := BuildTree(snapshotDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Run diff twice
	changes1 := Diff(current, snapshot)
	changes2 := Diff(current, snapshot)

	if len(changes1) != len(changes2) {
		t.Fatalf("non-deterministic: diff1 has %d changes, diff2 has %d", len(changes1), len(changes2))
	}

	for i := range changes1 {
		if changes1[i] != changes2[i] {
			t.Errorf("non-deterministic at index %d: %v vs %v", i, changes1[i], changes2[i])
		}
	}

	// Verify sorted by path
	for i := 1; i < len(changes1); i++ {
		if changes1[i].Path < changes1[i-1].Path {
			t.Errorf("changes not sorted: %q comes after %q", changes1[i].Path, changes1[i-1].Path)
		}
	}
}

func TestREQ4_Diff_SaveLoadRoundtrip(t *testing.T) {
	specDir := setupSpecDir(t)
	snapshot, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Save and reload the snapshot
	snapPath := filepath.Join(t.TempDir(), "snapshot.json")
	if err := Save(snapshot, snapPath, fixedTime); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Modify a file and build current tree
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# V2\n")
	current, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Diff against original and loaded should produce same results
	changesOrig := Diff(current, snapshot)
	changesLoaded := Diff(current, loaded)

	if len(changesOrig) != len(changesLoaded) {
		t.Fatalf("roundtrip mismatch: orig %d changes, loaded %d", len(changesOrig), len(changesLoaded))
	}
	for i := range changesOrig {
		if changesOrig[i] != changesLoaded[i] {
			t.Errorf("roundtrip mismatch at %d: %v vs %v", i, changesOrig[i], changesLoaded[i])
		}
	}
}

// fixedTime is a deterministic timestamp for snapshot tests.
var fixedTime = func() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	return t
}()
