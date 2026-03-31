package render

import (
	"encoding/json"
	"fmt"
	"io"
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
	Nodes []GraphNode `json:"nodes"`
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
			ID:          fmt.Sprintf("project:req:%d", r.ID),
			Type:        "requirement",
			Name:        r.Title,
			Description: r.Description,
		})
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
			nodeID := fmt.Sprintf("module:%s:req:%d", modName, r.ID)
			out.Nodes = append(out.Nodes, GraphNode{
				ID:          nodeID,
				Type:        "requirement",
				Name:        r.Title,
				Description: r.Description,
				Module:      modName,
			})
			if r.PreqID != 0 {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("project:req:%d", r.PreqID),
					Type: "preq_id",
				})
			}
			for _, depID := range r.DependsOn {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:req:%d", modName, depID),
					Type: "depends_on",
				})
			}
		}

		// Components
		for _, c := range mg.Spec.Components {
			nodeID := fmt.Sprintf("module:%s:comp:%d", modName, c.ID)
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
					To:   fmt.Sprintf("module:%s:req:%d", modName, reqID),
					Type: "implements",
				})
			}
			for _, useID := range c.Uses {
				out.Edges = append(out.Edges, GraphEdge{
					From: nodeID,
					To:   fmt.Sprintf("module:%s:comp:%d", modName, useID),
					Type: "uses",
				})
			}
		}

		// Impl sections
		for _, s := range mg.Spec.ImplSections {
			nodeID := fmt.Sprintf("module:%s:impl:%d", modName, s.ID)
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
					To:   fmt.Sprintf("module:%s:comp:%d", modName, compID),
					Type: "describes",
				})
			}
		}

		// Data flows
		for _, f := range mg.Spec.DataFlows {
			nodeID := fmt.Sprintf("module:%s:flow:%d", modName, f.ID)
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
					To:   fmt.Sprintf("module:%s:comp:%d", modName, compID),
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
