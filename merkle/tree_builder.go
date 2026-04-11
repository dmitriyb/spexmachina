package merkle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

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
	NodeType string  `json:"node_type,omitempty"`  // "component", "impl_section", "data_flow", "test_section", "meta", "requirement", "module"
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

// BuildTree constructs a merkle tree from the spec directory. It reads
// project.json to discover modules, then module.json files to discover
// content files, and hashes everything bottom-up. Nodes are keyed by
// identity hash.
func BuildTree(specDir string) (*Node, error) {
	proj, err := readProject(specDir)
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	projectJSONPath := filepath.Join(specDir, "project.json")
	projLeaf, err := hashLeaf(projectJSONPath, "meta/project", "meta", "")
	if err != nil {
		return nil, fmt.Errorf("merkle: build tree: %w", err)
	}

	var projReqNodes []*Node
	for _, req := range proj.Requirements {
		key := schema.IdentityHash("project", "requirement", strconv.Itoa(req.ID))
		node, err := hashRequirement(req, key, "")
		if err != nil {
			return nil, fmt.Errorf("merkle: build tree: %w", err)
		}
		projReqNodes = append(projReqNodes, node)
	}

	var moduleNodes []*Node
	for _, mod := range proj.Modules {
		mNode, err := buildModule(specDir, mod)
		if err != nil {
			return nil, fmt.Errorf("merkle: build tree: %w", err)
		}
		moduleNodes = append(moduleNodes, mNode)
	}

	children := []*Node{projLeaf}
	children = append(children, projReqNodes...)
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

func buildModule(specDir string, mod schema.Module) (*Node, error) {
	modDir := filepath.Join(specDir, mod.Path)
	modJSONPath := filepath.Join(modDir, "module.json")

	modSpec, err := readModuleSpec(modJSONPath)
	if err != nil {
		return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
	}

	moduleHash := schema.IdentityHash("module", mod.Name)

	metaKey := "meta/" + moduleHash
	modLeaf, err := hashLeaf(modJSONPath, metaKey, "meta", moduleHash)
	if err != nil {
		return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
	}

	children := []*Node{modLeaf}

	for _, req := range modSpec.Requirements {
		node, err := hashModuleRequirement(req, req.ID, moduleHash)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, node)
	}

	for _, c := range modSpec.Components {
		if c.Content == "" {
			continue
		}
		leaf, err := hashLeaf(filepath.Join(modDir, c.Content), c.ID, "component", moduleHash)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, leaf)
	}

	for _, s := range modSpec.ImplSections {
		if s.Content == "" {
			continue
		}
		leaf, err := hashLeaf(filepath.Join(modDir, s.Content), s.ID, "impl_section", moduleHash)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, leaf)
	}

	for _, f := range modSpec.DataFlows {
		if f.Content == "" {
			continue
		}
		leaf, err := hashLeaf(filepath.Join(modDir, f.Content), f.ID, "data_flow", moduleHash)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, leaf)
	}

	for _, ts := range modSpec.TestSections {
		if ts.Content == "" {
			continue
		}
		leaf, err := hashLeaf(filepath.Join(modDir, ts.Content), ts.ID, "test_section", moduleHash)
		if err != nil {
			return nil, fmt.Errorf("merkle: build module %s: %w", mod.Name, err)
		}
		children = append(children, leaf)
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

func hashModuleRequirement(req schema.ModuleRequirement, key string, module string) (*Node, error) {
	fields := map[string]interface{}{}
	if len(req.DependsOn) > 0 {
		fields["depends_on"] = req.DependsOn
	}
	if req.Description != "" {
		fields["description"] = req.Description
	}
	fields["id"] = req.ID
	if req.PreqID != "" {
		fields["preq_id"] = req.PreqID
	}
	if req.Title != "" {
		fields["title"] = req.Title
	}
	fields["type"] = req.Type

	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("merkle: hash requirement %s: %w", key, err)
	}
	h := HashBytes(data)
	return &Node{
		Key:      key,
		Hash:     h,
		Type:     "leaf",
		NodeType: "requirement",
		Module:   module,
	}, nil
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

// hashRequirement creates a leaf node for a project-level requirement by
// hashing its deterministic JSON serialization. Fields are sorted by key
// and zero-value fields are excluded (matching omitempty semantics).
func hashRequirement(req schema.Requirement, key string, module string) (*Node, error) {
	fields := map[string]interface{}{}
	if len(req.DependsOn) > 0 {
		fields["depends_on"] = req.DependsOn
	}
	if req.Description != "" {
		fields["description"] = req.Description
	}
	fields["id"] = req.ID
	if req.PreqID != 0 {
		fields["preq_id"] = req.PreqID
	}
	if req.Priority != nil {
		fields["priority"] = *req.Priority
	}
	if req.Title != "" {
		fields["title"] = req.Title
	}
	if req.Type != "" {
		fields["type"] = req.Type
	}

	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("merkle: hash requirement %s: %w", key, err)
	}
	return &Node{
		Key:      key,
		Hash:     HashBytes(data),
		Type:     "leaf",
		NodeType: "requirement",
		Module:   module,
	}, nil
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
