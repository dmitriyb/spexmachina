package render

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// RenderMarkdown generates a collated markdown document from the spec graph.
func RenderMarkdown(spec *SpecGraph, w io.Writer) error {
	fmt.Fprintf(w, "# %s\n\n", spec.Project.Name)
	if spec.Project.Description != "" {
		fmt.Fprintf(w, "%s\n\n", spec.Project.Description)
	}

	writeProjectRequirements(w, spec.Project.Requirements)

	if err := writeSections(w, spec.Project.Sections); err != nil {
		return err
	}

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

	writeModuleRequirements(w, mg.Spec.Requirements)
	writeModuleAPIs(w, mg.Spec.APIs)

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

// writeModuleRequirements enumerates each module's requirements locally as
// FR1, FR2, ... NFR1, NFR2, ... — module-scoped numbering, independent of
// the 12-char identity hash IDs.
func writeModuleRequirements(w io.Writer, reqs []schema.ModuleRequirement) {
	if len(reqs) == 0 {
		return
	}

	fmt.Fprintf(w, "### Requirements\n\n")

	var functional, nonFunctional []schema.ModuleRequirement
	for _, r := range reqs {
		if r.Type == "non_functional" {
			nonFunctional = append(nonFunctional, r)
		} else {
			functional = append(functional, r)
		}
	}

	for i, r := range functional {
		fmt.Fprintf(w, "- FR%d: %s", i+1, r.Title)
		if r.Description != "" {
			fmt.Fprintf(w, " — %s", r.Description)
		}
		fmt.Fprintf(w, "\n")
	}
	for i, r := range nonFunctional {
		fmt.Fprintf(w, "- NFR%d: %s", i+1, r.Title)
		if r.Description != "" {
			fmt.Fprintf(w, " — %s", r.Description)
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "\n")
}

// writeModuleAPIs lists the module's external surface in declaration order.
// An api has no content leaf, so the whole node renders on one line: the exact
// surface string, its freeform group when set, and its description.
func writeModuleAPIs(w io.Writer, apis []schema.API) {
	if len(apis) == 0 {
		return
	}

	fmt.Fprintf(w, "### APIs\n\n")
	for _, a := range apis {
		fmt.Fprintf(w, "- `%s`", a.Name)
		if a.Group != "" {
			fmt.Fprintf(w, " (%s)", a.Group)
		}
		if a.Description != "" {
			fmt.Fprintf(w, " — %s", a.Description)
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "\n")
}

// writeSections emits the `## Sections` heading followed by each section's
// envelope and freeform content. Sections preserve their declaration order
// from project.json. Nothing is emitted when the spec has no sections.
func writeSections(w io.Writer, sections []schema.Section) error {
	if len(sections) == 0 {
		return nil
	}

	fmt.Fprintf(w, "## Sections\n\n")
	for _, s := range sections {
		fmt.Fprintf(w, "### %s (%s)\n\n", s.Name, s.Type)
		if err := renderSectionContent(w, s.Raw); err != nil {
			return fmt.Errorf("render: section %q: %w", s.Name, err)
		}
	}
	return nil
}

// renderSectionContent walks the freeform JSON body of a section, skipping
// the envelope fields (id, name, type), and renders remaining fields as
// markdown. Objects become nested bullet lists with bold keys, arrays
// become ordered lists, scalars render inline.
func renderSectionContent(w io.Writer, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	keys := make([]string, 0, len(body))
	for k := range body {
		if k == "id" || k == "name" || k == "type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		return nil
	}

	for _, k := range keys {
		renderSectionField(w, k, body[k], 0)
	}
	fmt.Fprintf(w, "\n")
	return nil
}

// renderSectionField emits a single key/value pair in the freeform body.
// indent is the bullet nesting level (0 = top-level list item).
func renderSectionField(w io.Writer, key string, val any, indent int) {
	pad := strings.Repeat("  ", indent)
	switch v := val.(type) {
	case map[string]any:
		fmt.Fprintf(w, "%s- **%s**:\n", pad, key)
		subKeys := make([]string, 0, len(v))
		for sk := range v {
			subKeys = append(subKeys, sk)
		}
		sort.Strings(subKeys)
		for _, sk := range subKeys {
			renderSectionField(w, sk, v[sk], indent+1)
		}
	case []any:
		fmt.Fprintf(w, "%s- **%s**:\n", pad, key)
		for i, item := range v {
			switch iv := item.(type) {
			case map[string]any:
				fmt.Fprintf(w, "%s  %d. ", pad, i+1)
				renderSectionInlineObject(w, iv)
				fmt.Fprintf(w, "\n")
			default:
				fmt.Fprintf(w, "%s  %d. %s\n", pad, i+1, formatScalar(item))
			}
		}
	default:
		fmt.Fprintf(w, "%s- **%s**: %s\n", pad, key, formatScalar(val))
	}
}

// renderSectionInlineObject flattens a small object into a single line as
// `key=value, key=value`. Used for list items that are objects.
func renderSectionInlineObject(w io.Writer, obj map[string]any) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, formatScalar(obj[k])))
	}
	fmt.Fprint(w, strings.Join(parts, ", "))
}

func formatScalar(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", s)
	}
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
