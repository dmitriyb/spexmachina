package merkle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/schema"
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

	alphaHash := schema.IdentityHash("module", "Alpha")
	betaHash := schema.IdentityHash("module", "Beta")

	// Expect flat keys using identity hash format
	expectedKeys := []string{
		"project",
		"meta/project",
		fixtureProjReq1ID,
		fixtureProjReq2ID,
		alphaHash,
		"meta/" + alphaHash,
		schema.IdentityHash("alpha", "requirement", "Alpha req 1"),
		schema.IdentityHash("alpha", "requirement", "Alpha req 2"),
		schema.IdentityHash("alpha", "component", "Comp1"),
		schema.IdentityHash("alpha", "component", "Comp2"),
		schema.IdentityHash("alpha", "test_section", "Test1"),
		betaHash,
		"meta/" + betaHash,
		schema.IdentityHash("beta", "component", "BetaComp"),
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

	alphaHash := schema.IdentityHash("module", "Alpha")
	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")

	tests := []struct {
		key      string
		wantType string
	}{
		{"project", "project"},
		{"meta/project", "leaf"},
		{alphaHash, "module"},
		{"meta/" + alphaHash, "leaf"},
		{comp1Key, "leaf"},
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

	alphaHash := schema.IdentityHash("module", "Alpha")
	moduleNode := snap.Nodes[alphaHash]
	if moduleNode == nil {
		t.Fatalf("%s node not found", alphaHash)
	}

	// Alpha has 6 children sorted by key
	if len(moduleNode.Children) != 6 {
		t.Fatalf("children count: want 6, got %d", len(moduleNode.Children))
	}

	// All children should be valid node keys
	for _, childKey := range moduleNode.Children {
		if _, ok := snap.Nodes[childKey]; !ok {
			t.Errorf("child key %q not found in nodes", childKey)
		}
	}

	// Children should be sorted
	for i := 1; i < len(moduleNode.Children); i++ {
		if moduleNode.Children[i] < moduleNode.Children[i-1] {
			t.Errorf("children not sorted: %s comes after %s", moduleNode.Children[i], moduleNode.Children[i-1])
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

	leaf := snap.Nodes["meta/project"]
	if leaf == nil {
		t.Fatal("meta/project node not found")
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
		t.Fatalf("%s module: want %q, got %q", path, want.Module, got.Module)
	}
	if len(want.Children) != len(got.Children) {
		t.Fatalf("%s children count: want %d, got %d", path, len(want.Children), len(got.Children))
	}
	for i := range want.Children {
		assertTreeEqual(t, want.Children[i], got.Children[i], path+"/"+want.Children[i].Key)
	}
}

// TestREQ3_Load_MissingFile_AbsenceError pins the contract from
// spec/merkle/arch_snapshot_store.md ("Read vs. write call sites") and
// test_snapshots.md E1: when the snapshot file does not exist, Load
// returns a nil tree and an error wrapping ErrSnapshotAbsent — never a
// fallback tree. The empty tree is produced in exactly one place now:
// the seed snapshot `spex init` writes at project birth.
func TestREQ3_Load_MissingFile_AbsenceError(t *testing.T) {
	tree, err := Load(filepath.Join(t.TempDir(), "does-not-exist", ".snapshot.json"))
	if err == nil {
		t.Fatal("Load on missing file: want error, got nil")
	}
	if tree != nil {
		t.Fatalf("Load on missing file: want nil tree, got %+v", tree)
	}
	if !errors.Is(err, ErrSnapshotAbsent) {
		t.Fatalf("Load on missing file: want error wrapping ErrSnapshotAbsent, got %v", err)
	}
}

// TestREQ3_Load_MissingFile_DistinguishableFromParseFailure is E1's
// second clause: the absence error must be distinguishable from a parse
// failure, so a caller can report "uninitialised or broken" rather than
// "invalid".
func TestREQ3_Load_MissingFile_DistinguishableFromParseFailure(t *testing.T) {
	_, absenceErr := Load(filepath.Join(t.TempDir(), "does-not-exist", ".snapshot.json"))

	badPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, os.WriteFile(badPath, []byte("not json"), 0644))
	_, parseErr := Load(badPath)

	if errors.Is(parseErr, ErrSnapshotAbsent) {
		t.Fatal("parse failure must not be reported as ErrSnapshotAbsent")
	}
	if !errors.Is(absenceErr, ErrSnapshotAbsent) {
		t.Fatal("missing-file failure must be reported as ErrSnapshotAbsent")
	}
}

// TestREQ3_Load_PermissionErrorStillFails ensures the missing-file
// special case does not swallow other I/O errors. A read failure that
// is NOT os.IsNotExist must still surface as an error so callers can
// distinguish "no snapshot yet" from "the snapshot is unreadable".
func TestREQ3_Load_PermissionErrorStillFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".snapshot.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	// Skip when running as root (chmod 0 does not block root reads).
	if data, readErr := os.ReadFile(path); readErr == nil {
		_ = data
		t.Skip("running as root — chmod 0 does not block reads, skipping permission-error contract test")
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("want error for unreadable snapshot, got nil")
	}
	if errors.Is(err, ErrSnapshotAbsent) {
		t.Fatal("permission-denied error must not be reported as ErrSnapshotAbsent")
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

	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")
	compNode := snap.Nodes[comp1Key]
	if compNode == nil {
		t.Fatalf("%s node not found", comp1Key)
	}
	if compNode.NodeType != "component" {
		t.Errorf("node_type: want component, got %s", compNode.NodeType)
	}
	alphaHash := schema.IdentityHash("module", "Alpha")
	if compNode.Module != alphaHash {
		t.Errorf("module: want %s, got %s", alphaHash, compNode.Module)
	}
}

func TestREQ3_Save_OverwritesPrevious(t *testing.T) {
	specDir := setupSpecDir(t)
	tree1, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(tree1, snapPath, time.Now().UTC()))

	// Modify a content file to produce a different tree
	modDir := filepath.Join(specDir, "alpha")
	must(t, os.WriteFile(filepath.Join(modDir, "arch_comp1.md"), []byte("# Modified content\n"), 0644))
	tree2, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree after modification: %v", err)
	}
	if tree1.Hash == tree2.Hash {
		t.Fatal("trees should have different root hashes after modification")
	}

	must(t, Save(tree2, snapPath, time.Now().UTC()))

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Hash != tree2.Hash {
		t.Fatalf("loaded hash: want %s (tree2), got %s", tree2.Hash, loaded.Hash)
	}
}

func TestREQ3_Save_PrettyPrintedJSON(t *testing.T) {
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

	content := string(data)
	if !strings.Contains(content, "\n  ") {
		t.Fatal("snapshot JSON is not indented (not pretty-printed)")
	}
	if !strings.Contains(content, "root_hash") {
		t.Fatal("snapshot missing root_hash field in readable form")
	}
}

func TestREQ3_Save_EmptyTree(t *testing.T) {
	root := &Node{
		Key:  "project",
		Hash: "abc123",
		Type: "project",
	}

	snapPath := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, Save(root, snapPath, time.Now().UTC()))

	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snap Snapshot
	must(t, json.Unmarshal(data, &snap))
	if len(snap.Nodes) != 1 {
		t.Fatalf("nodes: want 1, got %d", len(snap.Nodes))
	}

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Hash != root.Hash {
		t.Fatalf("hash: want %s, got %s", root.Hash, loaded.Hash)
	}
	if len(loaded.Children) != 0 {
		t.Fatalf("children: want 0, got %d", len(loaded.Children))
	}
}

func TestREQ3_Load_UnknownNodeType(t *testing.T) {
	snap := Snapshot{
		RootHash: "abc",
		RootKey:  "project",
		Nodes: map[string]*SnapshotNode{
			"project":        {Hash: "abc", Type: "project", Children: []string{"project/widget"}},
			"project/widget": {Hash: "def", Type: "unknown_type", NodeType: "future_kind"},
		},
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	path := filepath.Join(t.TempDir(), ".snapshot.json")
	must(t, os.WriteFile(path, data, 0644))

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load should succeed with unknown type, got: %v", err)
	}
	child := loaded.Children[0]
	if child.Type != "unknown_type" {
		t.Fatalf("type: want unknown_type, got %s", child.Type)
	}
	if child.NodeType != "future_kind" {
		t.Fatalf("node_type: want future_kind, got %s", child.NodeType)
	}
}

func TestREQ3_Save_CreatesParentDirs(t *testing.T) {
	root := &Node{
		Key:  "project",
		Hash: "xyz",
		Type: "project",
	}

	snapPath := filepath.Join(t.TempDir(), "spec", "nested", ".snapshot.json")
	if err := Save(root, snapPath, time.Now().UTC()); err != nil {
		t.Fatalf("Save should create parent dirs, got: %v", err)
	}

	loaded, err := Load(snapPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Hash != root.Hash {
		t.Fatalf("hash: want %s, got %s", root.Hash, loaded.Hash)
	}
}
