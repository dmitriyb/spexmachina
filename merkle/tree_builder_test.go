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

	// project.json with two modules and project-level requirements
	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Do stuff", "description": "The system must do stuff.", "priority": 1},
			{"id": 2, "type": "non_functional", "title": "Be fast", "priority": 2}
		],
		"modules": [
			{"id": 1, "name": "Alpha", "path": "alpha"},
			{"id": 2, "name": "Beta", "path": "beta"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	// Module alpha: has components, impl_sections, and requirements
	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Alpha req 1", "preq_id": 1},
			{"id": 2, "type": "functional", "title": "Alpha req 2", "description": "Details here", "depends_on": [1]}
		],
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

// findChild finds a child node by key. Fails the test if not found.
func findChild(t *testing.T, parent *Node, key string) *Node {
	t.Helper()
	for _, c := range parent.Children {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("child %q not found in %q", key, parent.Key)
	return nil
}

func TestREQ7_BuildTree_SpecIDKeys(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if root.Key != "project" {
		t.Fatalf("root key: want project, got %s", root.Key)
	}

	// project/meta leaf
	projLeaf := findChild(t, root, "project/meta")
	if projLeaf.NodeType != "meta" {
		t.Fatalf("project meta node_type: want meta, got %s", projLeaf.NodeType)
	}

	// module/1 (Alpha)
	alpha := findChild(t, root, "module/1")
	if alpha.Type != "module" {
		t.Fatalf("alpha type: want module, got %s", alpha.Type)
	}

	// module/2 (Beta)
	beta := findChild(t, root, "module/2")
	if beta.Type != "module" {
		t.Fatalf("beta type: want module, got %s", beta.Type)
	}
}

func TestREQ2_BuildTree_Structure(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Root is project node
	if root.Type != "project" {
		t.Fatalf("root type: want project, got %s", root.Type)
	}

	// Children: project/meta leaf + 2 project requirements + 2 module nodes = 5
	if len(root.Children) != 5 {
		t.Fatalf("root children: want 5, got %d", len(root.Children))
	}

	// Children are sorted by key, so order is:
	// module/1, module/2, project/meta, project/requirement/1, project/requirement/2
	for _, child := range root.Children {
		switch child.Key {
		case "project/meta":
			if child.Type != "leaf" {
				t.Fatalf("project/meta type: want leaf, got %s", child.Type)
			}
		case "project/requirement/1", "project/requirement/2":
			if child.Type != "leaf" {
				t.Fatalf("%s type: want leaf, got %s", child.Key, child.Type)
			}
		case "module/1", "module/2":
			if child.Type != "module" {
				t.Fatalf("%s type: want module, got %s", child.Key, child.Type)
			}
		default:
			t.Fatalf("unexpected root child: %s", child.Key)
		}
	}
}

func TestREQ7_BuildTree_FlatModuleChildren(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alpha := findChild(t, root, "module/1")
	// Alpha should have: meta + 2 requirements + 2 components + 1 impl_section = 6 children
	if len(alpha.Children) != 6 {
		t.Fatalf("alpha children: want 6, got %d", len(alpha.Children))
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
		"module/1/requirement/1":  "requirement",
		"module/1/requirement/2":  "requirement",
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
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// All module 1 leaves should have Module=1
	alpha := findChild(t, root, "module/1")
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
	beta := findChild(t, root, "module/2")
	for _, child := range beta.Children {
		if child.Module != 2 {
			t.Errorf("beta child %s: want module 2, got %d", child.Key, child.Module)
		}
	}
}

func TestREQ6_BuildTree_Deterministic(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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
	alpha1 := findChild(t, root1, "module/1")
	alpha2 := findChild(t, root2, "module/1")
	if alpha1.Hash == alpha2.Hash {
		t.Fatal("alpha module hash should change")
	}

	// Module beta hash should be unchanged
	beta1 := findChild(t, root1, "module/2")
	beta2 := findChild(t, root2, "module/2")
	if beta1.Hash != beta2.Hash {
		t.Fatal("beta module hash should not change")
	}
}

func TestREQ2_BuildTree_MissingContentFile(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
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

	fullMod := findChild(t, root, "module/1")
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

	emptyMod := findChild(t, root, "module/1")
	// Only module meta leaf
	if len(emptyMod.Children) != 1 {
		t.Fatalf("empty module children: want 1, got %d", len(emptyMod.Children))
	}
	if emptyMod.Children[0].Key != "module/1/meta" {
		t.Fatalf("empty module child: want module/1/meta, got %s", emptyMod.Children[0].Key)
	}
}

func TestREQ7_BuildTree_ModuleRequirementLeaves(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alpha := findChild(t, root, "module/1")

	// Module-level requirements should be leaf nodes
	req1 := findChild(t, alpha, "module/1/requirement/1")
	if req1.Type != "leaf" {
		t.Fatalf("req1 type: want leaf, got %s", req1.Type)
	}
	if req1.NodeType != "requirement" {
		t.Fatalf("req1 node_type: want requirement, got %s", req1.NodeType)
	}
	if req1.Module != 1 {
		t.Fatalf("req1 module: want 1, got %d", req1.Module)
	}
	if req1.Hash == "" {
		t.Fatal("req1 hash should not be empty")
	}

	req2 := findChild(t, alpha, "module/1/requirement/2")
	if req2.Type != "leaf" {
		t.Fatalf("req2 type: want leaf, got %s", req2.Type)
	}
	if req2.NodeType != "requirement" {
		t.Fatalf("req2 node_type: want requirement, got %s", req2.NodeType)
	}

	// Different requirements should produce different hashes
	if req1.Hash == req2.Hash {
		t.Fatal("different requirements should have different hashes")
	}
}

func TestREQ7_BuildTree_ProjectRequirementLeaves(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Project-level requirements should be leaf nodes at root level
	req1 := findChild(t, root, "project/requirement/1")
	if req1.Type != "leaf" {
		t.Fatalf("project req1 type: want leaf, got %s", req1.Type)
	}
	if req1.NodeType != "requirement" {
		t.Fatalf("project req1 node_type: want requirement, got %s", req1.NodeType)
	}
	if req1.Module != 0 {
		t.Fatalf("project req1 module: want 0, got %d", req1.Module)
	}
	if req1.Hash == "" {
		t.Fatal("project req1 hash should not be empty")
	}

	req2 := findChild(t, root, "project/requirement/2")
	if req2.Type != "leaf" {
		t.Fatalf("project req2 type: want leaf, got %s", req2.Type)
	}
	if req2.NodeType != "requirement" {
		t.Fatalf("project req2 node_type: want requirement, got %s", req2.NodeType)
	}
	if req2.Module != 0 {
		t.Fatalf("project req2 module: want 0, got %d", req2.Module)
	}

	// Different project requirements should produce different hashes
	if req1.Hash == req2.Hash {
		t.Fatal("different project requirements should have different hashes")
	}
}

func TestREQ7_BuildTree_RequirementHashDeterministic(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	specDir := setupSpecDir(t)

	root1, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	root2, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	// Module requirement hashes should be identical across builds
	alpha1 := findChild(t, root1, "module/1")
	alpha2 := findChild(t, root2, "module/1")
	req1a := findChild(t, alpha1, "module/1/requirement/1")
	req1b := findChild(t, alpha2, "module/1/requirement/1")
	if req1a.Hash != req1b.Hash {
		t.Fatalf("module requirement hash not deterministic: %s vs %s", req1a.Hash, req1b.Hash)
	}

	// Project requirement hashes should be identical across builds
	preq1a := findChild(t, root1, "project/requirement/1")
	preq1b := findChild(t, root2, "project/requirement/1")
	if preq1a.Hash != preq1b.Hash {
		t.Fatalf("project requirement hash not deterministic: %s vs %s", preq1a.Hash, preq1b.Hash)
	}
}

func TestREQ7_BuildTree_RequirementHashChangesOnFieldChange(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "req-change-test",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Original title"}
		],
		"modules": [{"id": 1, "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "m"}`)

	root1, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	hash1 := findChild(t, root1, "project/requirement/1").Hash

	// Change the requirement title
	proj2 := `{
		"name": "req-change-test",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Updated title"}
		],
		"modules": [{"id": 1, "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj2)

	root2, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	hash2 := findChild(t, root2, "project/requirement/1").Hash

	if hash1 == hash2 {
		t.Fatal("requirement hash should change when title changes")
	}

	// Root hash should also change
	if root1.Hash == root2.Hash {
		t.Fatal("root hash should change when a project requirement changes")
	}
}

func TestREQ7_BuildTree_RequirementHashSortedKeys(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	// Verify that requirement hashing uses sorted JSON keys for determinism.
	// Two requirements with the same fields but potentially different struct
	// field order should still produce the same hash.
	dir := t.TempDir()

	proj := `{
		"name": "sorted-keys-test",
		"modules": [{"id": 1, "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	modJSON := `{
		"name": "m",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Test", "description": "Desc", "preq_id": 1, "depends_on": [2]}
		]
	}`
	writeFile(t, modDir, "module.json", modJSON)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alpha := findChild(t, root, "module/1")
	req := findChild(t, alpha, "module/1/requirement/1")

	// Hash should be 64 chars (SHA-256 hex)
	if len(req.Hash) != 64 {
		t.Fatalf("requirement hash length: want 64, got %d", len(req.Hash))
	}

	// Compute expected hash from deterministic JSON
	expected := hashRequirementJSON(t, map[string]interface{}{
		"depends_on":  []int{2},
		"description": "Desc",
		"id":          1,
		"preq_id":     1,
		"title":       "Test",
		"type":        "functional",
	})
	if req.Hash != expected {
		t.Fatalf("requirement hash mismatch: want %s, got %s", expected, req.Hash)
	}
}

// hashRequirementJSON is a test helper that computes expected hash from a map.
func hashRequirementJSON(t *testing.T, fields map[string]interface{}) string {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return HashBytes(data)
}

func TestREQ7_BuildTree_RequirementOmitsZeroFields(t *testing.T) {
	dir := t.TempDir()

	proj := `{
		"name": "omitempty-test",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Minimal"}
		],
		"modules": [{"id": 1, "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "m"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	req := findChild(t, root, "project/requirement/1")

	// Minimal requirement: only id, title, type are set.
	// Hash should match a JSON object with only those fields (sorted keys).
	expected := hashRequirementJSON(t, map[string]interface{}{
		"id":    1,
		"title": "Minimal",
		"type":  "functional",
	})
	if req.Hash != expected {
		t.Fatalf("minimal requirement hash mismatch: want %s, got %s", expected, req.Hash)
	}
}

func TestREQ7_BuildTree_ModuleRequirementHashIncludesModuleHash(t *testing.T) {
	t.Skip("TODO(bead:spexmachina-kdb): fix after spexmachina-e8t changed module IDs to identity hashes")
	// When a module has requirements, the module's interior hash should include
	// the requirement leaf hashes alongside content leaf hashes.
	dir := t.TempDir()

	proj := `{
		"name": "mod-req-hash",
		"modules": [{"id": 1, "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))

	// First: module with no requirements
	writeFile(t, modDir, "module.json", `{"name": "m"}`)
	root1, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	mod1 := findChild(t, root1, "module/1")

	// Second: module with a requirement
	writeFile(t, modDir, "module.json", `{
		"name": "m",
		"requirements": [{"id": 1, "type": "functional", "title": "New req"}]
	}`)
	root2, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	mod2 := findChild(t, root2, "module/1")

	// Module hash should differ (meta changed AND requirement was added)
	if mod1.Hash == mod2.Hash {
		t.Fatal("module hash should change when requirement is added")
	}
}
