package render

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

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

// nodeIDAbbrev maps a built-in node-type name to the abbreviated kind used
// in synthetic node IDs (module:<name>:<kind>:<id>). A profile-declared type
// outside this set uses its own type name as the kind instead, so a new
// type reaches the graph with a readable ID without renderer changes.
var nodeIDAbbrev = map[string]string{
	"requirement":  "req",
	"component":    "comp",
	"data_flow":    "flow",
	"test_section": "test",
}

func nodeIDKind(nodeType string) string {
	if abbr, ok := nodeIDAbbrev[nodeType]; ok {
		return abbr
	}
	return nodeType
}

// nodeIndex resolves a node's declared identity hash to the synthetic ID
// RenderJSON gives it, so an edge naming a raw hash target can be rewritten
// to the same synthetic ID its node carries — across module boundaries,
// since preq_id points from a module requirement to a project one.
type nodeIndex map[string]string

// buildNodeIndex indexes every node RenderJSON will emit — the project's
// own nodes, every module, and every node each module declares — before any
// node or edge is written, so an edge encountered anywhere in the walk can
// already resolve any target.
func buildNodeIndex(spec *SpecGraph) nodeIndex {
	idx := make(nodeIndex)
	for _, n := range spec.ProjectNodes {
		idx[n.ID] = fmt.Sprintf("project:%s:%s", nodeIDKind(n.Type), n.ID)
	}
	for _, mg := range spec.Modules {
		idx[mg.Module.ID] = fmt.Sprintf("module:%s", mg.Module.Name)
		for _, n := range mg.Nodes {
			idx[n.ID] = fmt.Sprintf("module:%s:%s:%s", mg.Module.Name, nodeIDKind(n.Type), n.ID)
		}
	}
	return idx
}

// graphNode converts one generically-read Node into the JSON envelope,
// resolving its own synthetic ID via idx and inlining content looked up
// from content, the same content map ReadSpec built alongside n.
func graphNode(n Node, idx nodeIndex, content map[string]string) GraphNode {
	gn := GraphNode{
		ID:          idx[n.ID],
		Type:        n.Type,
		Name:        n.Name,
		Description: n.Description,
		Module:      n.Module,
		Group:       n.Group,
	}
	if n.Content != "" {
		gn.Content = content[n.Content]
	}
	return gn
}

// nodeEdges emits one GraphEdge per (edge kind, target) pair n declares. It
// walks the resolved profile's edge declarations, in their declared order,
// rather than n.Edges directly: n.Edges is a map, and Go map iteration
// order is randomized, which would make two renderings of one unchanged
// spec byte-different. A target absent from idx is skipped rather than
// emitted as a bare hash — a dangling reference is a validator concern, not
// a rendering one.
func nodeEdges(n Node, idx nodeIndex, profile *schema.Profile) []GraphEdge {
	fromID, ok := idx[n.ID]
	if !ok {
		return nil
	}
	var edges []GraphEdge
	for _, e := range profile.Edges {
		if !slices.Contains(e.From, n.Type) {
			continue
		}
		for _, target := range n.Edges[e.Kind] {
			toID, ok := idx[target]
			if !ok {
				continue
			}
			edges = append(edges, GraphEdge{From: fromID, To: toID, Type: e.Kind})
		}
	}
	return edges
}

// RenderJSON generates a machine-readable JSON graph from the spec.
//
// Both arrays follow one declaration walk: the project node, the project's
// generic nodes and sections, then each module in turn contributing its own
// node and its declared nodes in the resolved profile's node-type order.
// Node envelopes and edges are read off spec.ProjectNodes and
// spec.Modules[].Nodes — the profile-generic node lists ReadSpec built —
// rather than off the fixed schema.ModuleSpec fields, so a profile-declared
// type reaches the graph exactly as the built-in five do (see
// flow_render_pipeline.md "Shape contract"). requires_module is the one
// edge kept off the generic node walk: "module" is a fixed interior
// concept the resolved profile's node types never declare.
func RenderJSON(spec *SpecGraph, w io.Writer) error {
	out := graphOutput{}
	idx := buildNodeIndex(spec)

	// Project node
	out.Nodes = append(out.Nodes, GraphNode{
		ID:          "project",
		Type:        "project",
		Name:        spec.Project.Name,
		Description: spec.Project.Description,
	})

	// Project-scoped nodes (requirements, and any profile-declared
	// project-scoped type)
	for _, n := range spec.ProjectNodes {
		out.Nodes = append(out.Nodes, graphNode(n, idx, spec.Content))
		out.Edges = append(out.Edges, nodeEdges(n, idx, spec.Profile)...)
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

		// This module's declared nodes, in the resolved profile's
		// node-type order — under the default profile: requirements,
		// components, data flows, test sections, apis.
		for _, n := range mg.Nodes {
			out.Nodes = append(out.Nodes, graphNode(n, idx, mg.Content))
			out.Edges = append(out.Edges, nodeEdges(n, idx, spec.Profile)...)
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
// not a slim node — it has no identity hash. Output is compact — this is a
// lookup table, not a document — and carries no edges, which callers read
// from module.json directly.
func RenderJSONSlim(spec *SpecGraph, w io.Writer) error {
	out := slimOutput{Nodes: slimNodes(spec)}

	enc := json.NewEncoder(w)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("render: encode slim json: %w", err)
	}
	return nil
}

// slimNodes walks the spec graph in declaration order and returns one
// [SlimNode] per node [RenderJSON] emits, bar the project root. It reads
// spec.ProjectNodes and spec.Modules[].Nodes — the same profile-generic
// node lists RenderJSON walks — so a profile-declared type reaches the
// slim lookup table beside the built-in five.
//
// Every ID is the declared ID copied verbatim. Identity hashes are read out of
// the spec, never recomputed from the node's name — legacy IDs that predate the
// current identity string (15 of them in this project's own project.json) must
// survive a render untouched or every consumer keyed on them breaks.
func slimNodes(spec *SpecGraph) []SlimNode {
	nodes := make([]SlimNode, 0, 64)

	for _, n := range spec.ProjectNodes {
		nodes = append(nodes, SlimNode{ID: n.ID, Type: n.Type, Name: n.Name})
	}
	for _, s := range spec.Project.Sections {
		nodes = append(nodes, SlimNode{ID: s.ID, Type: "section", Name: s.Name})
	}

	for _, mg := range spec.Modules {
		modName := mg.Module.Name
		nodes = append(nodes, SlimNode{ID: mg.Module.ID, Type: "module", Name: modName})

		for _, n := range mg.Nodes {
			nodes = append(nodes, SlimNode{ID: n.ID, Type: n.Type, Name: n.Name, Module: modName})
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
