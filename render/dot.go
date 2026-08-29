package render

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// dotStyle is the shape and fill a DOT node declaration carries.
type dotStyle struct {
	Shape string
	Fill  string
}

// builtinDOTStyles maps a "<scope>:<type>" key to the shape and fill
// arch_dot_renderer.md's Node Mapping table fixes for it. "requirement" is
// keyed by scope because a project requirement and a module requirement
// share a type name but are drawn with different fills.
var builtinDOTStyles = map[string]dotStyle{
	"project:requirement": {Shape: "box", Fill: "lightblue"},
	"module:requirement":  {Shape: "box", Fill: "lightgreen"},
	"module:component":    {Shape: "component", Fill: "lightyellow"},
	"module:data_flow":    {Shape: "ellipse", Fill: "plum1"},
	"module:api":          {Shape: "cds", Fill: "paleturquoise"},
}

// fallbackDOTStyle draws a profile-declared type beyond the built-in five:
// a shape distinct from every built-in kind's — box, folder, component,
// ellipse, cds and tab — so a custom vocabulary reaches the picture without
// renderer changes.
var fallbackDOTStyle = dotStyle{Shape: "note", Fill: "white"}

// builtinDOTEdgeAttrs maps a built-in edge kind to the DOT attributes
// beyond its label. A profile-declared edge kind absent from this table
// still gets a labelled arrow — plain, with no extra style — so a custom
// vocabulary's edges reach the picture too.
var builtinDOTEdgeAttrs = map[string]string{
	"depends_on":  "style=dashed",
	"implements":  "color=blue",
	"uses":        "style=dotted",
	"provided_by": "color=purple",
	"preq_id":     "style=dashed, color=blue",
}

// RenderDOT generates a graphviz DOT graph from the spec graph.
//
// Node IDs are the bare 12-char identity hashes, exactly as `spex hash-id`
// prints them, so hand-written diagrams can reference the same node names.
// They are always quoted: a hash such as "8f2beb43e606" starts with a digit
// and is not a legal bare DOT identifier. Cluster names keep the readable
// module name — a cluster is not a node.
//
// Nodes are read off spec.ProjectNodes and spec.Modules[].Nodes — the
// resolved profile's declared node types, generically — rather than off the
// fixed schema.ModuleSpec fields, so a profile-declared type reaches the
// graph without renderer changes (flow_render_pipeline.md "Shape
// contract"). Test sections are the one declarable type this format omits
// (they reach the JSON output alone); "module" is drawn separately since it
// is a fixed interior concept no resolved profile declares as a node type.
func RenderDOT(spec *SpecGraph, w io.Writer) error {
	fmt.Fprintf(w, "digraph spec {\n")
	fmt.Fprintf(w, "  rankdir=LR;\n")
	fmt.Fprintf(w, "  node [fontsize=10];\n")
	fmt.Fprintf(w, "  edge [fontsize=8];\n\n")

	// Project-scoped nodes (requirements, and any profile-declared
	// project-scoped type), drawn before the first module cluster so they
	// stay at project scope.
	for _, n := range spec.ProjectNodes {
		writeDOTNode(w, "  ", n, "project")
	}
	if len(spec.ProjectNodes) > 0 {
		fmt.Fprintf(w, "\n")
	}

	// Project-level section nodes.
	for _, s := range spec.Project.Sections {
		fmt.Fprintf(w, "  %q [label=%q, shape=tab, fillcolor=mistyrose, style=filled];\n",
			s.ID, s.Name)
	}
	if len(spec.Project.Sections) > 0 {
		fmt.Fprintf(w, "\n")
	}

	// Module subgraphs
	for _, mg := range spec.Modules {
		fmt.Fprintf(w, "  subgraph cluster_%s {\n", sanitizeDOTID(mg.Module.Name))
		fmt.Fprintf(w, "    label=%q;\n", mg.Module.Name)
		fmt.Fprintf(w, "    style=dashed;\n")

		// Module node
		fmt.Fprintf(w, "    %q [label=%q, shape=folder, fillcolor=lightgray, style=filled];\n",
			mg.Module.ID, mg.Module.Name)

		// This module's declared nodes, in the resolved profile's
		// node-type order — under the default profile: requirements,
		// components, data flows, apis (test sections excluded).
		for _, n := range mg.Nodes {
			writeDOTNode(w, "    ", n, "module")
		}

		fmt.Fprintf(w, "  }\n\n")
	}

	// Edges. Project-scoped nodes' edges first, then requires_module and
	// each module's declared nodes' edges, then section coupling — edges
	// come last because they are the only statements that join two
	// clusters.
	for _, n := range spec.ProjectNodes {
		writeDOTEdges(w, n, spec.Profile)
	}

	for _, mg := range spec.Modules {
		// requires_module. requires_module already holds module identity
		// hashes; the lookup only confirms the target module is declared.
		for _, depID := range mg.Module.RequiresModule {
			if findModuleByID(spec, depID) == "" {
				continue
			}
			fmt.Fprintf(w, "  %q -> %q [label=\"requires_module\"];\n", mg.Module.ID, depID)
		}

		for _, n := range mg.Nodes {
			writeDOTEdges(w, n, spec.Profile)
		}
	}

	// Section coupling edges. A coupled section links to the module sharing
	// its name; sections without a matching module or of another type emit
	// no edge.
	for _, s := range spec.Project.Sections {
		if s.Type != "coupled" {
			continue
		}
		modID := findModuleIDByName(spec, s.Name)
		if modID == "" {
			continue
		}
		fmt.Fprintf(w, "  %q -> %q [label=\"coupled\", style=bold];\n", s.ID, modID)
	}

	fmt.Fprintf(w, "}\n")
	return nil
}

// writeDOTNode declares one generically-read Node, skipping test sections —
// this format's own excluded projection. A built-in type is drawn with its
// arch_dot_renderer.md-declared shape and fill; any other profile-declared
// type falls back to a shape and fill distinct from every built-in kind's.
func writeDOTNode(w io.Writer, indent string, n Node, scope string) {
	if n.Type == "test_section" {
		return
	}
	style, ok := builtinDOTStyles[scope+":"+n.Type]
	if !ok {
		style = fallbackDOTStyle
	}
	fmt.Fprintf(w, "%s%q [label=%q, shape=%s, fillcolor=%s, style=filled];\n",
		indent, n.ID, n.Name, style.Shape, style.Fill)
}

// writeDOTEdges emits one labelled edge per (edge kind, target) pair n
// declares, walking the resolved profile's edge declarations rather than a
// fixed set of struct fields — so a profile-declared edge kind reaches the
// picture as a labelled arrow exactly as the built-in kinds do. Every edge
// carries a label naming its kind, so the relationship survives in a
// rendering that drops colour. requires_module is emitted by the caller
// instead: "module" is a fixed interior concept no resolved profile
// declares as a node type, so it never appears in n.Edges. describes is
// never reached here either — it is only ever declared from a test_section,
// and test_section nodes are skipped by [writeDOTNode], so this format
// never iterates one to find it.
func writeDOTEdges(w io.Writer, n Node, profile *schema.Profile) {
	if n.Type == "test_section" {
		return
	}
	for _, e := range profile.Edges {
		if e.Kind == "requires_module" {
			continue
		}
		if !slices.Contains(e.From, n.Type) {
			continue
		}
		for _, target := range n.Edges[e.Kind] {
			if attrs, ok := builtinDOTEdgeAttrs[e.Kind]; ok {
				fmt.Fprintf(w, "  %q -> %q [label=%q, %s];\n", n.ID, target, e.Kind, attrs)
			} else {
				fmt.Fprintf(w, "  %q -> %q [label=%q];\n", n.ID, target, e.Kind)
			}
		}
	}
}

// findModuleIDByName returns the identity hash of the module with the given
// name, or "" when no module matches.
func findModuleIDByName(spec *SpecGraph, name string) string {
	for _, mg := range spec.Modules {
		if mg.Module.Name == name {
			return mg.Module.ID
		}
	}
	return ""
}

func sanitizeDOTID(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func findModuleByID(spec *SpecGraph, id string) string {
	for _, mg := range spec.Modules {
		if mg.Module.ID == id {
			return mg.Module.Name
		}
	}
	return ""
}
