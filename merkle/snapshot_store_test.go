package merkle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestREQ3_Save_CreatesValidJSON(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	if err := Save(tree, snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}

	if snap.RootHash == "" {
		t.Fatal("root_hash is empty")
	}
	if snap.RootHash != tree.Hash {
		t.Fatalf("root_hash: want %s, got %s", tree.Hash, snap.RootHash)
	}
	if snap.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}
	if len(snap.Nodes) == 0 {
		t.Fatal("nodes map is empty")
	}
}

func TestREQ3_Save_FlatNodeMap(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	if err := Save(tree, snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	// Expect flat keys like "test-project", "test-project/project.json",
	// "test-project/Alpha", "test-project/Alpha/module.json", etc.
	expectedKeys := []string{
		"test-project",
		"test-project/project.json",
		"test-project/Alpha",
		"test-project/Alpha/module.json",
		"test-project/Alpha/arch",
		"test-project/Alpha/arch/arch_comp1.md",
		"test-project/Alpha/arch/arch_comp2.md",
		"test-project/Alpha/impl",
		"test-project/Alpha/impl/impl_comp1.md",
		"test-project/Beta",
		"test-project/Beta/module.json",
		"test-project/Beta/arch",
		"test-project/Beta/arch/arch_beta.md",
	}

	for _, key := range expectedKeys {
		if _, ok := snap.Nodes[key]; !ok {
			t.Errorf("missing expected node key %q", key)
		}
	}

	if len(snap.Nodes) != len(expectedKeys) {
		t.Errorf("node count: want %d, got %d", len(expectedKeys), len(snap.Nodes))
	}
}

func TestREQ3_Save_NodeTypes(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	tests := []struct {
		key      string
		wantType string
	}{
		{"test-project", "project"},
		{"test-project/project.json", "leaf"},
		{"test-project/Alpha", "module"},
		{"test-project/Alpha/arch", "arch"},
		{"test-project/Alpha/impl", "impl"},
		{"test-project/Alpha/arch/arch_comp1.md", "leaf"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			sn, ok := snap.Nodes[tt.key]
			if !ok {
				t.Fatalf("node %q not found", tt.key)
			}
			if sn.Type != tt.wantType {
				t.Fatalf("type: want %s, got %s", tt.wantType, sn.Type)
			}
		})
	}
}

func TestREQ3_Save_ChildrenArePaths(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	archNode := snap.Nodes["test-project/Alpha/arch"]
	if archNode == nil {
		t.Fatal("arch node not found")
	}
	wantChildren := []string{
		"test-project/Alpha/arch/arch_comp1.md",
		"test-project/Alpha/arch/arch_comp2.md",
	}
	if len(archNode.Children) != len(wantChildren) {
		t.Fatalf("children count: want %d, got %d", len(wantChildren), len(archNode.Children))
	}
	for i, want := range wantChildren {
		if archNode.Children[i] != want {
			t.Errorf("child[%d]: want %s, got %s", i, want, archNode.Children[i])
		}
	}
}

func TestREQ3_Save_LeafNoChildren(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	leaf := snap.Nodes["test-project/project.json"]
	if leaf == nil {
		t.Fatal("project.json node not found")
	}
	if len(leaf.Children) != 0 {
		t.Fatalf("leaf should have no children, got %d", len(leaf.Children))
	}
}

func TestREQ3_LoadSave_RoundTrip(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	if err := Save(tree, snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Hash != tree.Hash {
		t.Fatalf("root hash: want %s, got %s", tree.Hash, loaded.Hash)
	}
	if loaded.Name != tree.Name {
		t.Fatalf("root name: want %s, got %s", tree.Name, loaded.Name)
	}
	if loaded.Type != tree.Type {
		t.Fatalf("root type: want %s, got %s", tree.Type, loaded.Type)
	}
}

func TestREQ3_LoadSave_PreservesStructure(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath))

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify full tree structure matches
	assertTreeEqual(t, tree, loaded, "root")
}

func assertTreeEqual(t *testing.T, want, got *Node, path string) {
	t.Helper()
	if want.Name != got.Name {
		t.Fatalf("%s name: want %s, got %s", path, want.Name, got.Name)
	}
	if want.Hash != got.Hash {
		t.Fatalf("%s hash: want %s, got %s", path, want.Hash, got.Hash)
	}
	if want.Type != got.Type {
		t.Fatalf("%s type: want %s, got %s", path, want.Type, got.Type)
	}
	if len(want.Children) != len(got.Children) {
		t.Fatalf("%s children count: want %d, got %d", path, len(want.Children), len(got.Children))
	}
	for i := range want.Children {
		assertTreeEqual(t, want.Children[i], got.Children[i], path+"/"+want.Children[i].Name)
	}
}

func TestREQ3_Load_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/.snapshot.json")
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("error should mention snapshot, got: %v", err)
	}
}

func TestREQ3_Load_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, os.WriteFile(path, []byte("not json"), 0644))

	_, err := Load(path)
	if err == nil {
		t.Fatal("want error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error should mention parse, got: %v", err)
	}
}

func TestREQ3_Load_MissingRootNode(t *testing.T) {
	snap := Snapshot{
		RootHash: "nonexistent-hash",
		Nodes:    map[string]*SnapshotNode{},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	path := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, os.WriteFile(path, data, 0644))

	_, err = Load(path)
	if err == nil {
		t.Fatal("want error for missing root node, got nil")
	}
	if !strings.Contains(err.Error(), "root_hash") {
		t.Fatalf("error should mention root_hash, got: %v", err)
	}
}

func TestREQ3_Save_Deterministic(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	dir := t.TempDir()
	path1 := filepath.Join(dir, "snap1.json")
	path2 := filepath.Join(dir, "snap2.json")

	must(t, Save(tree, path1))
	must(t, Save(tree, path2))

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	// Parse both to compare without timestamp
	var s1, s2 Snapshot
	must(t, json.Unmarshal(data1, &s1))
	must(t, json.Unmarshal(data2, &s2))

	if s1.RootHash != s2.RootHash {
		t.Fatal("root hashes differ between saves")
	}
	if len(s1.Nodes) != len(s2.Nodes) {
		t.Fatal("node counts differ between saves")
	}
	for key, n1 := range s1.Nodes {
		n2, ok := s2.Nodes[key]
		if !ok {
			t.Fatalf("node %q missing from second save", key)
		}
		if n1.Hash != n2.Hash {
			t.Fatalf("node %q hash differs between saves", key)
		}
	}
}
