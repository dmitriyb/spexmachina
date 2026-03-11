package merkle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSpecDir creates a minimal spec directory for testing.
// Returns the spec dir path.
func setupSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// project.json with two modules
	proj := `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "Alpha", "path": "alpha"},
			{"id": 2, "name": "Beta", "path": "beta"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	// Module alpha: has components and impl_sections
	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := `{
		"name": "alpha",
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md"},
			{"id": 2, "name": "Comp2", "content": "arch_comp2.md"}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md"}
		]
	}`
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeFile(t, alphaDir, "arch_comp2.md", "# Comp2 architecture\n")
	writeFile(t, alphaDir, "impl_comp1.md", "# Comp1 implementation\n")

	// Module beta: has only one component
	betaDir := filepath.Join(dir, "beta")
	must(t, os.MkdirAll(betaDir, 0755))
	betaMod := `{
		"name": "beta",
		"components": [
			{"id": 1, "name": "BetaComp", "content": "arch_beta.md"}
		]
	}`
	writeFile(t, betaDir, "module.json", betaMod)
	writeFile(t, betaDir, "arch_beta.md", "# Beta architecture\n")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestREQ7_BuildTree_SpecIDKeys(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if root.Key != "project" {
		t.Fatalf("root key: want project, got %s", root.Key)
	}

	// project/meta leaf
	projLeaf := root.Children[0]
	if projLeaf.Key != "project/meta" {
		t.Fatalf("project meta key: want project/meta, got %s", projLeaf.Key)
	}
	if projLeaf.NodeType != "meta" {
		t.Fatalf("project meta node_type: want meta, got %s", projLeaf.NodeType)
	}

	// module/1 (Alpha)
	alpha := root.Children[1]
	if alpha.Key != "module/1" {
		t.Fatalf("alpha key: want module/1, got %s", alpha.Key)
	}

	// module/2 (Beta)
	beta := root.Children[2]
	if beta.Key != "module/2" {
		t.Fatalf("beta key: want module/2, got %s", beta.Key)
	}
}

func TestREQ2_BuildTree_Structure(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Root is project node
	if root.Type != "project" {
		t.Fatalf("root type: want project, got %s", root.Type)
	}

	// Children: project/meta leaf + 2 module nodes
	if len(root.Children) != 3 {
		t.Fatalf("root children: want 3, got %d", len(root.Children))
	}

	projLeaf := root.Children[0]
	if projLeaf.Type != "leaf" {
		t.Fatalf("first child type: want leaf, got %s", projLeaf.Type)
	}

	alpha := root.Children[1]
	if alpha.Type != "module" {
		t.Fatalf("second child type: want module, got %s", alpha.Type)
	}

	beta := root.Children[2]
	if beta.Type != "module" {
		t.Fatalf("third child type: want module, got %s", beta.Type)
	}
}

func TestREQ7_BuildTree_FlatModuleChildren(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alpha := root.Children[1]
	// Alpha should have: module.json meta + 2 components + 1 impl_section = 4 children
	if len(alpha.Children) != 4 {
		t.Fatalf("alpha children: want 4, got %d", len(alpha.Children))
	}

	// All children should be leaf type (no intermediate group nodes)
	for _, child := range alpha.Children {
		if child.Type != "leaf" {
			t.Fatalf("child %s: want type leaf, got %s", child.Key, child.Type)
		}
	}

	// Children should be sorted by key
	for i := 1; i < len(alpha.Children); i++ {
		if alpha.Children[i].Key < alpha.Children[i-1].Key {
			t.Fatalf("children not sorted: %s comes after %s", alpha.Children[i].Key, alpha.Children[i-1].Key)
		}
	}

	// Verify specific keys exist
	wantKeys := map[string]string{
		"module/1/meta":           "meta",
		"module/1/component/1":    "component",
		"module/1/component/2":    "component",
		"module/1/impl_section/1": "impl_section",
	}
	for _, child := range alpha.Children {
		wantType, ok := wantKeys[child.Key]
		if !ok {
			t.Errorf("unexpected child key: %s", child.Key)
			continue
		}
		if child.NodeType != wantType {
			t.Errorf("child %s: want node_type %s, got %s", child.Key, wantType, child.NodeType)
		}
		if child.Module != 1 {
			t.Errorf("child %s: want module 1, got %d", child.Key, child.Module)
		}
		delete(wantKeys, child.Key)
	}
	for key := range wantKeys {
		t.Errorf("missing expected child key: %s", key)
	}
}

func TestREQ7_BuildTree_ModuleID(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// All module 1 leaves should have Module=1
	alpha := root.Children[1]
	for _, child := range alpha.Children {
		if child.Module != 1 {
			t.Errorf("alpha child %s: want module 1, got %d", child.Key, child.Module)
		}
	}

	// Module node itself should have Module=1
	if alpha.Module != 1 {
		t.Errorf("alpha module: want 1, got %d", alpha.Module)
	}

	// Beta children should have Module=2
	beta := root.Children[2]
	for _, child := range beta.Children {
		if child.Module != 2 {
			t.Errorf("beta child %s: want module 2, got %d", child.Key, child.Module)
		}
	}
}

func TestREQ6_BuildTree_Deterministic(t *testing.T) {
	specDir := setupSpecDir(t)

	root1, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	root2, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	if root1.Hash != root2.Hash {
		t.Fatalf("determinism: hash1=%s hash2=%s", root1.Hash, root2.Hash)
	}
}

func TestREQ2_BuildTree_HashChangesOnFileEdit(t *testing.T) {
	specDir := setupSpecDir(t)

	root1, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	// Modify a content file
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Updated Comp1\n")

	root2, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	if root1.Hash == root2.Hash {
		t.Fatal("root hash should change when a leaf file changes")
	}

	// Module alpha hash should differ
	if root1.Children[1].Hash == root2.Children[1].Hash {
		t.Fatal("alpha module hash should change")
	}

	// Module beta hash should be unchanged
	if root1.Children[2].Hash != root2.Children[2].Hash {
		t.Fatal("beta module hash should not change")
	}
}

func TestREQ2_BuildTree_MissingContentFile(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "bad-project",
		"modules": [{"id": 1, "name": "Bad", "path": "bad"}]
	}`
	writeFile(t, dir, "project.json", proj)

	badDir := filepath.Join(dir, "bad")
	must(t, os.MkdirAll(badDir, 0755))
	badMod := `{
		"name": "bad",
		"components": [
			{"id": 1, "name": "Ghost", "content": "arch_ghost.md"}
		]
	}`
	writeFile(t, badDir, "module.json", badMod)
	// arch_ghost.md does NOT exist

	_, err := BuildTree(dir)
	if err == nil {
		t.Fatal("want error for missing content file, got nil")
	}
	if !strings.Contains(err.Error(), "module/1/component/1") {
		t.Fatalf("error should mention spec key, got: %v", err)
	}
}

func TestREQ2_BuildTree_MissingProjectJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := BuildTree(dir)
	if err == nil {
		t.Fatal("want error for missing project.json, got nil")
	}
	if !strings.Contains(err.Error(), "project.json") {
		t.Fatalf("error should mention project.json, got: %v", err)
	}
}

func TestREQ2_BuildTree_MissingModuleJSON(t *testing.T) {
	dir := t.TempDir()
	proj := `{
		"name": "no-module",
		"modules": [{"id": 1, "name": "Ghost", "path": "ghost"}]
	}`
	writeFile(t, dir, "project.json", proj)
	must(t, os.MkdirAll(filepath.Join(dir, "ghost"), 0755))
	// module.json does NOT exist

	_, err := BuildTree(dir)
	if err == nil {
		t.Fatal("want error for missing module.json, got nil")
	}
	if !strings.Contains(err.Error(), "module.json") {
		t.Fatalf("error should mention module.json, got: %v", err)
	}
}

func TestREQ2_BuildTree_AllNodesHaveHashes(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n.Hash == "" {
			t.Fatalf("node %q (type=%s) has empty hash", n.Key, n.Type)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
}

func TestREQ2_BuildTree_JSONRoundTrip(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	data, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Hash != root.Hash {
		t.Fatalf("round-trip hash mismatch: want %s, got %s", root.Hash, decoded.Hash)
	}
	if decoded.Key != root.Key {
		t.Fatalf("round-trip key mismatch: want %s, got %s", root.Key, decoded.Key)
	}
}

func TestREQ7_BuildTree_WithAllNodeTypes(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "full-project",
		"modules": [{"id": 1, "name": "FullMod", "path": "fullmod"}]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "fullmod")
	must(t, os.MkdirAll(modDir, 0755))
	modJSON := `{
		"name": "fullmod",
		"components": [
			{"id": 1, "name": "C1", "content": "arch_c1.md"}
		],
		"impl_sections": [
			{"id": 1, "name": "I1", "content": "impl_c1.md"}
		],
		"data_flows": [
			{"id": 1, "name": "F1", "content": "flow_c1.md"}
		]
	}`
	writeFile(t, modDir, "module.json", modJSON)
	writeFile(t, modDir, "arch_c1.md", "# arch\n")
	writeFile(t, modDir, "impl_c1.md", "# impl\n")
	writeFile(t, modDir, "flow_c1.md", "# flow\n")

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	fullMod := root.Children[1]
	// meta + 1 component + 1 impl_section + 1 data_flow = 4 children
	if len(fullMod.Children) != 4 {
		t.Fatalf("fullmod children: want 4, got %d", len(fullMod.Children))
	}

	wantKeys := map[string]string{
		"module/1/meta":           "meta",
		"module/1/component/1":    "component",
		"module/1/impl_section/1": "impl_section",
		"module/1/data_flow/1":    "data_flow",
	}
	for _, child := range fullMod.Children {
		wantType, ok := wantKeys[child.Key]
		if !ok {
			t.Errorf("unexpected child key: %s", child.Key)
			continue
		}
		if child.NodeType != wantType {
			t.Errorf("child %s: want node_type %s, got %s", child.Key, wantType, child.NodeType)
		}
		delete(wantKeys, child.Key)
	}
	for key := range wantKeys {
		t.Errorf("missing expected child key: %s", key)
	}
}

func TestREQ2_BuildTree_EmptyModule(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "empty-project",
		"modules": [{"id": 1, "name": "Empty", "path": "empty"}]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "empty")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "empty"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	emptyMod := root.Children[1]
	// Only module meta leaf
	if len(emptyMod.Children) != 1 {
		t.Fatalf("empty module children: want 1, got %d", len(emptyMod.Children))
	}
	if emptyMod.Children[0].Key != "module/1/meta" {
		t.Fatalf("empty module child: want module/1/meta, got %s", emptyMod.Children[0].Key)
	}
}
