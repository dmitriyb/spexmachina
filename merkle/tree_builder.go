package merkle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dmitriyb/spexmachina/schema"
)

// Node represents a node in the merkle tree. Leaf nodes correspond to files,
// interior nodes correspond to groups (arch, impl, flow), modules, or the project root.
type Node struct {
	Name     string  `json:"name"`
	Hash     string  `json:"hash"`
	Type     string  `json:"type"` // "leaf", "arch", "impl", "flow", "module", "project"
	Children []*Node `json:"children,omitempty"`
}

// BuildTree constructs a merkle tree from the spec directory. It reads
// project.json to discover modules, then module.json files to discover
// content files, and hashes everything bottom-up.
func BuildTree(specDir string) (*Node, error) {
	proj, err := readProject(specDir)
	if err != nil {
		return nil, err
	}

	projectJSONPath := filepath.Join(specDir, "project.json")
	projLeaf, err := hashLeaf(projectJSONPath, "project.json")
	if err != nil {
		return nil, err
	}

	var moduleNodes []*Node
	for _, mod := range proj.Modules {
		mNode, err := buildModule(specDir, mod)
		if err != nil {
			return nil, err
		}
		moduleNodes = append(moduleNodes, mNode)
	}

	children := append([]*Node{projLeaf}, moduleNodes...)
	childHashes := make([]string, len(children))
	for i, c := range children {
		childHashes[i] = c.Hash
	}

	return &Node{
		Name:     proj.Name,
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
		return nil, err
	}

	modLeaf, err := hashLeaf(modJSONPath, "module.json")
	if err != nil {
		return nil, err
	}

	children := []*Node{modLeaf}

	archNode, err := buildGroup(modDir, "arch", componentContents(modSpec.Components))
	if err != nil {
		return nil, err
	}
	if archNode != nil {
		children = append(children, archNode)
	}

	implNode, err := buildGroup(modDir, "impl", implSectionContents(modSpec.ImplSections))
	if err != nil {
		return nil, err
	}
	if implNode != nil {
		children = append(children, implNode)
	}

	flowNode, err := buildGroup(modDir, "flow", dataFlowContents(modSpec.DataFlows))
	if err != nil {
		return nil, err
	}
	if flowNode != nil {
		children = append(children, flowNode)
	}

	childHashes := make([]string, len(children))
	for i, c := range children {
		childHashes[i] = c.Hash
	}

	return &Node{
		Name:     mod.Name,
		Hash:     HashChildren(childHashes),
		Type:     "module",
		Children: children,
	}, nil
}

func buildGroup(modDir, groupType string, contentPaths []string) (*Node, error) {
	if len(contentPaths) == 0 {
		return nil, nil
	}

	sort.Strings(contentPaths)

	var leaves []*Node
	for _, rel := range contentPaths {
		absPath := filepath.Join(modDir, rel)
		leaf, err := hashLeaf(absPath, rel)
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, leaf)
	}

	childHashes := make([]string, len(leaves))
	for i, l := range leaves {
		childHashes[i] = l.Hash
	}

	return &Node{
		Name:     groupType,
		Hash:     HashChildren(childHashes),
		Type:     groupType,
		Children: leaves,
	}, nil
}

func hashLeaf(path, name string) (*Node, error) {
	h, err := HashFile(path)
	if err != nil {
		return nil, err
	}
	return &Node{
		Name: name,
		Hash: h,
		Type: "leaf",
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

func componentContents(components []schema.Component) []string {
	var paths []string
	for _, c := range components {
		if c.Content != "" {
			paths = append(paths, c.Content)
		}
	}
	return paths
}

func implSectionContents(sections []schema.ImplSection) []string {
	var paths []string
	for _, s := range sections {
		if s.Content != "" {
			paths = append(paths, s.Content)
		}
	}
	return paths
}

func dataFlowContents(flows []schema.DataFlow) []string {
	var paths []string
	for _, f := range flows {
		if f.Content != "" {
			paths = append(paths, f.Content)
		}
	}
	return paths
}
