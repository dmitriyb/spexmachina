package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// specGraph implements SpecGraph by reading spec files from disk.
type specGraph struct {
	project    *schema.Project
	modules    map[string]*schema.ModuleSpec // keyed by module name
	modulePaths map[string]string            // module name → directory path
	tree       *merkle.Node
}

// NewSpecGraph reads the spec directory and builds a SpecGraph for preflight
// checking. It loads project.json, all module.json files, and the merkle tree.
func NewSpecGraph(specDir string) (SpecGraph, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("preflight: read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("preflight: parse project.json: %w", err)
	}

	modules := map[string]*schema.ModuleSpec{}
	modulePaths := map[string]string{}
	for _, mod := range proj.Modules {
		modDir := filepath.Join(specDir, mod.Path)
		modPath := filepath.Join(modDir, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("preflight: read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return nil, fmt.Errorf("preflight: parse %s: %w", modPath, err)
		}
		modules[mod.Name] = &ms
		modulePaths[mod.Name] = modDir
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return nil, fmt.Errorf("preflight: build tree: %w", err)
	}

	return &specGraph{
		project:     &proj,
		modules:     modules,
		modulePaths: modulePaths,
		tree:        tree,
	}, nil
}

func (sg *specGraph) ModuleByName(name string) (ModuleInfo, error) {
	for _, mod := range sg.project.Modules {
		if mod.Name == name {
			return sg.buildModuleInfo(mod)
		}
	}
	return ModuleInfo{}, fmt.Errorf("preflight: module %q not found", name)
}

func (sg *specGraph) ModuleByID(id int) (ModuleInfo, error) {
	for _, mod := range sg.project.Modules {
		if mod.ID == id {
			return sg.buildModuleInfo(mod)
		}
	}
	return ModuleInfo{}, fmt.Errorf("preflight: module id %d not found", id)
}

func (sg *specGraph) buildModuleInfo(mod schema.Module) (ModuleInfo, error) {
	ms, ok := sg.modules[mod.Name]
	if !ok {
		return ModuleInfo{}, fmt.Errorf("preflight: module spec %q not loaded", mod.Name)
	}

	comps := make([]ComponentInfo, len(ms.Components))
	for i, c := range ms.Components {
		comps[i] = ComponentInfo{
			ID:   c.ID,
			Name: c.Name,
			Uses: c.Uses,
		}
	}

	return ModuleInfo{
		ID:             mod.ID,
		Name:           mod.Name,
		RequiresModule: mod.RequiresModule,
		Components:     comps,
	}, nil
}

// NodeHash returns the current merkle hash for a spec node. The specNodeID
// uses the format "<module_name>/<node_type>/<node_id>" (e.g. "schema/component/1"),
// which is translated to the merkle tree key format "module/<module_id>/<node_type>/<node_id>".
func (sg *specGraph) NodeHash(specNodeID string) (string, error) {
	parts := strings.Split(specNodeID, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("preflight: invalid spec_node_id format: %q", specNodeID)
	}
	moduleName := parts[0]
	nodeType := parts[1]
	nodeID, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("preflight: invalid spec_node_id format: %q", specNodeID)
	}

	// Translate module name to module ID.
	var moduleID int
	found := false
	for _, mod := range sg.project.Modules {
		if mod.Name == moduleName {
			moduleID = mod.ID
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("preflight: module %q not found for hash lookup", moduleName)
	}

	merkleKey := fmt.Sprintf("module/%d/%s/%d", moduleID, nodeType, nodeID)
	return findNodeHash(sg.tree, merkleKey)
}

// findNodeHash searches the merkle tree for a node with the given key.
func findNodeHash(node *merkle.Node, key string) (string, error) {
	if node == nil {
		return "", fmt.Errorf("preflight: node %q not found in tree", key)
	}
	if node.Key == key {
		return node.Hash, nil
	}
	for _, child := range node.Children {
		h, err := findNodeHash(child, key)
		if err == nil {
			return h, nil
		}
	}
	return "", fmt.Errorf("preflight: node %q not found in tree", key)
}
