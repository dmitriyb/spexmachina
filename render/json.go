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
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("render: encode json: %w", err)
	}
	return nil
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
