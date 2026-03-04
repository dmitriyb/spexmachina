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

	// Module alpha: has arch and impl content
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

	// Module beta: has only arch content
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
	if root.Name != "test-project" {
		t.Fatalf("root name: want test-project, got %s", root.Name)
	}

	// Children: project.json leaf + 2 module nodes
	if len(root.Children) != 3 {
		t.Fatalf("root children: want 3, got %d", len(root.Children))
	}

	projLeaf := root.Children[0]
	if projLeaf.Type != "leaf" || projLeaf.Name != "project.json" {
		t.Fatalf("first child: want leaf/project.json, got %s/%s", projLeaf.Type, projLeaf.Name)
	}

	alpha := root.Children[1]
	if alpha.Type != "module" || alpha.Name != "Alpha" {
		t.Fatalf("second child: want module/Alpha, got %s/%s", alpha.Type, alpha.Name)
	}

	beta := root.Children[2]
	if beta.Type != "module" || beta.Name != "Beta" {
		t.Fatalf("third child: want module/Beta, got %s/%s", beta.Type, beta.Name)
	}
}

func TestREQ2_BuildTree_ModuleStructure(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alpha := root.Children[1]
	// Alpha should have: module.json leaf, arch group, impl group
	if len(alpha.Children) != 3 {
		t.Fatalf("alpha children: want 3, got %d", len(alpha.Children))
	}

	modLeaf := alpha.Children[0]
	if modLeaf.Type != "leaf" || modLeaf.Name != "module.json" {
		t.Fatalf("module leaf: want leaf/module.json, got %s/%s", modLeaf.Type, modLeaf.Name)
	}

	archGroup := alpha.Children[1]
	if archGroup.Type != "arch" {
		t.Fatalf("arch group type: want arch, got %s", archGroup.Type)
	}
	if len(archGroup.Children) != 2 {
		t.Fatalf("arch leaves: want 2, got %d", len(archGroup.Children))
	}

	implGroup := alpha.Children[2]
	if implGroup.Type != "impl" {
		t.Fatalf("impl group type: want impl, got %s", implGroup.Type)
	}
	if len(implGroup.Children) != 1 {
		t.Fatalf("impl leaves: want 1, got %d", len(implGroup.Children))
	}
}

func TestREQ2_BuildTree_NoFlowGroup(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Beta has no impl or flow content, only arch
	beta := root.Children[2]
	// Should have: module.json leaf + arch group (no impl, no flow)
	if len(beta.Children) != 2 {
		t.Fatalf("beta children: want 2, got %d", len(beta.Children))
	}

	for _, child := range beta.Children {
		if child.Type == "impl" || child.Type == "flow" {
			t.Fatalf("beta should not have %s group", child.Type)
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
	if !strings.Contains(err.Error(), "arch_ghost.md") {
		t.Fatalf("error should mention missing file, got: %v", err)
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
			t.Fatalf("node %q (type=%s) has empty hash", n.Name, n.Type)
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
	if decoded.Name != root.Name {
		t.Fatalf("round-trip name mismatch: want %s, got %s", root.Name, decoded.Name)
	}
}

func TestREQ2_BuildTree_WithFlowContent(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "flow-project",
		"modules": [{"id": 1, "name": "FlowMod", "path": "flowmod"}]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "flowmod")
	must(t, os.MkdirAll(modDir, 0755))
	modJSON := `{
		"name": "flowmod",
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

	flowMod := root.Children[1]
	// module.json + arch + impl + flow = 4 children
	if len(flowMod.Children) != 4 {
		t.Fatalf("flowmod children: want 4, got %d", len(flowMod.Children))
	}

	flowGroup := flowMod.Children[3]
	if flowGroup.Type != "flow" {
		t.Fatalf("flow group type: want flow, got %s", flowGroup.Type)
	}
	if len(flowGroup.Children) != 1 {
		t.Fatalf("flow leaves: want 1, got %d", len(flowGroup.Children))
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
	// Only module.json leaf, no groups
	if len(emptyMod.Children) != 1 {
		t.Fatalf("empty module children: want 1, got %d", len(emptyMod.Children))
	}
	if emptyMod.Children[0].Name != "module.json" {
		t.Fatalf("empty module child: want module.json, got %s", emptyMod.Children[0].Name)
	}
}
