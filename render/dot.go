package render

import (
	"fmt"
	"io"
	"strings"
)

// RenderDOT generates a graphviz DOT graph from the spec graph.
//
// Node IDs are the bare 12-char identity hashes, exactly as `spex hash-id`
// prints them, so hand-written diagrams can reference the same node names.
// They are always quoted: a hash such as "8f2beb43e606" starts with a digit
// and is not a legal bare DOT identifier. Cluster names keep the readable
// module name — a cluster is not a node.
//
// TODO(bead:spexmachina-h4gv.9): the loops below walk the five built-in
// node types by their fixed schema.ModuleSpec fields, so a profile-declared
// type beyond those five never reaches this graph. spec.Modules[].Nodes and
// spec.ProjectNodes carry the same declarations generically, keyed by
// spec.Profile's node types — walk those instead so a profile-declared type
// reaches the graph without renderer changes, per flow_render_pipeline.md
// "Shape contract".
func RenderDOT(spec *SpecGraph, w io.Writer) error {
	fmt.Fprintf(w, "digraph spec {\n")
	fmt.Fprintf(w, "  rankdir=LR;\n")
	fmt.Fprintf(w, "  node [fontsize=10];\n")
	fmt.Fprintf(w, "  edge [fontsize=8];\n\n")

	// Project requirement nodes
	for _, r := range spec.Project.Requirements {
		fmt.Fprintf(w, "  %q [label=%q, shape=box, fillcolor=lightblue, style=filled];\n",
			r.ID, r.Title)
	}
	fmt.Fprintf(w, "\n")

	// Project-level section nodes (emitted before module subgraphs so they
	// remain at the project scope, not inside any cluster).
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

		// Requirements
		for _, r := range mg.Spec.Requirements {
			fmt.Fprintf(w, "    %q [label=%q, shape=box, fillcolor=lightgreen, style=filled];\n",
				r.ID, r.Title)
		}

		// Components
		for _, c := range mg.Spec.Components {
			fmt.Fprintf(w, "    %q [label=%q, shape=component, fillcolor=lightyellow, style=filled];\n",
				c.ID, c.Name)
		}

		// Data flows
		for _, f := range mg.Spec.DataFlows {
			fmt.Fprintf(w, "    %q [label=%q, shape=ellipse, fillcolor=plum1, style=filled];\n",
				f.ID, f.Name)
		}

		// APIs
		for _, a := range mg.Spec.APIs {
			fmt.Fprintf(w, "    %q [label=%q, shape=cds, fillcolor=paleturquoise, style=filled];\n",
				a.ID, a.Name)
		}

		fmt.Fprintf(w, "  }\n\n")
	}

	// Edges
	for _, mg := range spec.Modules {
		// requires_module. requires_module already holds module identity
		// hashes; the lookup only confirms the target module is declared.
		for _, depID := range mg.Module.RequiresModule {
			if findModuleByID(spec, depID) == "" {
				continue
			}
			fmt.Fprintf(w, "  %q -> %q [label=\"requires_module\"];\n", mg.Module.ID, depID)
		}

		// Component edges
		for _, c := range mg.Spec.Components {
			for _, reqID := range c.Implements {
				fmt.Fprintf(w, "  %q -> %q [label=\"implements\", color=blue];\n", c.ID, reqID)
			}
			for _, useID := range c.Uses {
				fmt.Fprintf(w, "  %q -> %q [label=\"uses\", style=dotted];\n", c.ID, useID)
			}
		}

		// Data flow edges
		for _, f := range mg.Spec.DataFlows {
			for _, compID := range f.Uses {
				fmt.Fprintf(w, "  %q -> %q [label=\"uses\", style=dotted];\n", f.ID, compID)
			}
		}

		// API edges
		for _, a := range mg.Spec.APIs {
			for _, compID := range a.ProvidedBy {
				fmt.Fprintf(w, "  %q -> %q [label=\"provided_by\", color=purple];\n", a.ID, compID)
			}
		}

		// Requirement edges
		for _, r := range mg.Spec.Requirements {
			if r.PreqID != "" {
				fmt.Fprintf(w, "  %q -> %q [label=\"preq_id\", style=dashed, color=blue];\n",
					r.ID, r.PreqID)
			}
			for _, depID := range r.DependsOn {
				fmt.Fprintf(w, "  %q -> %q [label=\"depends_on\", style=dashed];\n", r.ID, depID)
			}
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
