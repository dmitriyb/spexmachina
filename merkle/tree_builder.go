package merkle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dmitriyb/spexmachina/schema"
)

// Node represents a node in the merkle tree. Leaf nodes correspond to spec
// content files, interior nodes correspond to modules or the project root.
// Nodes are keyed by identity hash — the same 12-character hex string stored
// in the JSON `id` field. Synthetic keys use "meta/" prefix for envelope leaves.
type Node struct {
	Key      string  `json:"key"`
	Hash     string  `json:"hash"`
	Type     string  `json:"type"`                // "leaf", "module", "project"
	NodeType string  `json:"node_type,omitempty"`  // "component", "data_flow", "test_section", "api", "meta", "requirement", "module"
	Module   string  `json:"module,omitempty"`     // identity hash of parent module ("" for project-level nodes)
	Children []*Node `json:"children,omitempty"`
	moduleName string // unexported; module name for ModuleNames extraction
}

// ModuleNames extracts a map of module identity hash → module name from the tree.
// Module names are stored during tree construction for downstream use.
func ModuleNames(tree *Node) map[string]string {
	names := map[string]string{}
	if tree == nil {
		return names
	}
	for _, child := range tree.Children {
		if child.Type == "module" {
			names[child.Module] = child.moduleName
		}
	}
	return names
}

// BuildTree constructs a merkle tree from the spec directory. It resolves
// the spec profile to learn which node types exist and how each JSON-backed
// type's leaf hashes, reads project.json to discover modules, then
// module.json files to discover content files, and hashes everything
// bottom-up. Nodes are keyed by identity hash.
func BuildTree(specDir string) (*Node, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	projectJSONPath := filepath.Join(specDir, "project.json")
	projLeaf, err := hashLeaf(projectJSONPath, "meta/project", "meta", "")
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	rawProj, err := readRawFields(projectJSONPath)
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	modules, err := extractModules(rawProj)
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	var projNodes []*Node
	for _, nt := range profile.NodeTypes {
		if nt.Scope != "project" {
			continue
		}
		nodes, err := buildTypeNodes(rawProj, nt, specDir, "", profile)
		if err != nil {
			return nil, fmt.Errorf("merkle: build tree: %w", err)
		}
		projNodes = append(projNodes, nodes...)
	}

	var moduleNodes []*Node
	for _, mod := range modules {
		mNode, err := buildModule(specDir, mod, profile)
		if err != nil {
			return nil, fmt.Errorf("merkle: build tree: %w", err)
		}
		moduleNodes = append(moduleNodes, mNode)
	}

	children := []*Node{projLeaf}
	children = append(children, projNodes...)
	children = append(children, moduleNodes...)

	sort.Slice(children, func(i, j int) bool {
		return children[i].Key < children[j].Key
	})

	childHashes := collectHashes(children)

	return &Node{
		Key:      "project",
		Hash:     HashChildren(childHashes),
		Type:     "project",
		Children: children,
	}, nil
}

func buildModule(specDir string, mod schema.Module, profile *schema.Profile) (*Node, error) {
	modDir := filepath.Join(specDir, mod.Path)
	modJSONPath := filepath.Join(modDir, "module.json")

	moduleHash := mod.ID

	metaKey := "meta/" + moduleHash
	modLeaf, err := hashLeaf(modJSONPath, metaKey, "meta", moduleHash)
	if err != nil {
		return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
	}

	rawMod, err := readRawFields(modJSONPath)
	if err != nil {
		return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
	}

	children := []*Node{modLeaf}

	for _, nt := range profile.NodeTypes {
		if nt.Scope != "module" {
			continue
		}
		nodes, err := buildTypeNodes(rawMod, nt, modDir, moduleHash, profile)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, nodes...)
	}

	// Sort leaf children by key for deterministic hashing.
	sort.Slice(children, func(i, j int) bool {
		return children[i].Key < children[j].Key
	})

	childHashes := collectHashes(children)

	return &Node{
		Key:        moduleHash,
		Hash:       HashChildren(childHashes),
		Type:       "module",
		Module:     moduleHash,
		Children:   children,
		moduleName: mod.Name,
	}, nil
}

// buildTypeNodes builds one leaf node per entry of a single profile-declared
// node type, found under its plural key in raw (project.json's or a
// module.json's top-level fields). A content-bearing type hashes each
// entry's referenced file, resolved against baseDir; a non-content type
// hashes a deterministic JSON serialization of the fields the resolved
// profile's allowlist names for "<scope>:<name>". An entry whose content
// field is the empty string is skipped silently — see "Empty content is not
// a node" in arch_tree_builder.md. A type absent from raw (no module in this
// spec declares it) contributes no nodes.
func buildTypeNodes(raw map[string]json.RawMessage, nt schema.NodeType, baseDir, moduleHash string, profile *schema.Profile) ([]*Node, error) {
	data, ok := raw[nt.PluralKey]
	if !ok {
		return nil, nil
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("merkle: parse %s: %w", nt.PluralKey, err)
	}

	var nodes []*Node
	for _, entry := range entries {
		id, _ := entry["id"].(string)

		if nt.RequiresContent {
			content, _ := entry["content"].(string)
			if content == "" {
				continue
			}
			node, err := hashLeaf(filepath.Join(baseDir, content), id, nt.Name, moduleHash)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
			continue
		}

		allowlist := profile.HashedFields[nt.Scope+":"+nt.Name]
		node, err := hashFields(entry, allowlist, id, nt.Name, moduleHash)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// hashFields creates a leaf node for a JSON-backed spec node — a
// requirement, an api, or any other non-content-bearing type the resolved
// profile declares — by hashing a deterministic serialization of the fields
// named in allowlist. A field absent from entry, or present with a zero
// value, is excluded — matching omitempty semantics — so the leaf hash
// reflects only fields the author actually set. Marshaling a
// map[string]interface{} sorts keys, so the serialization is sorted by
// construction.
func hashFields(entry map[string]interface{}, allowlist []string, key, nodeType, module string) (*Node, error) {
	fields := map[string]interface{}{}
	for _, name := range allowlist {
		v, ok := entry[name]
		if !ok || isZeroJSONValue(v) {
			continue
		}
		fields[name] = v
	}

	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("merkle: hash %s %s: %w", nodeType, key, err)
	}
	return &Node{
		Key:      key,
		Hash:     HashBytes(data),
		Type:     "leaf",
		NodeType: nodeType,
		Module:   module,
	}, nil
}

// isZeroJSONValue reports whether v — a value decoded from JSON into
// interface{} — is the zero value for its JSON type: absent (nil), an empty
// string, the number zero, or an empty array/object.
func isZeroJSONValue(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case float64:
		return x == 0
	case []interface{}:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	default:
		return false
	}
}

// readRawFields reads a JSON file into its top-level fields, left
// unparsed as json.RawMessage, so a profile-declared array can be decoded
// generically by its plural key without a fixed Go struct field for every
// possible node type.
func readRawFields(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("merkle: read %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("merkle: parse %s: %w", path, err)
	}
	return raw, nil
}

// extractModules decodes project.json's "modules" array. Modules are the
// fixed interior-node concept — not a profile-declarable node type — so
// they are always read the same way regardless of the resolved profile.
func extractModules(raw map[string]json.RawMessage) ([]schema.Module, error) {
	data, ok := raw["modules"]
	if !ok {
		return nil, nil
	}
	var modules []schema.Module
	if err := json.Unmarshal(data, &modules); err != nil {
		return nil, fmt.Errorf("merkle: parse modules: %w", err)
	}
	return modules, nil
}

func hashLeaf(path, key, nodeType string, module string) (*Node, error) {
	h, err := HashFile(path)
	if err != nil {
		return nil, fmt.Errorf("merkle: hash leaf %s: %w", key, err)
	}
	return &Node{
		Key:      key,
		Hash:     h,
		Type:     "leaf",
		NodeType: nodeType,
		Module:   module,
	}, nil
}

func collectHashes(nodes []*Node) []string {
	hashes := make([]string, len(nodes))
	for i, n := range nodes {
		hashes[i] = n.Hash
	}
	return hashes
}

func readProject(specDir string) (*schema.Project, error) {
	data, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("merkle: read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("merkle: parse project.json: %w", err)
	}
	return &proj, nil
}

func readModuleSpec(path string) (*schema.ModuleSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("merkle: read %s: %w", path, err)
	}
	var spec schema.ModuleSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("merkle: parse %s: %w", path, err)
	}
	return &spec, nil
}
