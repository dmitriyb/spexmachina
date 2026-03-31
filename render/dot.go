package render

import (
	"fmt"
	"io"
	"strings"
)

// RenderDOT generates a graphviz DOT graph from the spec graph.
func RenderDOT(spec *SpecGraph, w io.Writer) error {
	fmt.Fprintf(w, "digraph spec {\n")
	fmt.Fprintf(w, "  rankdir=LR;\n")
	fmt.Fprintf(w, "  node [fontsize=10];\n")
	fmt.Fprintf(w, "  edge [fontsize=8];\n\n")

	// Project requirement nodes
	for _, r := range spec.Project.Requirements {
		nid := fmt.Sprintf("preq_%d", r.ID)
		fmt.Fprintf(w, "  %s [label=%q, shape=box, fillcolor=lightblue, style=filled];\n",
			nid, r.Title)
	}
	fmt.Fprintf(w, "\n")

	// Module subgraphs
	for _, mg := range spec.Modules {
		modID := sanitizeDOTID(mg.Module.Name)
		fmt.Fprintf(w, "  subgraph cluster_%s {\n", modID)
		fmt.Fprintf(w, "    label=%q;\n", mg.Module.Name)
		fmt.Fprintf(w, "    style=dashed;\n")

		// Module node
		fmt.Fprintf(w, "    %s [label=%q, shape=folder, fillcolor=lightgray, style=filled];\n",
			modID, mg.Module.Name)

		// Requirements
		for _, r := range mg.Spec.Requirements {
			nid := fmt.Sprintf("%s_req_%d", modID, r.ID)
			fmt.Fprintf(w, "    %s [label=%q, shape=box, fillcolor=lightgreen, style=filled];\n",
				nid, r.Title)
		}

		// Components
		for _, c := range mg.Spec.Components {
			nid := fmt.Sprintf("%s_comp_%d", modID, c.ID)
			fmt.Fprintf(w, "    %s [label=%q, shape=component, fillcolor=lightyellow, style=filled];\n",
				nid, c.Name)
		}

		// Impl sections
		for _, s := range mg.Spec.ImplSections {
			nid := fmt.Sprintf("%s_impl_%d", modID, s.ID)
			fmt.Fprintf(w, "    %s [label=%q, shape=note, fillcolor=moccasin, style=filled];\n",
				nid, s.Name)
		}

		// Data flows
		for _, f := range mg.Spec.DataFlows {
			nid := fmt.Sprintf("%s_flow_%d", modID, f.ID)
			fmt.Fprintf(w, "    %s [label=%q, shape=ellipse, fillcolor=plum1, style=filled];\n",
				nid, f.Name)
		}

		fmt.Fprintf(w, "  }\n\n")
	}

	// Edges
	for _, mg := range spec.Modules {
		modID := sanitizeDOTID(mg.Module.Name)

		// requires_module
		for _, depID := range mg.Module.RequiresModule {
			depMod := findModuleByID(spec, depID)
			if depMod != "" {
				fmt.Fprintf(w, "  %s -> %s [label=\"requires_module\"];\n",
					modID, sanitizeDOTID(depMod))
			}
		}

		// Component edges
		for _, c := range mg.Spec.Components {
			compID := fmt.Sprintf("%s_comp_%d", modID, c.ID)

			for _, reqID := range c.Implements {
				fmt.Fprintf(w, "  %s -> %s_req_%d [label=\"implements\", color=blue];\n",
					compID, modID, reqID)
			}
			for _, useID := range c.Uses {
				fmt.Fprintf(w, "  %s -> %s_comp_%d [label=\"uses\", style=dotted];\n",
					compID, modID, useID)
			}
		}

		// Impl section edges
		for _, s := range mg.Spec.ImplSections {
			implID := fmt.Sprintf("%s_impl_%d", modID, s.ID)
			for _, compID := range s.Describes {
				fmt.Fprintf(w, "  %s -> %s_comp_%d [label=\"describes\", color=green];\n",
					implID, modID, compID)
			}
		}

		// Data flow edges
		for _, f := range mg.Spec.DataFlows {
			flowID := fmt.Sprintf("%s_flow_%d", modID, f.ID)
			for _, compID := range f.Uses {
				fmt.Fprintf(w, "  %s -> %s_comp_%d [label=\"uses\", style=dotted];\n",
					flowID, modID, compID)
			}
		}

		// Requirement edges
		for _, r := range mg.Spec.Requirements {
			reqID := fmt.Sprintf("%s_req_%d", modID, r.ID)
			if r.PreqID != 0 {
				fmt.Fprintf(w, "  %s -> preq_%d [label=\"preq_id\", style=dashed, color=blue];\n",
					reqID, r.PreqID)
			}
			for _, depID := range r.DependsOn {
				fmt.Fprintf(w, "  %s -> %s_req_%d [label=\"depends_on\", style=dashed];\n",
					reqID, modID, depID)
			}
		}
	}

	fmt.Fprintf(w, "}\n")
	return nil
}

func sanitizeDOTID(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func findModuleByID(spec *SpecGraph, id int) string {
	for _, mg := range spec.Modules {
		if mg.Module.ID == id {
			return mg.Module.Name
		}
	}
	return ""
}
