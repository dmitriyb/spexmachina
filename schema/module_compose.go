package schema

import (
	"encoding/json"
	"fmt"
)

// moduleProfileArrays lists the module.json property keys supplied by the
// resolved profile rather than fixed in the frame — today's five
// module-scoped node types.
var moduleProfileArrays = []string{"requirements", "components", "data_flows", "test_sections", "apis"}

// ModuleNodeType describes one module-scoped node type as declared by the
// resolved profile: its name (used to key $defs and, elsewhere, to derive
// identity hashes as "<module>/<name>/<node name>"), the plural key naming
// its array property in module.json, and whether nodes of this type carry a
// content leaf.
type ModuleNodeType struct {
	Name            string
	PluralKey       string
	RequiresContent bool
}

// DefaultModuleNodeTypes returns the module-scoped node types of the
// built-in default profile: requirement, component, data_flow, test_section
// and api, in that order. Composing the module schema from this set
// reproduces the shipped frame's five arrays.
func DefaultModuleNodeTypes() []ModuleNodeType {
	return []ModuleNodeType{
		{Name: "requirement", PluralKey: "requirements"},
		{Name: "component", PluralKey: "components", RequiresContent: true},
		{Name: "data_flow", PluralKey: "data_flows", RequiresContent: true},
		{Name: "test_section", PluralKey: "test_sections", RequiresContent: true},
		{Name: "api", PluralKey: "apis"},
	}
}

// ComposeModuleSchema composes the effective module.json JSON Schema from
// the shipped frame plus one array property per given module-scoped node
// type, keyed by its plural key. A type whose name matches one of the
// frame's built-in $defs (requirement, component, data_flow, test_section,
// api) reuses that definition unchanged. Any other name gets a generic
// envelope definition — an identity-hash id, a non-empty name, and, only
// when RequiresContent is set, a required content path — the same
// constraints today's components array enforces. additionalProperties:false
// at the root, inherited unchanged from the frame, is what rejects any
// array no passed type declares. Composing with DefaultModuleNodeTypes
// reproduces the shipped module.schema.json.
func ComposeModuleSchema(types []ModuleNodeType) ([]byte, error) {
	frame, err := moduleSchemaFrame()
	if err != nil {
		return nil, fmt.Errorf("schema: compose module schema: %w", err)
	}

	props, ok := frame["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema: compose module schema: frame has no properties object")
	}
	defs, ok := frame["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema: compose module schema: frame has no $defs object")
	}

	for _, t := range types {
		if _, declared := defs[t.Name]; !declared {
			defs[t.Name] = genericNodeDef(t)
		}
		props[t.PluralKey] = map[string]any{
			"type":        "array",
			"description": fmt.Sprintf("%s entries declared by the resolved profile.", t.Name),
			"items":       map[string]any{"$ref": "#/$defs/" + t.Name},
		}
	}

	out, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("schema: compose module schema: marshal: %w", err)
	}
	return out, nil
}

// moduleSchemaFrame loads the shipped module.schema.json and strips the
// profile-supplied array properties, leaving the envelope fields, the
// identity-hash pattern, additionalProperties:false, and the $defs library
// of built-in node-type shapes — the frame a resolved profile composes
// against.
func moduleSchemaFrame() (map[string]any, error) {
	data, err := ModuleSchema()
	if err != nil {
		return nil, err
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal module schema: %w", err)
	}
	props, ok := frame["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("module schema has no properties object")
	}
	for _, key := range moduleProfileArrays {
		delete(props, key)
	}
	return frame, nil
}

// genericNodeDef synthesizes the JSON Schema definition for a module-scoped
// node type the shipped frame does not already define: an identity-hash id,
// a non-empty name, an optional description, and — only when the type
// requires a content leaf — a required content path. A type that does not
// require content gets no content property at all, the same shape an api
// entry has: there is nowhere to put a markdown leaf.
func genericNodeDef(t ModuleNodeType) map[string]any {
	properties := map[string]any{
		"id": map[string]any{
			"type":        "string",
			"pattern":     identityHashPattern,
			"description": fmt.Sprintf("Identity hash ID (12-char hex), unique within the %s array.", t.PluralKey),
		},
		"name": map[string]any{
			"type":        "string",
			"minLength":   1,
			"description": fmt.Sprintf("%s name.", t.Name),
		},
		"description": map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("%s description.", t.Name),
		},
	}
	required := []any{"id", "name"}
	if t.RequiresContent {
		properties["content"] = map[string]any{
			"type":        "string",
			"minLength":   1,
			"description": "Relative path to the markdown content leaf (described_in edge to content leaf).",
		}
		required = append(required, "content")
	}

	return map[string]any{
		"type":                 "object",
		"required":             required,
		"additionalProperties": false,
		"properties":           properties,
	}
}
