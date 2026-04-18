package merkle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/schema"
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
		if c.NodeType == "" {
			t.Errorf("expected non-empty NodeType for %s", c.Path)
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
	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")
	alphaHash := schema.IdentityHash("module", "Alpha")
	if modified[0].Path != comp1Key {
		t.Errorf("expected modified key %s, got %s", comp1Key, modified[0].Path)
	}
	if modified[0].NodeType != "component" {
		t.Errorf("expected NodeType 'component', got %q", modified[0].NodeType)
	}
	if modified[0].Module != alphaHash {
		t.Errorf("expected Module %s, got %q", alphaHash, modified[0].Module)
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

	// Extend alpha's module.json with a second impl_section and create the file.
	alphaReq1 := schema.IdentityHash("alpha", "requirement", "Alpha req 1")
	alphaReq2 := schema.IdentityHash("alpha", "requirement", "Alpha req 2")
	alphaComp1 := schema.IdentityHash("alpha", "component", "Comp1")
	alphaComp2 := schema.IdentityHash("alpha", "component", "Comp2")
	alphaImpl1 := schema.IdentityHash("alpha", "impl_section", "Impl1")
	alphaImpl2 := schema.IdentityHash("alpha", "impl_section", "Impl2")
	projReq1 := schema.IdentityHash("project", "requirement", "000000000001")

	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": "` + alphaReq1 + `", "type": "functional", "title": "Alpha req 1", "preq_id": "` + projReq1 + `"},
			{"id": "` + alphaReq2 + `", "type": "functional", "title": "Alpha req 2", "description": "Details here", "depends_on": ["` + alphaReq1 + `"]}
		],
		"components": [
			{"id": "` + alphaComp1 + `", "name": "Comp1", "content": "arch_comp1.md"},
			{"id": "` + alphaComp2 + `", "name": "Comp2", "content": "arch_comp2.md"}
		],
		"impl_sections": [
			{"id": "` + alphaImpl1 + `", "name": "Impl1", "content": "impl_comp1.md"},
			{"id": "` + alphaImpl2 + `", "name": "Impl2", "content": "impl_comp2.md"}
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

	alphaHash := schema.IdentityHash("module", "Alpha")
	foundNewImpl := false
	for _, c := range added {
		if c.Path == alphaImpl2 {
			foundNewImpl = true
			if c.NodeType != "impl_section" {
				t.Errorf("expected NodeType 'impl_section', got %q", c.NodeType)
			}
			if c.Module != alphaHash {
				t.Errorf("expected Module %s, got %q", alphaHash, c.Module)
			}
		}
	}
	if !foundNewImpl {
		t.Errorf("expected added change for %s, changes: %v", alphaImpl2, changes)
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
		"requirements": [
			{"id": "000000000001", "type": "functional", "title": "Do stuff", "description": "The system must do stuff.", "priority": 1},
			{"id": "000000000002", "type": "non_functional", "title": "Be fast", "priority": 2}
		],
		"modules": [
			{"id": "000000000001", "name": "Alpha", "path": "alpha"}
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

	betaHash := schema.IdentityHash("module", "Beta")
	betaComp := schema.IdentityHash("beta", "component", "BetaComp")
	betaMetaKey := "meta/" + betaHash

	foundBetaMeta := false
	foundBetaComp := false
	for _, c := range removed {
		if c.Path == betaMetaKey {
			foundBetaMeta = true
			if c.NodeType != "meta" {
				t.Errorf("expected NodeType 'meta' for %s, got %q", c.Path, c.NodeType)
			}
			if c.Module != betaHash {
				t.Errorf("expected Module %s for %s, got %q", betaHash, c.Path, c.Module)
			}
		}
		if c.Path == betaComp {
			foundBetaComp = true
			if c.NodeType != "component" {
				t.Errorf("expected NodeType 'component' for %s, got %q", c.Path, c.NodeType)
			}
			if c.Module != betaHash {
				t.Errorf("expected Module %s for %s, got %q", betaHash, c.Path, c.Module)
			}
		}
	}
	if !foundBetaMeta {
		t.Errorf("expected removed change for %s, changes: %v", betaMetaKey, changes)
	}
	if !foundBetaComp {
		t.Errorf("expected removed change for %s, changes: %v", betaComp, changes)
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

func TestREQ7_Diff_MetadataOnAllNodeTypes(t *testing.T) {
	specDir := setupSpecDir(t)

	changes := Diff(mustBuildTree(t, specDir), nil)
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	projReq1 := schema.IdentityHash("project", "requirement", "000000000001")
	projReq2 := schema.IdentityHash("project", "requirement", "000000000002")
	projectLevelKeys := map[string]bool{
		"meta/project": true,
		projReq1:       true,
		projReq2:       true,
	}

	// Collect the node types present across all added leaves.
	nodeTypes := make(map[string]bool)
	for _, c := range changes {
		nodeTypes[c.NodeType] = true

		// Every leaf must carry metadata.
		if c.NodeType == "" {
			t.Errorf("missing NodeType for %s", c.Path)
		}
		// Project-level leaves have empty Module; all module-scoped leaves must have it.
		if !projectLevelKeys[c.Path] && c.Module == "" {
			t.Errorf("missing Module for %s", c.Path)
		}
	}

	// The test fixture has component, impl_section, and meta nodes at minimum.
	for _, want := range []string{"component", "impl_section", "meta"} {
		if !nodeTypes[want] {
			t.Errorf("expected at least one %s node type in changes", want)
		}
	}
}

func mustBuildTree(t *testing.T, specDir string) *Node {
	t.Helper()
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	return tree
}

// fixedTime is a deterministic timestamp for snapshot tests.
var fixedTime = func() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	return t
}()
