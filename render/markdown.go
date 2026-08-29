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
//
// Requirements, apis and content-bearing sections (Architecture, Data Flows,
// and any profile-declared type beyond those two) are all read off
// spec.ProjectNodes and spec.Modules[].Nodes — the profile-generic node
// lists ReadSpec built — rather than off the fixed schema.ModuleSpec fields,
// so a profile-declared type reaches this document without renderer
// changes (flow_render_pipeline.md "Shape contract"). test_section is the
// one declarable type this format omits, matching DOTRenderer: it reaches
// the JSON output alone.
func RenderMarkdown(spec *SpecGraph, w io.Writer) error {
	fmt.Fprintf(w, "# %s\n\n", spec.Project.Name)
	if spec.Project.Description != "" {
		fmt.Fprintf(w, "%s\n\n", spec.Project.Description)
	}

	writeRequirements(w, "## Requirements", filterNodesByType(spec.ProjectNodes, "requirement"), true)

	if err := writeSections(w, spec.Project.Sections); err != nil {
		return err
	}

	for _, mg := range spec.Modules {
		writeModule(w, &mg, spec.Profile)
	}

	return nil
}

// writeRequirements renders one requirement block: a heading, then
// functional requirements before non-functional, each group in declaration
// order and numbered from 1 within its own group — "FR"/"NFR" are
// positional labels, not identities. subheadings splits the two groups
// under their own "### Functional"/"### Non-functional" headings (the
// project scope); the module scope renders one flat list instead, with
// non-functional numbering continuing where functional left off would be
// wrong at module scope, so it also restarts at 1 there.
func writeRequirements(w io.Writer, heading string, nodes []Node, subheadings bool) {
	if len(nodes) == 0 {
		return
	}

	var functional, nonFunctional []Node
	for _, n := range nodes {
		if str(n.Fields, "type") == "non_functional" {
			nonFunctional = append(nonFunctional, n)
		} else {
			functional = append(functional, n)
		}
	}

	fmt.Fprintf(w, "%s\n\n", heading)

	if subheadings {
		if len(functional) > 0 {
			fmt.Fprintf(w, "### Functional\n\n")
			writeRequirementList(w, functional, "FR", 0)
			fmt.Fprintf(w, "\n")
		}
		if len(nonFunctional) > 0 {
			fmt.Fprintf(w, "### Non-functional\n\n")
			writeRequirementList(w, nonFunctional, "NFR", len(functional))
			fmt.Fprintf(w, "\n")
		}
		return
	}

	writeRequirementList(w, functional, "FR", 0)
	writeRequirementList(w, nonFunctional, "NFR", 0)
	fmt.Fprintf(w, "\n")
}

// writeRequirementList writes one bullet per requirement node, numbered
// prefix+(offset+1), prefix+(offset+2), ...
func writeRequirementList(w io.Writer, nodes []Node, prefix string, offset int) {
	for i, n := range nodes {
		fmt.Fprintf(w, "- %s%d: %s", prefix, offset+i+1, n.Name)
		if n.Description != "" {
			fmt.Fprintf(w, " — %s", n.Description)
		}
		fmt.Fprintf(w, "\n")
	}
}

func writeModule(w io.Writer, mg *ModuleGraph, profile *schema.Profile) {
	fmt.Fprintf(w, "## Module: %s\n\n", mg.Module.Name)
	if mg.Spec.Description != "" {
		fmt.Fprintf(w, "%s\n\n", mg.Spec.Description)
	}

	writeRequirements(w, "### Requirements", filterNodesByType(mg.Nodes, "requirement"), false)
	writeModuleAPIs(w, filterNodesByType(mg.Nodes, "api"))
	writeModuleContentSections(w, mg, profile)
}

// writeModuleAPIs lists the module's external surface in declaration order.
// An api has no content leaf, so the whole node renders on one line: the exact
// surface string, its freeform group when set, and its description.
func writeModuleAPIs(w io.Writer, apis []Node) {
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

// builtinMarkdownHeadings maps a module-scoped, content-bearing node type to
// the fixed English heading arch_markdown_renderer.md's Document Structure
// gives it. A type absent here (data_flow, or any profile-declared type
// beyond the built-in five) gets a heading humanized from its plural key
// instead — data_flow's own plural key "data_flows" already humanizes to
// "Data Flows", so only "component" needs an override.
var builtinMarkdownHeadings = map[string]string{
	"component": "Architecture",
}

// writeModuleContentSections renders one "###" heading per module-scoped,
// content-bearing node type the resolved profile declares, in profile
// declaration order — Architecture and Data Flows under the default
// profile, plus any profile-declared type beyond those two (see
// arch_markdown_renderer.md "Document Structure" and its Declared Type
// scenario). requirement and api are excluded — each already got its own
// fixed-heading treatment above — and so is test_section, this format's own
// excluded projection (it reaches the JSON output alone). A type with no
// nodes in this module contributes no heading at all.
func writeModuleContentSections(w io.Writer, mg *ModuleGraph, profile *schema.Profile) {
	if profile == nil {
		return
	}
	for _, nt := range profile.NodeTypes {
		if nt.Scope != "module" {
			continue
		}
		if nt.Name == "requirement" || nt.Name == "api" || nt.Name == "test_section" {
			continue
		}

		nodes := filterNodesByType(mg.Nodes, nt.Name)
		if len(nodes) == 0 {
			continue
		}

		heading, ok := builtinMarkdownHeadings[nt.Name]
		if !ok {
			heading = humanizeHeading(nt.PluralKey)
		}
		fmt.Fprintf(w, "### %s\n\n", heading)
		for _, n := range nodes {
			writeContentNode(w, mg, n)
		}
	}
}

// writeContentNode inlines n's content leaf, heading-adjusted, under its
// owning section — or, when n declares no content file, a bare "####
// <name>" with nothing beneath it.
func writeContentNode(w io.Writer, mg *ModuleGraph, n Node) {
	if n.Content != "" {
		if content, ok := mg.Content[n.Content]; ok && content != "" {
			fmt.Fprintf(w, "%s\n", adjustHeadings(content, 3))
		}
		return
	}
	fmt.Fprintf(w, "#### %s\n\n", n.Name)
}

// filterNodesByType returns the subset of nodes whose Type matches, in
// their original order.
func filterNodesByType(nodes []Node, typ string) []Node {
	var out []Node
	for _, n := range nodes {
		if n.Type == typ {
			out = append(out, n)
		}
	}
	return out
}

// humanizeHeading turns a profile-declared plural key ("endpoints",
// "data_flows") into a readable heading ("Endpoints", "Data Flows") by
// splitting on underscores and capitalizing each word.
func humanizeHeading(pluralKey string) string {
	words := strings.Split(pluralKey, "_")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
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
