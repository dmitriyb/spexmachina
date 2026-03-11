package merkle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestREQ3_Save_CreatesValidJSON(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	now := time.Now().UTC()
	if err := Save(tree, snapPath, now); err != nil {
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
	now := time.Now().UTC()
	if err := Save(tree, snapPath, now); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	// Expect flat keys using spec-ID format
	expectedKeys := []string{
		"project",
		"project/meta",
		"module/1",
		"module/1/meta",
		"module/1/component/1",
		"module/1/component/2",
		"module/1/impl_section/1",
		"module/2",
		"module/2/meta",
		"module/2/component/1",
	}

	for _, key := range expectedKeys {
		if _, ok := snap.Nodes[key]; !ok {
			t.Errorf("missing expected node key %q", key)
		}
	}

	if len(snap.Nodes) != len(expectedKeys) {
		t.Errorf("node count: want %d, got %d", len(expectedKeys), len(snap.Nodes))
		for k := range snap.Nodes {
			found := false
			for _, ek := range expectedKeys {
				if k == ek {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("unexpected key: %q", k)
			}
		}
	}
}

func TestREQ3_Save_NodeTypes(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath, time.Now().UTC()))

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
		{"project", "project"},
		{"project/meta", "leaf"},
		{"module/1", "module"},
		{"module/1/meta", "leaf"},
		{"module/1/component/1", "leaf"},
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

func TestREQ3_Save_ChildrenAreKeys(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath, time.Now().UTC()))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	moduleNode := snap.Nodes["module/1"]
	if moduleNode == nil {
		t.Fatal("module/1 node not found")
	}
	wantChildren := []string{
		"module/1/component/1",
		"module/1/component/2",
		"module/1/impl_section/1",
		"module/1/meta",
	}
	if len(moduleNode.Children) != len(wantChildren) {
		t.Fatalf("children count: want %d, got %d", len(wantChildren), len(moduleNode.Children))
	}
	for i, want := range wantChildren {
		if moduleNode.Children[i] != want {
			t.Errorf("child[%d]: want %s, got %s", i, want, moduleNode.Children[i])
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
	must(t, Save(tree, snapPath, time.Now().UTC()))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	leaf := snap.Nodes["project/meta"]
	if leaf == nil {
		t.Fatal("project/meta node not found")
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
	if err := Save(tree, snapPath, time.Now().UTC()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Hash != tree.Hash {
		t.Fatalf("root hash: want %s, got %s", tree.Hash, loaded.Hash)
	}
	if loaded.Key != tree.Key {
		t.Fatalf("root key: want %s, got %s", tree.Key, loaded.Key)
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
	must(t, Save(tree, snapPath, time.Now().UTC()))

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify full tree structure matches
	assertTreeEqual(t, tree, loaded, "root")
}

func assertTreeEqual(t *testing.T, want, got *Node, path string) {
	t.Helper()
	if want.Key != got.Key {
		t.Fatalf("%s key: want %s, got %s", path, want.Key, got.Key)
	}
	if want.Hash != got.Hash {
		t.Fatalf("%s hash: want %s, got %s", path, want.Hash, got.Hash)
	}
	if want.Type != got.Type {
		t.Fatalf("%s type: want %s, got %s", path, want.Type, got.Type)
	}
	if want.NodeType != got.NodeType {
		t.Fatalf("%s node_type: want %s, got %s", path, want.NodeType, got.NodeType)
	}
	if want.Module != got.Module {
		t.Fatalf("%s module: want %d, got %d", path, want.Module, got.Module)
	}
	if len(want.Children) != len(got.Children) {
		t.Fatalf("%s children count: want %d, got %d", path, len(want.Children), len(got.Children))
	}
	for i := range want.Children {
		assertTreeEqual(t, want.Children[i], got.Children[i], path+"/"+want.Children[i].Key)
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
		RootHash: "some-hash",
		RootKey:  "nonexistent",
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
	if !strings.Contains(err.Error(), "root_key") {
		t.Fatalf("error should mention root_key, got: %v", err)
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

	// Same timestamp ensures byte-identical output
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	must(t, Save(tree, path1, fixedTime))
	must(t, Save(tree, path2, fixedTime))

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	if string(data1) != string(data2) {
		t.Fatal("snapshot files are not byte-identical")
	}
}

func TestREQ3_Save_NodeTypeAndModulePreserved(t *testing.T) {
	specDir := setupSpecDir(t)
	tree, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree, snapPath, time.Now().UTC()))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))

	compNode := snap.Nodes["module/1/component/1"]
	if compNode == nil {
		t.Fatal("module/1/component/1 node not found")
	}
	if compNode.NodeType != "component" {
		t.Errorf("node_type: want component, got %s", compNode.NodeType)
	}
	if compNode.Module != 1 {
		t.Errorf("module: want 1, got %d", compNode.Module)
	}
}
