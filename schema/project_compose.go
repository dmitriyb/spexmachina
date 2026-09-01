package schema

import (
	"encoding/json"
	"fmt"
)

// projectProfileArrays lists the project.json property keys supplied by the
// resolved profile rather than fixed in the frame — today's one
// project-scoped node type. modules and sections stay in the frame: modules
// is structurally required (minItems 1) and sections is the project's
// generic extension mechanism, neither is part of the declarable node-type
// vocabulary.
var projectProfileArrays = []string{"requirements"}

// ProjectNodeType describes one project-scoped node type as declared by the
// resolved profile: its name (used to key $defs and, elsewhere, to derive
// identity hashes as "project/<name>/<node name>"), the plural key naming
// its array property in project.json, and whether nodes of this type carry
// a content leaf.
type ProjectNodeType struct {
	Name            string
	PluralKey       string
	RequiresContent bool
}

// DefaultProjectNodeTypes returns the project-scoped node types of the
// built-in default profile: requirement alone. Composing the project
// schema from this set reproduces the shipped frame's requirements array.
func DefaultProjectNodeTypes() []ProjectNodeType {
	return []ProjectNodeType{
		{Name: "requirement", PluralKey: "requirements"},
	}
}

// defaultProjectArrayKeyByTypeName maps each built-in default type name to
// the plural key its array property carries in the shipped frame — the
// lookup that lets ComposeProjectSchema find a built-in type's original
// array property definition to restore unchanged.
func defaultProjectArrayKeyByTypeName() map[string]string {
	keys := make(map[string]string, len(projectProfileArrays))
	for _, t := range DefaultProjectNodeTypes() {
		keys[t.Name] = t.PluralKey
	}
	return keys
}

// ComposeProjectSchema composes the effective project.json JSON Schema from
// the shipped frame plus one array property per given project-scoped node
// type, keyed by its plural key. A type whose name matches the frame's
// built-in $defs entry (requirement) reuses that definition, and its array
// property, unchanged from the frame — declared edges sourced at a built-in
// type never reach it, since the built-in $defs are frame-fixed. Any other
// name gets a generic envelope definition — an identity-hash id, a
// non-empty name, and, only when RequiresContent is set, a required content
// path — mirroring the module schema's generic envelope, plus one array-of-
// identity-hash property per given edge whose source names that type, and a
// synthesized array property description. additionalProperties:false at the
// root, inherited unchanged from the frame, is what rejects any array no
// passed type declares; the same constraint at the entry level, applied by
// genericProjectNodeDef, is what rejects any field that is neither envelope
// nor declared edge. Composing with DefaultProjectNodeTypes and
// DefaultProfile's edges reproduces the shipped project.schema.json.
func ComposeProjectSchema(types []ProjectNodeType, edges []Edge) ([]byte, error) {
	frame, originalArrays, err := projectSchemaFrame()
	if err != nil {
		return nil, fmt.Errorf("schema: compose project schema: %w", err)
	}

	props, ok := frame["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema: compose project schema: frame has no properties object")
	}
	defs, ok := frame["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema: compose project schema: frame has no $defs object")
	}
	defaultKeys := defaultProjectArrayKeyByTypeName()

	for _, t := range types {
		_, declared := defs[t.Name]
		if !declared {
			defs[t.Name] = genericProjectNodeDef(t, edgesSourcedAt(edges, t.Name))
		}
		if declared {
			if origKey, ok := defaultKeys[t.Name]; ok {
				if orig, ok := originalArrays[origKey]; ok {
					props[t.PluralKey] = orig
					continue
				}
			}
		}
		props[t.PluralKey] = map[string]any{
			"type":        "array",
			"description": fmt.Sprintf("%s entries declared by the resolved profile.", t.Name),
			"items":       map[string]any{"$ref": "#/$defs/" + t.Name},
		}
	}

	out, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("schema: compose project schema: marshal: %w", err)
	}
	return out, nil
}

// projectSchemaFrame loads the shipped project.schema.json and strips the
// profile-supplied array properties, leaving the envelope fields (name,
// description, version, modules, sections), the identity-hash pattern,
// additionalProperties:false, and the $defs library of built-in node-type
// shapes — the frame a resolved profile composes against. It also returns
// the stripped array properties, keyed by their original plural key, so a
// built-in type can have its original array property restored unchanged
// rather than overwritten with a synthesized one.
func projectSchemaFrame() (map[string]any, map[string]any, error) {
	data, err := projectSchemaBytes()
	if err != nil {
		return nil, nil, err
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, nil, fmt.Errorf("unmarshal project schema: %w", err)
	}
	props, ok := frame["properties"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("project schema has no properties object")
	}
	original := make(map[string]any, len(projectProfileArrays))
	for _, key := range projectProfileArrays {
		if v, ok := props[key]; ok {
			original[key] = v
		}
		delete(props, key)
	}
	return frame, original, nil
}

// genericProjectNodeDef synthesizes the JSON Schema definition for a
// project-scoped node type the shipped frame does not already define: an
// identity-hash id, a non-empty name, an optional description, and — only
// when the type requires a content leaf — a required content path. Mirrors
// the module schema's generic envelope (module_compose.go's
// genericNodeDef). edges supplies one additional property per
// profile-declared edge kind sourced at this type — an optional array of
// identity-hash values, matching the shape every built-in edge field
// (depends_on, requires_module) already carries. additionalProperties:false
// makes declaring an edge the only way to open a reference field on the
// type: any other field is rejected.
func genericProjectNodeDef(t ProjectNodeType, edges []Edge) map[string]any {
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
			"description": "Relative path to the markdown content leaf.",
		}
		required = append(required, "content")
	}
	for _, e := range edges {
		properties[e.Kind] = map[string]any{
			"type":        "array",
			"description": fmt.Sprintf("Identity hashes this %s references via the %s edge.", t.Name, e.Kind),
			"items": map[string]any{
				"type":    "string",
				"pattern": identityHashPattern,
			},
			"uniqueItems": true,
		}
	}

	return map[string]any{
		"type":                 "object",
		"required":             required,
		"additionalProperties": false,
		"properties":           properties,
	}
}
