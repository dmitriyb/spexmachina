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
// content leaf, the fields it declares beyond the fixed envelope, and an
// optional $comment on the composed $defs entry itself — mirrors
// ProjectNodeType.Comment (project_compose.go); the built-in requirement and
// api types are the ones that carry one.
type ModuleNodeType struct {
	Name            string
	PluralKey       string
	RequiresContent bool
	Fields          []Field
	Comment         string
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
// and api, in that order, their non-envelope properties declared as
// Fields — the reference kinds preq_id, depends_on, implements, uses,
// describes and provided_by, plus api's group field. This duplicates the
// embedded defaultProfile.json's module-scoped declarations (also reachable
// through DefaultProfile().ModuleNodeTypes()); it exists so callers can
// compose the module schema, and defaultArrayKeyByTypeName can look up a
// built-in type's plural key, without resolving a profile. Composing with
// this set and DefaultProfile's edges reproduces the shipped
// module.schema.json — every built-in type's $defs entry included, since no
// module-scoped $defs entry is frame-fixed. Mirrors DefaultProjectNodeTypes
// (project_compose.go).
func DefaultModuleNodeTypes() []ModuleNodeType {
	return []ModuleNodeType{
		{
			Name:      "requirement",
			PluralKey: "requirements",
			Comment:   "Module-level requirement. Extends the project-level requirement with preq_id to trace derivation from project requirements. IDs are 12-char hex identity hashes computed by schema.IdentityHash. name (renamed from title in spec format version 1) makes the envelope universal across every node type.",
			Fields: []Field{
				{
					Name:        "type",
					Kind:        FieldKindText,
					Required:    true,
					Enum:        []string{"functional", "non_functional"},
					Description: "Requirement type.",
				},
				{
					Name:        "preq_id",
					Kind:        FieldKindReference,
					Required:    true,
					Targets:     []string{"requirement"},
					Cardinality: "one",
					Description: "Identity hash of the project requirement this module requirement derives from.",
				},
				{
					Name:        "depends_on",
					Kind:        FieldKindReference,
					Targets:     []string{"requirement"},
					Cardinality: "many",
					Description: "Identity hashes of other requirements this one depends on (depends_on edge).",
				},
			},
		},
		{
			Name:            "component",
			PluralKey:       "components",
			RequiresContent: true,
			Fields: []Field{
				{
					Name:        "implements",
					Kind:        FieldKindReference,
					Targets:     []string{"requirement"},
					Cardinality: "many",
					Description: "Requirement identity hashes this component implements (implements edge).",
				},
				{
					Name:        "uses",
					Kind:        FieldKindReference,
					Targets:     []string{"component"},
					Cardinality: "many",
					Description: "Identity hashes of other components this one depends on (uses edge).",
				},
			},
		},
		{
			Name:            "data_flow",
			PluralKey:       "data_flows",
			RequiresContent: true,
			Fields: []Field{
				{
					Name:        "uses",
					Kind:        FieldKindReference,
					Targets:     []string{"component"},
					Cardinality: "many",
					Description: "Component identity hashes involved in this data flow (uses edge).",
				},
			},
		},
		{
			Name:            "test_section",
			PluralKey:       "test_sections",
			RequiresContent: true,
			Fields: []Field{
				{
					Name:        "describes",
					Kind:        FieldKindReference,
					Targets:     []string{"component"},
					Cardinality: "many",
					Description: "Component identity hashes described by this test section (describes edge).",
				},
			},
		},
		{
			Name:      "api",
			PluralKey: "apis",
			Comment:   "An external surface entry point. No content file — an api hashes from these fields alone. Identity is <module>/api/<name>, computed by schema.IdentityHash.",
			Fields: []Field{
				{
					Name:        "provided_by",
					Kind:        FieldKindReference,
					Targets:     []string{"component"},
					Cardinality: "many",
					Description: "Identity hashes of components in this module that provide this entry point (module-local).",
				},
				{
					Name:        "group",
					Kind:        FieldKindText,
					Description: "Freeform grouping label for renderers (e.g. \"cli\", \"http\"). Spex never branches on it.",
				},
			},
		},
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
// the shipped frame plus one array property and one $defs entry per given
// module-scoped node type, keyed by its plural key and its name. Every
// type — the five built-in types included — gets a generic envelope
// definition composed by genericNodeDef: an identity-hash id, a non-empty
// name, an optional description, and, only when RequiresContent is set, a
// required content path, exactly as the project schema's composition does
// for project-scoped types; no module-scoped $defs entry is frame-fixed.
// Beyond the envelope, each entry definition carries one property per
// declared field — typed, bounded, enum-constrained, identity-hash-patterned
// for references — plus one array-of-identity-hash property per edge whose
// source names that type; a field and an edge sharing a name compose the
// same property twice, the field's shape winning since its loop runs last.
// The five built-in types' own non-envelope properties are the default
// profile's field declarations materialized this way
// (DefaultModuleNodeTypes), so composing with them and DefaultProfile's
// edges reproduces the shipped module.schema.json's $defs entries exactly.
// The array property at the root, by contrast, is restored unchanged from
// the frame for a built-in type name — its shipped wording is
// hand-authored, not synthesized — and only synthesized for a name the
// frame does not already carry. additionalProperties:false at the root,
// inherited unchanged from the frame, is what rejects any array no passed
// type declares; the same constraint at the entry level, applied by
// genericNodeDef, is what rejects any property that is neither envelope,
// declared edge, nor declared field.
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
		defs[t.Name] = genericNodeDef(t, edgesSourcedAt(edges, t.Name))

		if origKey, ok := defaultKeys[t.Name]; ok {
			if orig, ok := originalArrays[origKey]; ok {
				props[t.PluralKey] = orig
				continue
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
// profile-supplied array properties and the built-in module-scoped $defs
// entries (today, requirement, component, data_flow, test_section and api),
// leaving the envelope fields, the identity-hash pattern,
// additionalProperties:false, and nothing else — the frame a resolved
// profile composes against. No built-in $defs entry remains in it:
// ComposeModuleSchema resynthesizes every one, the five built-ins included,
// from the given types. It also returns the stripped array properties,
// keyed by their original plural key, so a built-in type's array
// property — whose wording is hand-authored, not synthesized — can be
// restored unchanged rather than overwritten with a generic one. Mirrors
// projectSchemaFrame (project_compose.go).
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
	if defs, ok := frame["$defs"].(map[string]any); ok {
		for _, t := range DefaultModuleNodeTypes() {
			delete(defs, t.Name)
		}
	}
	return frame, original, nil
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
// node type: an identity-hash id, a non-empty name, an optional
// description, and — only when the type requires a content leaf — a
// required content path. A type that does not require content gets no
// content property at all, the same shape an api entry has: there is
// nowhere to put a markdown leaf. Mirrors the project schema's generic
// envelope (project_compose.go's genericProjectNodeDef), and, unlike the
// earlier version of this function, is every module-scoped type's path
// through composition — built-in and profile-declared alike, since no
// module-scoped $defs entry is frame-fixed. edges supplies one additional
// property per profile-declared edge kind sourced at this type — an
// optional array of identity-hash values, matching the shape every built-in
// edge field (implements, uses, describes, provided_by) already carries —
// and t.Fields supplies one further property per declared field, shaped by
// composeFieldSchema; a field wins over an edge of the same name, since its
// loop runs after. additionalProperties:false makes declaring an edge or a
// field the only way to open a property on the type beyond the envelope:
// any other property is rejected. required lists id first, then the
// declared fields marked Required in field order, then name (and content,
// when RequiresContent is set) — the order the requirement type's own
// required set (id, type, preq_id, name) needs to reproduce the shipped
// frame. When t.Comment is set, it becomes the entry definition's own
// "$comment", materialized the same way Description reaches a property.
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
	required := []any{"id"}
	for _, f := range t.Fields {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	required = append(required, "name")
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
	}

	def := map[string]any{
		"type":                 "object",
		"required":             required,
		"additionalProperties": false,
		"properties":           properties,
	}
	if t.Comment != "" {
		def["$comment"] = t.Comment
	}
	return def
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
