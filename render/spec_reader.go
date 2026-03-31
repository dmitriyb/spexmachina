package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/schema"
)

// SpecGraph holds the parsed spec directory as an in-memory graph.
type SpecGraph struct {
	Project schema.Project
	Modules []ModuleGraph
}

// ModuleGraph holds a parsed module and its content leaves.
type ModuleGraph struct {
	Module  schema.Module
	Spec    schema.ModuleSpec
	Content map[string]string // relative path → markdown content
}

// ReadSpec reads and parses the spec directory into a SpecGraph.
func ReadSpec(specDir string) (*SpecGraph, error) {
	projPath := filepath.Join(specDir, "project.json")
	projData, err := os.ReadFile(projPath)
	if err != nil {
		return nil, fmt.Errorf("render: read %s: %w", projPath, err)
	}

	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("render: parse %s: %w", projPath, err)
	}

	graph := &SpecGraph{Project: proj}

	for _, mod := range proj.Modules {
		modDir := filepath.Join(specDir, mod.Path)
		modPath := filepath.Join(modDir, "module.json")

		modData, err := os.ReadFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("render: read module %q (%s): %w", mod.Name, modPath, err)
		}

		var spec schema.ModuleSpec
		if err := json.Unmarshal(modData, &spec); err != nil {
			return nil, fmt.Errorf("render: parse module %q (%s): %w", mod.Name, modPath, err)
		}

		content := make(map[string]string)

		// Collect all content references
		var refs []string
		for _, c := range spec.Components {
			if c.Content != "" {
				refs = append(refs, c.Content)
			}
		}
		for _, s := range spec.ImplSections {
			if s.Content != "" {
				refs = append(refs, s.Content)
			}
		}
		for _, f := range spec.DataFlows {
			if f.Content != "" {
				refs = append(refs, f.Content)
			}
		}
		for _, ts := range spec.TestSections {
			if ts.Content != "" {
				refs = append(refs, ts.Content)
			}
		}

		for _, ref := range refs {
			contentPath := filepath.Join(modDir, ref)
			data, err := os.ReadFile(contentPath)
			if err != nil {
				return nil, fmt.Errorf("render: read content %s (module %q): %w", contentPath, mod.Name, err)
			}
			content[ref] = string(data)
		}

		graph.Modules = append(graph.Modules, ModuleGraph{
			Module:  mod,
			Spec:    spec,
			Content: content,
		})
	}

	return graph, nil
}
