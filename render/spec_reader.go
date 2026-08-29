package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/schema"
)

// SpecGraph holds the parsed spec directory as an in-memory graph — the one
// value SpecReader builds and hands to exactly one renderer (see
// flow_render_pipeline.md "Data Shapes"). Project and Modules[].Spec are
// the fixed envelope for today's five built-in node types; Profile,
// ProjectNodes and Modules[].Nodes are the same declarations read
// generically off the resolved profile's declared arrays, so a
// profile-declared type beyond the built-in five — invisible to the fixed
// fields — still reaches a renderer that walks Nodes instead. Content holds
// the text of every content leaf a project-scoped node names, keyed the same
// way as ModuleGraph.Content — a project-scoped type can be content-bearing
// too (ComposeProjectSchema gives one a required content path when
// RequiresContent is set), and this is the one place the pipeline reads it.
type SpecGraph struct {
	Project      schema.Project
	Modules      []ModuleGraph
	Profile      *schema.Profile
	ProjectNodes []Node
	Content      map[string]string // relative path → markdown content, project-scoped nodes
}

// ModuleGraph holds a parsed module and its content leaves. Spec is the
// fixed envelope for the built-in five node types; Nodes carries the same
// module's entries read generically off the resolved profile's declared
// arrays — see SpecGraph.
type ModuleGraph struct {
	Module  schema.Module
	Spec    schema.ModuleSpec
	Content map[string]string // relative path → markdown content
	Nodes   []Node
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

	// The profile is resolved once, here, as part of reading the spec
	// directory: SpecReader is the pipeline's sole reader (see
	// flow_render_pipeline.md "SpecReader then runs once"), so a malformed
	// profile.json surfaces as one failure alongside a malformed
	// project.json rather than as a separate pre-flight step or a cascade
	// once individual node-type lookups start failing downstream.
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	rawProj, err := rawTopLevelFields(projData)
	if err != nil {
		return nil, fmt.Errorf("render: parse %s: %w", projPath, err)
	}
	projectNodes, err := nodesForScope(rawProj, profile, "project", "")
	if err != nil {
		return nil, fmt.Errorf("render: parse %s: %w", projPath, err)
	}

	// Content is collected off the generic project node list, the same way
	// a module's is below: a profile-declared project-scoped type carrying
	// a non-empty "content" path gets its leaf read here, resolved against
	// specDir, so it is not invisible to every format for want of a reader.
	projectContent := make(map[string]string)
	for _, n := range projectNodes {
		if n.Content == "" {
			continue
		}
		if _, ok := projectContent[n.Content]; ok {
			continue
		}
		contentPath := filepath.Join(specDir, n.Content)
		data, err := os.ReadFile(contentPath)
		if err != nil {
			return nil, fmt.Errorf("render: read content %s (project): %w", contentPath, err)
		}
		projectContent[n.Content] = string(data)
	}

	graph := &SpecGraph{Project: proj, Profile: profile, ProjectNodes: projectNodes, Content: projectContent}

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

		rawMod, err := rawTopLevelFields(modData)
		if err != nil {
			return nil, fmt.Errorf("render: parse module %q (%s): %w", mod.Name, modPath, err)
		}
		nodes, err := nodesForScope(rawMod, profile, "module", mod.Name)
		if err != nil {
			return nil, fmt.Errorf("render: parse module %q (%s): %w", mod.Name, modPath, err)
		}

		// Content is collected off the generic node list, not a fixed
		// per-type field walk: any profile-declared type carrying a
		// non-empty "content" path — built-in or not — gets its leaf
		// read here, the one place the pipeline touches disk for it.
		content := make(map[string]string)
		for _, n := range nodes {
			if n.Content == "" {
				continue
			}
			if _, ok := content[n.Content]; ok {
				continue
			}
			contentPath := filepath.Join(modDir, n.Content)
			data, err := os.ReadFile(contentPath)
			if err != nil {
				return nil, fmt.Errorf("render: read content %s (module %q): %w", contentPath, mod.Name, err)
			}
			content[n.Content] = string(data)
		}

		graph.Modules = append(graph.Modules, ModuleGraph{
			Module:  mod,
			Spec:    spec,
			Content: content,
			Nodes:   nodes,
		})
	}

	return graph, nil
}
