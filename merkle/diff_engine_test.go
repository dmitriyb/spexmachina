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
			t.Errorf("expected type 'added' for %s, got %q", c.Key, c.Type)
		}
		if c.NewHash == "" {
			t.Errorf("expected non-empty NewHash for %s", c.Key)
		}
		if c.OldHash != "" {
			t.Errorf("expected empty OldHash for added %s, got %q", c.Key, c.OldHash)
		}
		if c.NodeType == "" {
			t.Errorf("expected non-empty NodeType for %s", c.Key)
		}
	}
}

func TestREQ4_Diff_BootstrapEmptyTreeBaseline(t *testing.T) {
	specDir := setupSpecDir(t)
	current := mustBuildTree(t, specDir)

	// Bootstrap path: on a project being born, SnapshotStore.Load reads
	// back the empty tree spex init seeded at spec/.snapshot.json — the
	// engine has no notion of a missing baseline; an absent file is
	// refused upstream by the lifecycle pre-flight, never surfaced here.
	// DiffEngine must treat that empty baseline the same as a fresh
	// project and report every current leaf as "added" — see
	// arch_diff_engine.md "Bootstrap behavior".
	changes := Diff(current, EmptyTree())

	if len(changes) == 0 {
		t.Fatal("expected every leaf reported as added, got no changes")
	}
	for _, c := range changes {
		if c.Type != Added {
			t.Errorf("expected Added for %s, got %s", c.Key, c.Type)
		}
		if c.OldHash != "" {
			t.Errorf("expected empty OldHash for added %s, got %q", c.Key, c.OldHash)
		}
		if c.NewHash == "" {
			t.Errorf("expected non-empty NewHash for added %s", c.Key)
		}
	}

	// One change per current leaf — nothing dropped, nothing synthesized.
	leaves := make(map[string]leafInfo)
	flattenLeaves(leaves, current)
	if len(changes) != len(leaves) {
		t.Errorf("expected %d added changes (one per leaf), got %d", len(leaves), len(changes))
	}

	// The EmptyTree() baseline and the nil baseline must agree: bootstrap
	// and the no-snapshot path share the same DiffEngine call.
	if nilChanges := Diff(current, nil); len(nilChanges) != len(changes) {
		t.Errorf("EmptyTree() baseline (%d changes) disagrees with nil baseline (%d changes)",
			len(changes), len(nilChanges))
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
	if modified[0].Key != comp1Key {
		t.Errorf("expected modified key %s, got %s", comp1Key, modified[0].Key)
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

	// Extend alpha's module.json with a second test_section and create the file.
	alphaReq1 := schema.IdentityHash("alpha", "requirement", "Alpha req 1")
	alphaReq2 := schema.IdentityHash("alpha", "requirement", "Alpha req 2")
	alphaComp1 := schema.IdentityHash("alpha", "component", "Comp1")
	alphaComp2 := schema.IdentityHash("alpha", "component", "Comp2")
	alphaTest1 := schema.IdentityHash("alpha", "test_section", "Test1")
	alphaTest2 := schema.IdentityHash("alpha", "test_section", "Test2")
	projReq1 := fixtureProjReq1ID

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
		"test_sections": [
			{"id": "` + alphaTest1 + `", "name": "Test1", "content": "test_comp1.md"},
			{"id": "` + alphaTest2 + `", "name": "Test2", "content": "test_comp2.md"}
		]
	}`
	alphaDir := filepath.Join(specDir, "alpha")
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "test_comp2.md", "# New tests\n")

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
		if c.Key == alphaTest2 {
			foundNewImpl = true
			if c.NodeType != "test_section" {
				t.Errorf("expected NodeType 'test_section', got %q", c.NodeType)
			}
			if c.Module != alphaHash {
				t.Errorf("expected Module %s, got %q", alphaHash, c.Module)
			}
		}
	}
	if !foundNewImpl {
		t.Errorf("expected added change for %s, changes: %v", alphaTest2, changes)
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
		if c.Key == betaMetaKey {
			foundBetaMeta = true
			if c.NodeType != "meta" {
				t.Errorf("expected NodeType 'meta' for %s, got %q", c.Key, c.NodeType)
			}
			if c.Module != betaHash {
				t.Errorf("expected Module %s for %s, got %q", betaHash, c.Key, c.Module)
			}
		}
		if c.Key == betaComp {
			foundBetaComp = true
			if c.NodeType != "component" {
				t.Errorf("expected NodeType 'component' for %s, got %q", c.Key, c.NodeType)
			}
			if c.Module != betaHash {
				t.Errorf("expected Module %s for %s, got %q", betaHash, c.Key, c.Module)
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
			t.Errorf("unexpected change type %d for path: %s", c.Type, c.Key)
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
		if changes1[i].Key < changes1[i-1].Key {
			t.Errorf("changes not sorted: %q comes after %q", changes1[i].Key, changes1[i-1].Key)
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

	projReq1 := fixtureProjReq1ID
	projReq2 := fixtureProjReq2ID
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
			t.Errorf("missing NodeType for %s", c.Key)
		}
		// Project-level leaves have empty Module; all module-scoped leaves must have it.
		if !projectLevelKeys[c.Key] && c.Module == "" {
			t.Errorf("missing Module for %s", c.Key)
		}
	}

	// The test fixture has component, test_section, and meta nodes at minimum.
	for _, want := range []string{"component", "test_section", "meta"} {
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
