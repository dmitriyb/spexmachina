package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dmitriyb/spexmachina/schema"
)

// GraphNode represents a node in the JSON graph output.
type GraphNode struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
	Module      string `json:"module,omitempty"`
	Group       string `json:"group,omitempty"`
}

// SlimNode is the reduced node record emitted by [RenderJSONSlim]. It carries
// the bare identity hash, the node type, the node name and the owning module —
// the name→hash lookup table authoring agents and the validator need. Inlined
// content and descriptions are dropped; edges live in module.json.
type SlimNode struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Module string `json:"module,omitempty"`
}

// GraphEdge represents an edge in the JSON graph output.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type graphOutput struct {
	Nodes []any       `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type slimOutput struct {
	Nodes []SlimNode `json:"nodes"`
}

// RenderJSON generates a machine-readable JSON graph from the spec.
func RenderJSON(spec *SpecGraph, w io.Writer) error {
	out := graphOutput{}

	// Project node
	out.Nodes = append(out.Nodes, GraphNode{
		ID:          "project",
		Type:        "project",
		Name:        spec.Project.Name,
		Description: spec.Project.Description,
	})

	// Project-level requirements
	for _, r := range spec.Project.Requirements {
		out.Nodes = append(out.Nodes, GraphNode{
			ID:          fmt.Sprintf("project:req:%s", r.ID),
			Type:        "requirement",
			Name:        r.Title,
			Description: r.Description,
		})
	}

	// Project-level sections (generic, iterate by name)
	for _, s := range spec.Project.Sections {
		node, err := sectionNode(s)
		if err != nil {
			return err
		}
		out.Nodes = append(out.Nodes, node)
		if s.Type == "coupled" {
			out.Edges = append(out.Edges, GraphEdge{
				From: fmt.Sprintf("section:%s", s.Name),
				To:   fmt.Sprintf("module:%s", s.Name),
				Type: "coupled",
			})
		}
	}

	for _, mg := range spec.Modules {
		modName := mg.Module.Name
		modNodeID := fmt.Sprintf("module:%s", modName)

		// Module node
		out.Nodes = append(out.Nodes, GraphNode{
			ID:          modNodeID,
			Type:        "module",
			Name:        modName,
			Description: mg.Module.Description,
		})

		// requires_module edges
		for _, depID := range mg.Module.RequiresModule {
			depMod := findModuleByID(spec, depID)
			if depMod != "" {
				out.Edges = append(out.Edges, GraphEdge{
					From: modNodeID,
					To:   fmt.Sprintf("module:%s", depMod),
					Type: "requires_module",
				})
			}
		}

		// Requirements
		for _, r := range mg.Spec.Requirements {
			nodeID := fmt.Sprintf("module:%s:req:%s", modName, r.ID)
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "requirement",
				Name:        r.Title,
				Description: r.Description,
				Module:      modName,
			})
			if r.PreqID != "" {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("project:req:%s", r.PreqID),
					Type: "preq_id",
				})
			}
			for _, depID := range r.DependsOn {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:req:%s", modName, depID),
					Type: "depends_on",
				})
			}
		}

		// Components
		for _, c := range mg.Spec.Components {
			nodeID := fmt.Sprintf("module:%s:comp:%s", modName, c.ID)
			contentText := ""
			if c.Content != "" {
				contentText = mg.Content[c.Content]
			}
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "component",
				Name:        c.Name,
				Description: c.Description,
				Content:     contentText,
				Module:      modName,
			})
			for _, reqID := range c.Implements {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:req:%s", modName, reqID),
					Type: "implements",
				})
			}
			for _, useID := range c.Uses {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%s", modName, useID),
					Type: "uses",
				})
			}
		}

		// Impl sections
		for _, s := range mg.Spec.ImplSections {
			nodeID := fmt.Sprintf("module:%s:impl:%s", modName, s.ID)
			contentText := ""
			if s.Content != "" {
				contentText = mg.Content[s.Content]
			}
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "impl_section",
				Name:        s.Name,
				Description: "",
				Content:     contentText,
				Module:      modName,
			})
			for _, compID := range s.Describes {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%s", modName, compID),
					Type: "describes",
				})
			}
		}

		// Data flows
		for _, f := range mg.Spec.DataFlows {
			nodeID := fmt.Sprintf("module:%s:flow:%s", modName, f.ID)
			contentText := ""
			if f.Content != "" {
				contentText = mg.Content[f.Content]
			}
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "data_flow",
				Name:        f.Name,
				Description: f.Description,
				Content:     contentText,
				Module:      modName,
			})
			for _, compID := range f.Uses {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%s", modName, compID),
					Type: "uses",
				})
			}
		}

		// Test sections
		for _, ts := range mg.Spec.TestSections {
			nodeID := fmt.Sprintf("module:%s:test:%s", modName, ts.ID)
			contentText := ""
			if ts.Content != "" {
				contentText = mg.Content[ts.Content]
			}
			out.Nodes = append(out.Nodes, GraphNode{
				ID:      nodeID,
				Type:    "test_section",
				Name:    ts.Name,
				Content: contentText,
				Module:  modName,
			})
			for _, compID := range ts.Describes {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%s", modName, compID),
					Type: "describes",
				})
			}
		}

		// APIs. An api has no content leaf; it hashes from its JSON fields
		// alone, so only the envelope and its provided_by edges are emitted.
		for _, a := range mg.Spec.APIs {
			nodeID := fmt.Sprintf("module:%s:api:%s", modName, a.ID)
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "api",
				Name:        a.Name,
				Description: a.Description,
				Module:      modName,
				Group:       a.Group,
			})
			for _, compID := range a.ProvidedBy {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%s", modName, compID),
					Type: "provided_by",
				})
			}
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("render: encode json: %w", err)
	}
	return nil
}

// RenderJSONSlim writes the nodes-only view of the spec: every node type
// [RenderJSON] emits, reduced to {id, type, name, module}. The project root is
// not a slim node — it has no identity hash. Milestones and test_plan scenarios
// are not slim nodes either: [RenderJSON] omits them, and the slim view mirrors
// it rather than inventing node types no other render output carries. Output is
// compact — this is a lookup table, not a document — and carries no edges,
// which callers read from module.json directly.
func RenderJSONSlim(spec *SpecGraph, w io.Writer) error {
	out := slimOutput{Nodes: slimNodes(spec)}

	enc := json.NewEncoder(w)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("render: encode slim json: %w", err)
	}
	return nil
}

// slimNodes walks the spec graph in declaration order and returns one
// [SlimNode] per node [RenderJSON] emits, bar the project root.
//
// Every ID is the declared ID copied verbatim. Identity hashes are read out of
// the spec, never recomputed from the node's name — legacy IDs that predate the
// current identity string (15 of them in this project's own project.json) must
// survive a render untouched or every consumer keyed on them breaks.
func slimNodes(spec *SpecGraph) []SlimNode {
	nodes := make([]SlimNode, 0, 64)

	for _, r := range spec.Project.Requirements {
		nodes = append(nodes, SlimNode{ID: r.ID, Type: "requirement", Name: r.Title})
	}
	for _, s := range spec.Project.Sections {
		nodes = append(nodes, SlimNode{ID: s.ID, Type: "section", Name: s.Name})
	}

	for _, mg := range spec.Modules {
		modName := mg.Module.Name
		nodes = append(nodes, SlimNode{ID: mg.Module.ID, Type: "module", Name: modName})

		for _, r := range mg.Spec.Requirements {
			nodes = append(nodes, SlimNode{ID: r.ID, Type: "requirement", Name: r.Title, Module: modName})
		}
		for _, c := range mg.Spec.Components {
			nodes = append(nodes, SlimNode{ID: c.ID, Type: "component", Name: c.Name, Module: modName})
		}
		for _, s := range mg.Spec.ImplSections {
			nodes = append(nodes, SlimNode{ID: s.ID, Type: "impl_section", Name: s.Name, Module: modName})
		}
		for _, f := range mg.Spec.DataFlows {
			nodes = append(nodes, SlimNode{ID: f.ID, Type: "data_flow", Name: f.Name, Module: modName})
		}
		for _, ts := range mg.Spec.TestSections {
			nodes = append(nodes, SlimNode{ID: ts.ID, Type: "test_section", Name: ts.Name, Module: modName})
		}
		for _, a := range mg.Spec.APIs {
			nodes = append(nodes, SlimNode{ID: a.ID, Type: "api", Name: a.Name, Module: modName})
		}
	}

	return nodes
}

// sectionNode builds a JSON node for a project-level section, merging the
// typed envelope (id, name, section_type) with the freeform content fields
// preserved in s.Raw. Envelope keys override raw duplicates so the synthetic
// id and node type are authoritative.
func sectionNode(s schema.Section) (map[string]any, error) {
	node := map[string]any{}
	if len(s.Raw) > 0 {
		if err := json.Unmarshal(s.Raw, &node); err != nil {
			return nil, fmt.Errorf("render: section %q raw: %w", s.Name, err)
		}
	}
	node["id"] = fmt.Sprintf("section:%s", s.Name)
	node["type"] = "section"
	node["name"] = s.Name
	node["section_type"] = s.Type
	return node, nil
}
