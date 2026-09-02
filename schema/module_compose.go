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
// its array property in module.json, whether nodes of this type carry a
// content leaf, and the fields it declares beyond the fixed envelope.
type ModuleNodeType struct {
	Name            string
	PluralKey       string
	RequiresContent bool
	Fields          []Field
}

// FieldKind is the kind of one field a profile-declared node type carries
// beyond its fixed envelope — see arch_module_schema.md's "Each type's
// declared fields reach the composed document too".
type FieldKind string

const (
	FieldKindText      FieldKind = "text"
	FieldKindInteger   FieldKind = "integer"
	FieldKindReference FieldKind = "reference"
)

// Field declares one field a profile-declared node type carries beyond its
// envelope. It composes into one property on that type's entry definition:
// enum-constrained for a text field carrying an enumeration, bounded for an
// integer field carrying bounds, an identity-hash value for a reference
// field — a scalar at cardinality "one", an array of identity hashes
// otherwise — with a required text field composed non-empty.
//
// A reference-kind field is the edge declaration: Targets names the node
// types it may point at, and Cyclic exempts it from the DAG cycle check
// (an omitted/false value means cycle-checked) — the separate "edges"
// section of the earlier profile format is unified into this field list, so
// one declaration answers what a type may carry, what it may point at, and
// whether it counts as a semantic change. Hashed controls whether the field
// participates in its node's identity/content hash; nil (the JSON field
// absent) means true, matching every declared field's default.
type Field struct {
	Name        string    `json:"name"`
	Kind        FieldKind `json:"kind"`
	Required    bool      `json:"required,omitempty"`
	Hashed      *bool     `json:"hashed,omitempty"`      // nil means true (the default); false opts the field out of hashing
	Enum        []string  `json:"enum,omitempty"`        // text field: permitted values, when non-empty
	Minimum     *int      `json:"minimum,omitempty"`     // integer field: inclusive lower bound, when set
	Maximum     *int      `json:"maximum,omitempty"`     // integer field: inclusive upper bound, when set
	Targets     []string  `json:"targets,omitempty"`     // reference field: permitted target node type names
	Cardinality string    `json:"cardinality,omitempty"` // reference field: "one" or "many" (default "many")
	Cyclic      bool      `json:"cyclic,omitempty"`      // reference field: true exempts this edge from the DAG cycle check
	Description string    `json:"description,omitempty"` // composed property's "description", when non-empty
}

// HashParticipates reports whether f participates in its node's hash: every
// declared field does unless it sets Hashed: false.
func (f Field) HashParticipates() bool {
	return f.Hashed == nil || *f.Hashed
}

// DefaultModuleNodeTypes returns the module-scoped node types of the
// built-in default profile: requirement, component, data_flow, test_section
// and api, in that order, name and plural key only. Composing the module
// schema from this set reproduces the shipped frame's five arrays: each
// name already resolves to the frame's own $defs entry, so ComposeModuleSchema
// reuses that entry's properties unchanged rather than synthesizing from
// Fields. The five built-in types' actual field declarations — the single
// record of that policy — live in the embedded defaultProfile.json, resolved
// through DefaultProfile().ModuleNodeTypes(); this function exists only to
// give defaultArrayKeyByTypeName the name-to-plural-key lookup it needs to
// restore a built-in type's hand-authored array property unchanged.
func DefaultModuleNodeTypes() []ModuleNodeType {
	return []ModuleNodeType{
		{Name: "requirement", PluralKey: "requirements"},
		{Name: "component", PluralKey: "components", RequiresContent: true},
		{Name: "data_flow", PluralKey: "data_flows", RequiresContent: true},
		{Name: "test_section", PluralKey: "test_sections", RequiresContent: true},
		{Name: "api", PluralKey: "apis"},
	}
}

// defaultArrayKeyByTypeName maps each built-in default type name to the
// plural key its array property carries in the shipped frame — the lookup
// that lets ComposeModuleSchema find a built-in type's original array
// property definition to restore unchanged.
func defaultArrayKeyByTypeName() map[string]string {
	keys := make(map[string]string, len(moduleProfileArrays))
	for _, t := range DefaultModuleNodeTypes() {
		keys[t.Name] = t.PluralKey
	}
	return keys
}

// ComposeModuleSchema composes the effective module.json JSON Schema from
// the shipped frame plus one array property per given module-scoped node
// type, keyed by its plural key. A type whose name matches one of the
// frame's built-in $defs (requirement, component, data_flow, test_section,
// api) reuses that definition, and its array property, unchanged from the
// frame — declared edges sourced at a built-in type never reach it, since
// the built-in $defs are frame-fixed. Any other name gets a generic
// envelope definition — an identity-hash id, a non-empty name, and, only
// when RequiresContent is set, a required content path — the same
// constraints today's components array enforces, plus one array-of-
// identity-hash property per given edge whose source names that type, and
// a synthesized array property description. additionalProperties:false at
// the root, inherited unchanged from the frame, is what rejects any array
// no passed type declares; the same constraint at the entry level, applied
// by genericNodeDef, is what rejects any property that is neither envelope,
// declared edge, nor declared field. Composing with DefaultModuleNodeTypes
// and DefaultProfile's edges reproduces the shipped module.schema.json.
func ComposeModuleSchema(types []ModuleNodeType, edges []Edge) ([]byte, error) {
	frame, originalArrays, err := moduleSchemaFrame()
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
	defaultKeys := defaultArrayKeyByTypeName()

	for _, t := range types {
		existing, declared := defs[t.Name]
		if !declared {
			defs[t.Name] = genericNodeDef(t, edgesSourcedAt(edges, t.Name))
		} else if defMap, ok := existing.(map[string]any); ok {
			mergeDeclaredFields(defMap, t.Fields)
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
		return nil, fmt.Errorf("schema: compose module schema: marshal: %w", err)
	}
	return out, nil
}

// moduleSchemaFrame loads the shipped module.schema.json and strips the
// profile-supplied array properties, leaving the envelope fields, the
// identity-hash pattern, additionalProperties:false, and the $defs library
// of built-in node-type shapes — the frame a resolved profile composes
// against. It also returns the stripped array properties, keyed by their
// original plural key, so a built-in type can have its original array
// property restored unchanged rather than overwritten with a synthesized
// one.
func moduleSchemaFrame() (map[string]any, map[string]any, error) {
	data, err := moduleSchemaBytes()
	if err != nil {
		return nil, nil, err
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, nil, fmt.Errorf("unmarshal module schema: %w", err)
	}
	props, ok := frame["properties"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("module schema has no properties object")
	}
	original := make(map[string]any, len(moduleProfileArrays))
	for _, key := range moduleProfileArrays {
		if v, ok := props[key]; ok {
			original[key] = v
		}
		delete(props, key)
	}
	return frame, original, nil
}

// mergeDeclaredFields composes one property per declared field into the
// definition, from the field declaration — the same composeFieldSchema path
// any declared type's fields take — overwriting whatever property the
// shipped frame's $defs entry carried under that name. A built-in type's
// field shape is therefore load-bearing: a profile that redeclares an
// existing built-in field (e.g. adding an enum to api's "group") reaches the
// composed schema, not just a wholly new field name the frame never gave the
// type. This is what retires the earlier rule that a built-in type's $defs
// entry is frame-fixed (test_schema_loading.md's P6) — there is no
// frame-fixed definition left for a declared field, new or pre-existing, to
// be unable to reach. required preserves the frame's original order for a
// name it already lists — required is an unordered set semantically, and
// reordering it would only churn the golden comparison — appending only the
// field names newly required that the frame's entry did not already list.
func mergeDeclaredFields(def map[string]any, fields []Field) {
	props, ok := def["properties"].(map[string]any)
	if !ok {
		return
	}
	required, _ := def["required"].([]any)
	alreadyRequired := make(map[string]bool, len(required))
	for _, r := range required {
		if name, ok := r.(string); ok {
			alreadyRequired[name] = true
		}
	}
	for _, f := range fields {
		props[f.Name] = composeFieldSchema(f)
		if f.Required && !alreadyRequired[f.Name] {
			required = append(required, f.Name)
			alreadyRequired[f.Name] = true
		}
	}
	def["required"] = required
}

// edgesSourcedAt returns the edges whose From list names typeName, in
// declaration order — the edge kinds a synthesized entry definition for
// typeName gains as reference-field properties. An edge whose From does not
// name typeName contributes nothing to that type's definition, even if it
// shares a Kind with one that does.
func edgesSourcedAt(edges []Edge, typeName string) []Edge {
	var out []Edge
	for _, e := range edges {
		for _, from := range e.From {
			if from == typeName {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// genericNodeDef synthesizes the JSON Schema definition for a module-scoped
// node type the shipped frame does not already define: an identity-hash id,
// a non-empty name, an optional description, and — only when the type
// requires a content leaf — a required content path. A type that does not
// require content gets no content property at all, the same shape an api
// entry has: there is nowhere to put a markdown leaf. edges supplies one
// additional property per profile-declared edge kind sourced at this type —
// an optional array of identity-hash values, matching the shape every
// built-in edge field (implements, uses, describes, provided_by) already
// carries. t.Fields supplies one further property per declared field —
// composeFieldSchema shapes it by kind, including a reference field, which
// opens a property the same way an edge does. additionalProperties:false
// makes declaring an edge or a field the only way to open a property on the
// type beyond the envelope: any other property is rejected.
func genericNodeDef(t ModuleNodeType, edges []Edge) map[string]any {
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
	for _, f := range t.Fields {
		properties[f.Name] = composeFieldSchema(f)
		if f.Required {
			required = append(required, f.Name)
		}
	}

	return map[string]any{
		"type":                 "object",
		"required":             required,
		"additionalProperties": false,
		"properties":           properties,
	}
}

// composeFieldSchema builds the JSON Schema property definition for one
// profile-declared [Field]: enum-constrained for a text field carrying an
// enumeration, bounded for an integer field carrying bounds, an
// identity-hash value for a reference field — a scalar at cardinality
// "one", an array of identity hashes otherwise (uniqueItems, matching the
// shape a declared edge field already carries) — with a required text field
// composed non-empty. A required text field carrying an enumeration skips
// minLength: the enum already excludes the empty string, so the bound would
// be redundant. Description, when set, becomes the property's "description".
func composeFieldSchema(f Field) map[string]any {
	var prop map[string]any
	switch f.Kind {
	case FieldKindInteger:
		prop = map[string]any{"type": "integer"}
		if f.Minimum != nil {
			prop["minimum"] = *f.Minimum
		}
		if f.Maximum != nil {
			prop["maximum"] = *f.Maximum
		}
	case FieldKindReference:
		if f.Cardinality == "one" {
			prop = map[string]any{
				"type":    "string",
				"pattern": identityHashPattern,
			}
		} else {
			prop = map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string", "pattern": identityHashPattern},
				"uniqueItems": true,
			}
		}
	default: // FieldKindText
		prop = map[string]any{"type": "string"}
		if f.Required && len(f.Enum) == 0 {
			prop["minLength"] = 1
		}
		if len(f.Enum) > 0 {
			enum := make([]any, len(f.Enum))
			for i, v := range f.Enum {
				enum[i] = v
			}
			prop["enum"] = enum
		}
	}
	if f.Description != "" {
		prop["description"] = f.Description
	}
	return prop
}
