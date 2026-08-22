package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// Distinct, realistic identity hashes for the shared fixture's project-level
// nodes. Every node's tree key is exactly its id field, so fixture ids must
// be unique across modules and project requirements.
var (
	fixtureProjReq1ID = schema.IdentityHash("project", "requirement", "Do stuff")
	fixtureProjReq2ID = schema.IdentityHash("project", "requirement", "Be fast")
)

// setupSpecDir creates a minimal spec directory for testing. All nodes —
// modules, requirements, and module-level nodes — carry identity hash ids,
// and the TreeBuilder keys tree nodes by those ids directly.
// Returns the spec dir path.
func setupSpecDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Compute identity hashes for module-level nodes.
	alphaComp1 := schema.IdentityHash("alpha", "component", "Comp1")
	alphaComp2 := schema.IdentityHash("alpha", "component", "Comp2")
	alphaReq1 := schema.IdentityHash("alpha", "requirement", "Alpha req 1")
	alphaReq2 := schema.IdentityHash("alpha", "requirement", "Alpha req 2")
	alphaTest1 := schema.IdentityHash("alpha", "test_section", "Test1")
	betaComp := schema.IdentityHash("beta", "component", "BetaComp")
	alphaModID := schema.IdentityHash("module", "Alpha")
	betaModID := schema.IdentityHash("module", "Beta")

	// project.json with two modules and project-level requirements
	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": "` + fixtureProjReq1ID + `", "type": "functional", "title": "Do stuff", "description": "The system must do stuff.", "priority": 1},
			{"id": "` + fixtureProjReq2ID + `", "type": "non_functional", "title": "Be fast", "priority": 2}
		],
		"modules": [
			{"id": "` + alphaModID + `", "name": "Alpha", "path": "alpha"},
			{"id": "` + betaModID + `", "name": "Beta", "path": "beta"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	// Module alpha: has components, test_sections, and requirements
	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": "` + alphaReq1 + `", "type": "functional", "title": "Alpha req 1", "preq_id": "` + fixtureProjReq1ID + `"},
			{"id": "` + alphaReq2 + `", "type": "functional", "title": "Alpha req 2", "description": "Details here", "depends_on": ["` + alphaReq1 + `"]}
		],
		"components": [
			{"id": "` + alphaComp1 + `", "name": "Comp1", "content": "arch_comp1.md"},
			{"id": "` + alphaComp2 + `", "name": "Comp2", "content": "arch_comp2.md"}
		],
		"test_sections": [
			{"id": "` + alphaTest1 + `", "name": "Test1", "content": "test_comp1.md"}
		]
	}`
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeFile(t, alphaDir, "arch_comp2.md", "# Comp2 architecture\n")
	writeFile(t, alphaDir, "test_comp1.md", "# Comp1 tests\n")

	// Module beta: has only one component
	betaDir := filepath.Join(dir, "beta")
	must(t, os.MkdirAll(betaDir, 0755))
	betaMod := `{
		"name": "beta",
		"components": [
			{"id": "` + betaComp + `", "name": "BetaComp", "content": "arch_beta.md"}
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
	t.Fatalf("child %q not found in %q (children: %v)", key, parent.Key, childKeys(parent))
	return nil
}

func childKeys(n *Node) []string {
	keys := make([]string, len(n.Children))
	for i, c := range n.Children {
		keys[i] = c.Key
	}
	return keys
}

func TestREQ7_BuildTree_IdentityHashKeys(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if root.Key != "project" {
		t.Fatalf("root key: want project, got %s", root.Key)
	}

	// meta/project leaf
	projLeaf := findChild(t, root, "meta/project")
	if projLeaf.NodeType != "meta" {
		t.Fatalf("project meta node_type: want meta, got %s", projLeaf.NodeType)
	}

	// Module nodes keyed by identity hash
	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)
	if alpha.Type != "module" {
		t.Fatalf("alpha type: want module, got %s", alpha.Type)
	}

	betaHash := schema.IdentityHash("module", "Beta")
	beta := findChild(t, root, betaHash)
	if beta.Type != "module" {
		t.Fatalf("beta type: want module, got %s", beta.Type)
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

	// Children: meta/project leaf + 2 project requirements + 2 module nodes = 5
	if len(root.Children) != 5 {
		t.Fatalf("root children: want 5, got %d", len(root.Children))
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	betaHash := schema.IdentityHash("module", "Beta")
	projReq1Hash := fixtureProjReq1ID
	projReq2Hash := fixtureProjReq2ID

	for _, child := range root.Children {
		switch child.Key {
		case "meta/project":
			if child.Type != "leaf" {
				t.Fatalf("meta/project type: want leaf, got %s", child.Type)
			}
		case projReq1Hash, projReq2Hash:
			if child.Type != "leaf" {
				t.Fatalf("%s type: want leaf, got %s", child.Key, child.Type)
			}
		case alphaHash, betaHash:
			if child.Type != "module" {
				t.Fatalf("%s type: want module, got %s", child.Key, child.Type)
			}
		default:
			t.Fatalf("unexpected root child: %s", child.Key)
		}
	}
}

func TestREQ7_BuildTree_FlatModuleChildren(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)
	// Alpha should have: meta + 2 requirements + 2 components + 1 test_section = 6 children
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

	// Verify specific keys exist (identity hash keys)
	wantKeys := map[string]string{
		"meta/" + alphaHash:                                       "meta",
		schema.IdentityHash("alpha", "component", "Comp1"):        "component",
		schema.IdentityHash("alpha", "component", "Comp2"):        "component",
		schema.IdentityHash("alpha", "test_section", "Test1"):     "test_section",
		schema.IdentityHash("alpha", "requirement", "Alpha req 1"): "requirement",
		schema.IdentityHash("alpha", "requirement", "Alpha req 2"): "requirement",
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
		if child.Module != alphaHash {
			t.Errorf("child %s: want module %s, got %s", child.Key, alphaHash, child.Module)
		}
		delete(wantKeys, child.Key)
	}
	for key := range wantKeys {
		t.Errorf("missing expected child key: %s", key)
	}
}

func TestREQ7_BuildTree_ModuleIdentityHash(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	betaHash := schema.IdentityHash("module", "Beta")

	// All alpha leaves should have Module = alphaHash
	alpha := findChild(t, root, alphaHash)
	for _, child := range alpha.Children {
		if child.Module != alphaHash {
			t.Errorf("alpha child %s: want module %s, got %s", child.Key, alphaHash, child.Module)
		}
	}

	// Module node itself should have Module = alphaHash
	if alpha.Module != alphaHash {
		t.Errorf("alpha module: want %s, got %s", alphaHash, alpha.Module)
	}

	// Beta children should have Module = betaHash
	beta := findChild(t, root, betaHash)
	for _, child := range beta.Children {
		if child.Module != betaHash {
			t.Errorf("beta child %s: want module %s, got %s", child.Key, betaHash, child.Module)
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

// collectHashesByKey walks a tree and returns a map from every node's key to
// its hash, so two trees can be compared node-by-node regardless of child
// ordering.
func collectHashesByKey(n *Node, out map[string]string) {
	out[n.Key] = n.Hash
	for _, c := range n.Children {
		collectHashesByKey(c, out)
	}
}

// TestS7_BuildTree_DeterministicAcrossSeparateDirs covers test_hashing.md S7:
// the fixture directory built twice in independent temp directories with
// identical file contents produces equal root hashes and equal hashes at
// every corresponding key — not merely a stable hash from rebuilding the same
// directory, which TestREQ6_BuildTree_Deterministic already covers.
func TestS7_BuildTree_DeterministicAcrossSeparateDirs(t *testing.T) {
	dirA := setupSpecDir(t)
	dirB := setupSpecDir(t)
	if dirA == dirB {
		t.Fatalf("fixture dirs must be distinct temp directories, both got %s", dirA)
	}

	rootA, err := BuildTree(dirA)
	if err != nil {
		t.Fatalf("BuildTree(dirA): %v", err)
	}
	rootB, err := BuildTree(dirB)
	if err != nil {
		t.Fatalf("BuildTree(dirB): %v", err)
	}

	if rootA.Hash != rootB.Hash {
		t.Fatalf("root hash: dirA=%s dirB=%s", rootA.Hash, rootB.Hash)
	}

	hashesA := map[string]string{}
	hashesB := map[string]string{}
	collectHashesByKey(rootA, hashesA)
	collectHashesByKey(rootB, hashesB)

	if len(hashesA) != len(hashesB) {
		t.Fatalf("node count: dirA=%d dirB=%d", len(hashesA), len(hashesB))
	}
	for key, hashA := range hashesA {
		hashB, ok := hashesB[key]
		if !ok {
			t.Fatalf("key %s present in dirA tree but not dirB tree", key)
		}
		if hashA != hashB {
			t.Fatalf("key %s: dirA=%s dirB=%s", key, hashA, hashB)
		}
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

	alphaHash := schema.IdentityHash("module", "Alpha")
	betaHash := schema.IdentityHash("module", "Beta")

	// Module alpha hash should differ
	alpha1 := findChild(t, root1, alphaHash)
	alpha2 := findChild(t, root2, alphaHash)
	if alpha1.Hash == alpha2.Hash {
		t.Fatal("alpha module hash should change")
	}

	// Module beta hash should be unchanged
	beta1 := findChild(t, root1, betaHash)
	beta2 := findChild(t, root2, betaHash)
	if beta1.Hash != beta2.Hash {
		t.Fatal("beta module hash should not change")
	}
}

func TestREQ2_BuildTree_MissingContentFile(t *testing.T) {
	dir := t.TempDir()

	ghostComp := schema.IdentityHash("bad", "component", "Ghost")
	proj := `{
		"name": "bad-project",
		"modules": [{"id": "` + schema.IdentityHash("module", "Bad") + `", "name": "Bad", "path": "bad"}]
	}`
	writeFile(t, dir, "project.json", proj)

	badDir := filepath.Join(dir, "bad")
	must(t, os.MkdirAll(badDir, 0755))
	badMod := `{
		"name": "bad",
		"components": [
			{"id": "` + ghostComp + `", "name": "Ghost", "content": "arch_ghost.md"}
		]
	}`
	writeFile(t, badDir, "module.json", badMod)
	// arch_ghost.md does NOT exist

	root, err := BuildTree(dir)
	if err == nil {
		t.Fatal("want error for missing content file, got nil")
	}
	if !strings.Contains(err.Error(), ghostComp) {
		t.Fatalf("error should mention spec key %s, got: %v", ghostComp, err)
	}
	if root != nil {
		t.Fatalf("want no partial tree on error, got: %+v", root)
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
		"modules": [{"id": "` + schema.IdentityHash("module", "Ghost") + `", "name": "Ghost", "path": "ghost"}]
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

// TestS6_BuildTree_BottomUpPropagation verifies test_hashing.md scenario S6:
// leaf hashes match HashFile of their backing files, and interior hashes
// equal HashChildren over their children's hashes — computed independently
// here, not merely re-read from the tree, so a wrong composition (unsorted
// concatenation, extra folded-in bytes) would fail this test.
func TestS6_BuildTree_BottomUpPropagation(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// 1. Each file-backed leaf's hash matches HashFile of its file.
	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)

	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")
	comp1 := findChild(t, alpha, comp1Key)
	wantComp1Hash, err := HashFile(filepath.Join(specDir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatalf("HashFile comp1: %v", err)
	}
	if comp1.Hash != wantComp1Hash {
		t.Fatalf("comp1 leaf hash: want %s, got %s", wantComp1Hash, comp1.Hash)
	}

	comp2Key := schema.IdentityHash("alpha", "component", "Comp2")
	comp2 := findChild(t, alpha, comp2Key)
	wantComp2Hash, err := HashFile(filepath.Join(specDir, "alpha", "arch_comp2.md"))
	if err != nil {
		t.Fatalf("HashFile comp2: %v", err)
	}
	if comp2.Hash != wantComp2Hash {
		t.Fatalf("comp2 leaf hash: want %s, got %s", wantComp2Hash, comp2.Hash)
	}

	test1Key := schema.IdentityHash("alpha", "test_section", "Test1")
	test1 := findChild(t, alpha, test1Key)
	wantTest1Hash, err := HashFile(filepath.Join(specDir, "alpha", "test_comp1.md"))
	if err != nil {
		t.Fatalf("HashFile test1: %v", err)
	}
	if test1.Hash != wantTest1Hash {
		t.Fatalf("test1 leaf hash: want %s, got %s", wantTest1Hash, test1.Hash)
	}

	metaKey := "meta/" + alphaHash
	metaLeaf := findChild(t, alpha, metaKey)
	wantMetaHash, err := HashFile(filepath.Join(specDir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("HashFile alpha module.json: %v", err)
	}
	if metaLeaf.Hash != wantMetaHash {
		t.Fatalf("alpha meta leaf hash: want %s, got %s", wantMetaHash, metaLeaf.Hash)
	}

	// 2. The alpha module interior hash equals HashChildren over the meta
	// envelope leaf hash plus each spec-node leaf hash directly — no
	// intermediate group hashes. Collected and sorted independently from
	// the actual child nodes, not assumed to already be in the right order.
	alphaChildHashes := make([]string, len(alpha.Children))
	for i, c := range alpha.Children {
		alphaChildHashes[i] = c.Hash
	}
	sort.Strings(alphaChildHashes)
	if len(alphaChildHashes) != 6 {
		t.Fatalf("alpha children: want 6, got %d", len(alphaChildHashes))
	}
	wantAlphaHash := HashChildren(alphaChildHashes)
	if alpha.Hash != wantAlphaHash {
		t.Fatalf("alpha module hash: want %s, got %s", wantAlphaHash, alpha.Hash)
	}

	// 3. The root hash equals HashChildren over its five children's hashes —
	// the project envelope leaf, the two project requirement leaves, and
	// the alpha and beta module hashes.
	if len(root.Children) != 5 {
		t.Fatalf("root children: want 5, got %d", len(root.Children))
	}
	rootChildHashes := make([]string, len(root.Children))
	for i, c := range root.Children {
		rootChildHashes[i] = c.Hash
	}
	sort.Strings(rootChildHashes)
	wantRootHash := HashChildren(rootChildHashes)
	if root.Hash != wantRootHash {
		t.Fatalf("root hash: want %s, got %s", wantRootHash, root.Hash)
	}
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

	comp1 := schema.IdentityHash("fullmod", "component", "C1")
	flow1 := schema.IdentityHash("fullmod", "data_flow", "F1")
	test1 := schema.IdentityHash("fullmod", "test_section", "T1")
	api1 := schema.IdentityHash("fullmod", "api", "spex full")

	proj := `{
		"name": "full-project",
		"modules": [{"id": "` + schema.IdentityHash("module", "FullMod") + `", "name": "FullMod", "path": "fullmod"}]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "fullmod")
	must(t, os.MkdirAll(modDir, 0755))
	modJSON := `{
		"name": "fullmod",
		"components": [
			{"id": "` + comp1 + `", "name": "C1", "content": "arch_c1.md"}
		],
		"data_flows": [
			{"id": "` + flow1 + `", "name": "F1", "content": "flow_c1.md"}
		],
		"test_sections": [
			{"id": "` + test1 + `", "name": "T1", "content": "test_c1.md"}
		],
		"apis": [
			{"id": "` + api1 + `", "name": "spex full", "provided_by": ["` + comp1 + `"], "group": "cli"}
		]
	}`
	writeFile(t, modDir, "module.json", modJSON)
	writeFile(t, modDir, "arch_c1.md", "# arch\n")
	writeFile(t, modDir, "flow_c1.md", "# flow\n")
	writeFile(t, modDir, "test_c1.md", "# test\n")

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	fullModHash := schema.IdentityHash("module", "FullMod")
	fullMod := findChild(t, root, fullModHash)
	// meta + 1 component + 1 data_flow + 1 test_section + 1 api = 5 children
	if len(fullMod.Children) != 5 {
		t.Fatalf("fullmod children: want 5, got %d", len(fullMod.Children))
	}

	wantKeys := map[string]string{
		"meta/" + fullModHash: "meta",
		comp1:                 "component",
		flow1:                 "data_flow",
		test1:                 "test_section",
		api1:                  "api",
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
		// Every module child — the file-backed leaves and the file-less
		// requirement/api leaves alike — must carry Type "leaf".
		// ingest.collectLeafHashes early-returns on Type == "leaf", so a
		// child with any other Type is silently dropped from the leaf-hash
		// map backing the refresh orphan gate and spec_hash updates.
		if child.Type != "leaf" {
			t.Errorf("child %s: want type leaf, got %s", child.Key, child.Type)
		}
		// Every module child must be attributed to its parent module. The
		// file-less api leaf is the fragile one: hashLeaf takes module as a
		// parameter for the file-backed types, but hashAPI sets Module from
		// its own argument, so the field can be dropped there alone. Diff
		// copies Change.Module straight through and merkle.resolveModule
		// passes "" along untouched, so an unattributed api surfaces in
		// `diff --json` as "module": "" and every consumer that groups or
		// keys by module reattributes it as a project-level change.
		if child.Module != fullModHash {
			t.Errorf("child %s: want module %s, got %q", child.Key, fullModHash, child.Module)
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
		"modules": [{"id": "` + schema.IdentityHash("module", "Empty") + `", "name": "Empty", "path": "empty"}]
	}`
	writeFile(t, dir, "project.json", proj)

	modDir := filepath.Join(dir, "empty")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "empty"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	emptyHash := schema.IdentityHash("module", "Empty")
	emptyMod := findChild(t, root, emptyHash)
	// Only module meta leaf
	if len(emptyMod.Children) != 1 {
		t.Fatalf("empty module children: want 1, got %d", len(emptyMod.Children))
	}
	if emptyMod.Children[0].Key != "meta/"+emptyHash {
		t.Fatalf("empty module child: want meta/%s, got %s", emptyHash, emptyMod.Children[0].Key)
	}
}

func TestREQ7_BuildTree_ModuleRequirementLeaves(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)

	req1Key := schema.IdentityHash("alpha", "requirement", "Alpha req 1")
	req2Key := schema.IdentityHash("alpha", "requirement", "Alpha req 2")

	// Module-level requirements should be leaf nodes
	req1 := findChild(t, alpha, req1Key)
	if req1.Type != "leaf" {
		t.Fatalf("req1 type: want leaf, got %s", req1.Type)
	}
	if req1.NodeType != "requirement" {
		t.Fatalf("req1 node_type: want requirement, got %s", req1.NodeType)
	}
	if req1.Module != alphaHash {
		t.Fatalf("req1 module: want %s, got %s", alphaHash, req1.Module)
	}
	if req1.Hash == "" {
		t.Fatal("req1 hash should not be empty")
	}

	req2 := findChild(t, alpha, req2Key)
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
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	req1Key := fixtureProjReq1ID
	req2Key := fixtureProjReq2ID

	// Project-level requirements should be leaf nodes at root level
	req1 := findChild(t, root, req1Key)
	if req1.Type != "leaf" {
		t.Fatalf("project req1 type: want leaf, got %s", req1.Type)
	}
	if req1.NodeType != "requirement" {
		t.Fatalf("project req1 node_type: want requirement, got %s", req1.NodeType)
	}
	if req1.Module != "" {
		t.Fatalf("project req1 module: want empty, got %q", req1.Module)
	}
	if req1.Hash == "" {
		t.Fatal("project req1 hash should not be empty")
	}

	req2 := findChild(t, root, req2Key)
	if req2.Type != "leaf" {
		t.Fatalf("project req2 type: want leaf, got %s", req2.Type)
	}
	if req2.NodeType != "requirement" {
		t.Fatalf("project req2 node_type: want requirement, got %s", req2.NodeType)
	}
	if req2.Module != "" {
		t.Fatalf("project req2 module: want empty, got %q", req2.Module)
	}

	// Different project requirements should produce different hashes
	if req1.Hash == req2.Hash {
		t.Fatal("different project requirements should have different hashes")
	}
}

func TestREQ7_BuildTree_RequirementHashDeterministic(t *testing.T) {
	specDir := setupSpecDir(t)

	root1, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	root2, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	req1Key := schema.IdentityHash("alpha", "requirement", "Alpha req 1")

	// Module requirement hashes should be identical across builds
	alpha1 := findChild(t, root1, alphaHash)
	alpha2 := findChild(t, root2, alphaHash)
	req1a := findChild(t, alpha1, req1Key)
	req1b := findChild(t, alpha2, req1Key)
	if req1a.Hash != req1b.Hash {
		t.Fatalf("module requirement hash not deterministic: %s vs %s", req1a.Hash, req1b.Hash)
	}

	// Project requirement hashes should be identical across builds
	projReq1Key := fixtureProjReq1ID
	preq1a := findChild(t, root1, projReq1Key)
	preq1b := findChild(t, root2, projReq1Key)
	if preq1a.Hash != preq1b.Hash {
		t.Fatalf("project requirement hash not deterministic: %s vs %s", preq1a.Hash, preq1b.Hash)
	}
}

func TestREQ7_BuildTree_RequirementHashChangesOnFieldChange(t *testing.T) {
	dir := t.TempDir()

	reqID := schema.IdentityHash("project", "requirement", "Original title")
	modID := schema.IdentityHash("module", "M")

	proj := `{
		"name": "req-change-test",
		"requirements": [
			{"id": "` + reqID + `", "type": "functional", "title": "Original title"}
		],
		"modules": [{"id": "` + modID + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "m"}`)

	root1, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	hash1 := findChild(t, root1, reqID).Hash

	// Change the requirement title (the id stays stable, as in real specs)
	proj2 := `{
		"name": "req-change-test",
		"requirements": [
			{"id": "` + reqID + `", "type": "functional", "title": "Updated title"}
		],
		"modules": [{"id": "` + modID + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj2)

	root2, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	hash2 := findChild(t, root2, reqID).Hash

	if hash1 == hash2 {
		t.Fatal("requirement hash should change when title changes")
	}

	// Root hash should also change
	if root1.Hash == root2.Hash {
		t.Fatal("root hash should change when a project requirement changes")
	}
}

func TestREQ7_BuildTree_RequirementHashSortedKeys(t *testing.T) {
	// Verify that requirement hashing uses sorted JSON keys for determinism.
	dir := t.TempDir()

	reqID := schema.IdentityHash("m", "requirement", "Test")

	proj := `{
		"name": "sorted-keys-test",
		"modules": [{"id": "` + schema.IdentityHash("module", "M") + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	modJSON := `{
		"name": "m",
		"requirements": [
			{"id": "` + reqID + `", "type": "functional", "title": "Test", "description": "Desc", "preq_id": "` + schema.IdentityHash("project", "requirement", "000000000001") + `", "depends_on": ["` + schema.IdentityHash("m", "requirement", "Other") + `"]}
		]
	}`
	writeFile(t, modDir, "module.json", modJSON)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	mHash := schema.IdentityHash("module", "M")
	mod := findChild(t, root, mHash)
	req := findChild(t, mod, reqID)

	// Hash should be 64 chars (SHA-256 hex)
	if len(req.Hash) != 64 {
		t.Fatalf("requirement hash length: want 64, got %d", len(req.Hash))
	}

	// Compute expected hash from deterministic JSON
	expected := hashRequirementJSON(t, map[string]interface{}{
		"depends_on":  []string{schema.IdentityHash("m", "requirement", "Other")},
		"description": "Desc",
		"id":          reqID,
		"preq_id":     schema.IdentityHash("project", "requirement", "000000000001"),
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

	reqID := schema.IdentityHash("project", "requirement", "Minimal")

	proj := `{
		"name": "omitempty-test",
		"requirements": [
			{"id": "` + reqID + `", "type": "functional", "title": "Minimal"}
		],
		"modules": [{"id": "` + schema.IdentityHash("module", "M") + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "m"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	req := findChild(t, root, reqID)

	// Minimal requirement: only id, title, type are set.
	expected := hashRequirementJSON(t, map[string]interface{}{
		"id":    reqID,
		"title": "Minimal",
		"type":  "functional",
	})
	if req.Hash != expected {
		t.Fatalf("minimal requirement hash mismatch: want %s, got %s", expected, req.Hash)
	}
}

// buildAPIModule writes a spec dir whose single module "m" holds exactly the
// given apis JSON array body, and returns the dir and the module identity hash.
func buildAPIModule(t *testing.T, apisJSON string) (string, string) {
	t.Helper()
	dir := t.TempDir()

	mHash := schema.IdentityHash("module", "M")
	proj := `{
		"name": "api-test",
		"modules": [{"id": "` + mHash + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))
	writeFile(t, modDir, "module.json", `{"name": "m", "apis": [`+apisJSON+`]}`)

	return dir, mHash
}

func TestREQ7_BuildTree_APIHashDeterministic(t *testing.T) {
	apiID := schema.IdentityHash("m", "api", "spex diff")
	dir, mHash := buildAPIModule(t, `{"id": "`+apiID+`", "name": "spex diff", "description": "Compute changes", "group": "cli"}`)

	root1, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	root2, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	// API hashes should be identical across builds.
	api1 := findChild(t, findChild(t, root1, mHash), apiID)
	api2 := findChild(t, findChild(t, root2, mHash), apiID)
	if api1.Hash != api2.Hash {
		t.Fatalf("api hash not deterministic: %s vs %s", api1.Hash, api2.Hash)
	}
}

func TestREQ7_BuildTree_APIHashSortedKeys(t *testing.T) {
	// Verify that api hashing uses sorted JSON keys for determinism.
	apiID := schema.IdentityHash("m", "api", "spex diff")
	compID := schema.IdentityHash("m", "component", "DiffEngine")
	dir, mHash := buildAPIModule(t, `{"id": "`+apiID+`", "name": "spex diff", "description": "Desc", "provided_by": ["`+compID+`"], "group": "cli"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	api := findChild(t, findChild(t, root, mHash), apiID)

	// Hash should be 64 chars (SHA-256 hex)
	if len(api.Hash) != 64 {
		t.Fatalf("api hash length: want 64, got %d", len(api.Hash))
	}

	// Compute expected hash from deterministic JSON
	expected := hashRequirementJSON(t, map[string]interface{}{
		"description": "Desc",
		"group":       "cli",
		"id":          apiID,
		"name":        "spex diff",
		"provided_by": []string{compID},
	})
	if api.Hash != expected {
		t.Fatalf("api hash mismatch: want %s, got %s", expected, api.Hash)
	}
}

func TestREQ7_BuildTree_APIOmitsZeroFields(t *testing.T) {
	apiID := schema.IdentityHash("m", "api", "spex diff")
	dir, mHash := buildAPIModule(t, `{"id": "`+apiID+`", "name": "spex diff"}`)

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	api := findChild(t, findChild(t, root, mHash), apiID)

	// Minimal api: only id and name are set.
	expected := hashRequirementJSON(t, map[string]interface{}{
		"id":   apiID,
		"name": "spex diff",
	})
	if api.Hash != expected {
		t.Fatalf("minimal api hash mismatch: want %s, got %s", expected, api.Hash)
	}
}

func TestREQ7_BuildTree_ModuleRequirementHashIncludesModuleHash(t *testing.T) {
	// When a module has requirements, the module's interior hash should include
	// the requirement leaf hashes alongside content leaf hashes.
	dir := t.TempDir()

	proj := `{
		"name": "mod-req-hash",
		"modules": [{"id": "` + schema.IdentityHash("module", "M") + `", "name": "M", "path": "m"}]
	}`
	writeFile(t, dir, "project.json", proj)
	modDir := filepath.Join(dir, "m")
	must(t, os.MkdirAll(modDir, 0755))

	mHash := schema.IdentityHash("module", "M")

	// First: module with no requirements
	writeFile(t, modDir, "module.json", `{"name": "m"}`)
	root1, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	mod1 := findChild(t, root1, mHash)

	// Second: module with a requirement
	reqID := schema.IdentityHash("m", "requirement", "New req")
	writeFile(t, modDir, "module.json", `{
		"name": "m",
		"requirements": [{"id": "`+reqID+`", "type": "functional", "title": "New req"}]
	}`)
	root2, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	mod2 := findChild(t, root2, mHash)

	// Module hash should differ (meta changed AND requirement was added)
	if mod1.Hash == mod2.Hash {
		t.Fatal("module hash should change when requirement is added")
	}
}

func TestREQ7_BuildTree_MetaKeyFormat(t *testing.T) {
	// Verify that meta keys use the "meta/" prefix format.
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	// Project meta key
	projMeta := findChild(t, root, "meta/project")
	if projMeta.NodeType != "meta" {
		t.Fatalf("meta/project node_type: want meta, got %s", projMeta.NodeType)
	}

	// Module meta key
	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)
	metaKey := "meta/" + alphaHash
	alphaMeta := findChild(t, alpha, metaKey)
	if alphaMeta.NodeType != "meta" {
		t.Fatalf("%s node_type: want meta, got %s", metaKey, alphaMeta.NodeType)
	}
}

func TestREQ7_BuildTree_ModuleNamesMap(t *testing.T) {
	specDir := setupSpecDir(t)

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	names := ModuleNames(root)

	alphaHash := schema.IdentityHash("module", "Alpha")
	betaHash := schema.IdentityHash("module", "Beta")

	if names[alphaHash] != "Alpha" {
		t.Errorf("want Alpha for %s, got %q", alphaHash, names[alphaHash])
	}
	if names[betaHash] != "Beta" {
		t.Errorf("want Beta for %s, got %q", betaHash, names[betaHash])
	}
	if len(names) != 2 {
		t.Errorf("want 2 module names, got %d", len(names))
	}
}

func TestREQ7_BuildTree_IgnoresExtraneousFiles(t *testing.T) {
	specDir := setupSpecDir(t)

	// Add an extra file not referenced in module.json
	writeFile(t, filepath.Join(specDir, "alpha"), "notes.txt", "extra notes\n")

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)

	// Still 6 children (meta + 2 reqs + 2 comps + 1 impl), no notes.txt
	if len(alpha.Children) != 6 {
		t.Fatalf("alpha children: want 6, got %d (extraneous file included?)", len(alpha.Children))
	}
}

func TestREQ2_BuildTree_ComponentWithEmptyContentFile(t *testing.T) {
	specDir := setupSpecDir(t)

	// arch_comp1.md exists but has zero bytes.
	writeFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "")

	root, err := BuildTree(specDir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	alpha := findChild(t, root, alphaHash)
	comp1Key := schema.IdentityHash("alpha", "component", "Comp1")
	comp1 := findChild(t, alpha, comp1Key)

	s := sha256.Sum256([]byte{})
	want := hex.EncodeToString(s[:])
	if comp1.Hash != want {
		t.Fatalf("empty content file leaf hash: want %s, got %s", want, comp1.Hash)
	}
}

// buildDerivationFixture builds a minimal spec directory whose single project
// requirement carries derivationJSON verbatim after its other fields (empty
// string for none, `, "derivation": "pending"` to add the field), and returns
// the built tree plus the requirement's identity hash.
func buildDerivationFixture(t *testing.T, derivationJSON string) (*Node, string) {
	t.Helper()
	dir := t.TempDir()

	reqID := schema.IdentityHash("project", "requirement", "Derivable req")
	modID := schema.IdentityHash("module", "Alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")

	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": "` + reqID + `", "type": "functional", "title": "Derivable req", "priority": 1` + derivationJSON + `}
		],
		"modules": [
			{"id": "` + modID + `", "name": "Alpha", "path": "alpha"}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	must(t, os.MkdirAll(alphaDir, 0755))
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+compID+`", "name": "Comp1", "content": "arch_comp1.md"}
		]
	}`)
	writeFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")

	root, err := BuildTree(dir)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	return root, reqID
}

// TestREQ7_BuildTree_ProjectRequirementDerivationInvariant covers
// test_hashing.md S9: a project requirement's leaf hash must not move when
// `derivation` is added and later removed again (the graduation round trip),
// because `derivation` is deliberately excluded from the project-requirement
// field allowlist. The `meta/project` envelope leaf, hashed from
// project.json's raw bytes, is the only key allowed to move on the
// "pending" variant.
func TestREQ7_BuildTree_ProjectRequirementDerivationInvariant(t *testing.T) {
	rootNoDerivation, reqID := buildDerivationFixture(t, "")
	rootPending, _ := buildDerivationFixture(t, `, "derivation": "pending"`)
	rootGraduated, _ := buildDerivationFixture(t, "")

	reqNoDerivation := findChild(t, rootNoDerivation, reqID)
	reqPending := findChild(t, rootPending, reqID)
	reqGraduated := findChild(t, rootGraduated, reqID)

	if reqNoDerivation.Hash != reqPending.Hash {
		t.Fatalf("requirement leaf hash must be invariant when derivation is added: %s != %s", reqNoDerivation.Hash, reqPending.Hash)
	}
	if reqNoDerivation.Hash != reqGraduated.Hash {
		t.Fatalf("requirement leaf hash must be invariant when derivation is removed again: %s != %s", reqNoDerivation.Hash, reqGraduated.Hash)
	}

	// project.json is byte-identical before and after the graduation round
	// trip, so every key — including the root — must match exactly.
	if rootNoDerivation.Hash != rootGraduated.Hash {
		t.Fatalf("root hash must match after the graduation round trip (byte-identical project.json): %s != %s", rootNoDerivation.Hash, rootGraduated.Hash)
	}

	// The "pending" variant's project.json bytes differ, so meta/project
	// moves and carries the root with it — but nothing else does.
	if rootNoDerivation.Hash == rootPending.Hash {
		t.Fatal("root hash should change when derivation is added to project.json")
	}
	metaNoDerivation := findChild(t, rootNoDerivation, "meta/project")
	metaPending := findChild(t, rootPending, "meta/project")
	if metaNoDerivation.Hash == metaPending.Hash {
		t.Fatal("meta/project leaf hash should differ when derivation is added")
	}

	alphaHash := schema.IdentityHash("module", "Alpha")
	alphaNoDerivation := findChild(t, rootNoDerivation, alphaHash)
	alphaPending := findChild(t, rootPending, alphaHash)
	if alphaNoDerivation.Hash != alphaPending.Hash {
		t.Fatal("alpha module hash should be unaffected by a project-level derivation field")
	}
}
