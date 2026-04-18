package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// RenderMarkdown generates a collated markdown document from the spec graph.
func RenderMarkdown(spec *SpecGraph, w io.Writer) error {
	// Project heading and description
	fmt.Fprintf(w, "# %s\n\n", spec.Project.Name)
	if spec.Project.Description != "" {
		fmt.Fprintf(w, "%s\n\n", spec.Project.Description)
	}

	// Project requirements
	writeProjectRequirements(w, spec.Project.Requirements)

	// Modules in declaration order
	for _, mg := range spec.Modules {
		writeModule(w, &mg)
	}

	return nil
}

func writeProjectRequirements(w io.Writer, reqs []schema.Requirement) {
	if len(reqs) == 0 {
		return
	}

	fmt.Fprintf(w, "## Requirements\n\n")

	var functional, nonFunctional []schema.Requirement
	for _, r := range reqs {
		if r.Type == "non_functional" {
			nonFunctional = append(nonFunctional, r)
		} else {
			functional = append(functional, r)
		}
	}

	if len(functional) > 0 {
		fmt.Fprintf(w, "### Functional\n\n")
		for i, r := range functional {
			fmt.Fprintf(w, "- FR%d: %s", i+1, r.Title)
			if r.Description != "" {
				fmt.Fprintf(w, " — %s", r.Description)
			}
			fmt.Fprintf(w, "\n")
			if i == len(functional)-1 {
				fmt.Fprintf(w, "\n")
			}
		}
	}

	if len(nonFunctional) > 0 {
		fmt.Fprintf(w, "### Non-functional\n\n")
		for i, r := range nonFunctional {
			fmt.Fprintf(w, "- NFR%d: %s", i+len(functional)+1, r.Title)
			if r.Description != "" {
				fmt.Fprintf(w, " — %s", r.Description)
			}
			fmt.Fprintf(w, "\n")
			if i == len(nonFunctional)-1 {
				fmt.Fprintf(w, "\n")
			}
		}
	}
}

func writeModule(w io.Writer, mg *ModuleGraph) {
	fmt.Fprintf(w, "## Module: %s\n\n", mg.Module.Name)
	if mg.Spec.Description != "" {
		fmt.Fprintf(w, "%s\n\n", mg.Spec.Description)
	}

	// Module requirements
	writeModuleRequirements(w, mg.Spec.Requirements)

	// Architecture
	if len(mg.Spec.Components) > 0 {
		fmt.Fprintf(w, "### Architecture\n\n")
		for _, c := range mg.Spec.Components {
			if c.Content != "" {
				if content, ok := mg.Content[c.Content]; ok && content != "" {
					fmt.Fprintf(w, "%s\n", adjustHeadings(content, 3))
				}
			} else {
				fmt.Fprintf(w, "#### %s\n\n", c.Name)
			}
		}
	}

	// Implementation
	if len(mg.Spec.ImplSections) > 0 {
		fmt.Fprintf(w, "### Implementation\n\n")
		for _, s := range mg.Spec.ImplSections {
			if s.Content != "" {
				if content, ok := mg.Content[s.Content]; ok && content != "" {
					fmt.Fprintf(w, "%s\n", adjustHeadings(content, 3))
				}
			} else {
				fmt.Fprintf(w, "#### %s\n\n", s.Name)
			}
		}
	}

	// Data Flows
	if len(mg.Spec.DataFlows) > 0 {
		fmt.Fprintf(w, "### Data Flows\n\n")
		for _, f := range mg.Spec.DataFlows {
			if f.Content != "" {
				if content, ok := mg.Content[f.Content]; ok && content != "" {
					fmt.Fprintf(w, "%s\n", adjustHeadings(content, 3))
				}
			} else {
				fmt.Fprintf(w, "#### %s\n\n", f.Name)
			}
		}
	}
}

// TODO(bead:spexmachina-qpn): fix after spexmachina-e8t changed ModuleRequirement to string IDs
func writeModuleRequirements(w io.Writer, reqs []schema.ModuleRequirement) {
	if len(reqs) == 0 {
		return
	}

	fmt.Fprintf(w, "### Requirements\n\n")
	for _, r := range reqs {
		prefix := "FR"
		if r.Type == "non_functional" {
			prefix = "NFR"
		}
		fmt.Fprintf(w, "- %s%s: %s", prefix, r.ID, r.Title)
		if r.Description != "" {
			fmt.Fprintf(w, " — %s", r.Description)
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "\n")
}

// adjustHeadings increases heading levels in content by baseLevel.
// A # heading at baseLevel 3 becomes #### (3 + 1 = 4 hashes).
// Caps at 6 hashes (markdown maximum).
func adjustHeadings(content string, baseLevel int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			hashes := 0
			for _, ch := range line {
				if ch == '#' {
					hashes++
				} else {
					break
				}
			}
			if hashes > 0 && hashes < len(line) && line[hashes] == ' ' {
				newLevel := hashes + baseLevel
				if newLevel > 6 {
					newLevel = 6
				}
				lines[i] = strings.Repeat("#", newLevel) + line[hashes:]
			}
		}
	}
	return strings.Join(lines, "\n")
}
