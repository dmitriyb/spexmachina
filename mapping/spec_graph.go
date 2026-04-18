package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// specGraph implements SpecGraph by reading spec files from disk.
type specGraph struct {
	project *schema.Project
	modules map[string]*schema.ModuleSpec // keyed by module name
	tree    *merkle.Node
}

// NewSpecGraph reads the spec directory and builds a SpecGraph. It loads
// project.json, all module.json files, and the merkle tree.
func NewSpecGraph(specDir string) (SpecGraph, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("spec graph: read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("spec graph: parse project.json: %w", err)
	}

	modules := map[string]*schema.ModuleSpec{}
	for _, mod := range proj.Modules {
		modDir := filepath.Join(specDir, mod.Path)
		modPath := filepath.Join(modDir, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("spec graph: read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return nil, fmt.Errorf("spec graph: parse %s: %w", modPath, err)
		}
		modules[mod.Name] = &ms
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return nil, fmt.Errorf("spec graph: build tree: %w", err)
	}

	return &specGraph{
		project: &proj,
		modules: modules,
		tree:    tree,
	}, nil
}

func (sg *specGraph) ModuleByName(name string) (ModuleInfo, error) {
	for _, mod := range sg.project.Modules {
		if mod.Name == name {
			return sg.buildModuleInfo(mod)
		}
	}
	return ModuleInfo{}, fmt.Errorf("spec graph: module %q not found", name)
}

func (sg *specGraph) ModuleByID(id string) (ModuleInfo, error) {
	for _, mod := range sg.project.Modules {
		if mod.ID == id {
			return sg.buildModuleInfo(mod)
		}
	}
	return ModuleInfo{}, fmt.Errorf("spec graph: module id %s not found", id)
}

func (sg *specGraph) buildModuleInfo(mod schema.Module) (ModuleInfo, error) {
	ms, ok := sg.modules[mod.Name]
	if !ok {
		return ModuleInfo{}, fmt.Errorf("spec graph: module spec %q not loaded", mod.Name)
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
