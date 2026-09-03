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
// its array property in project.json, whether nodes of this type carry a
// content leaf, the fields it declares beyond the fixed envelope, and an
// optional $comment on the composed $defs entry itself (arch_project_schema.md's
// "each copy carries a $comment naming the other" — the way Description
// reaches a property, Comment reaches the entry definition it annotates).
type ProjectNodeType struct {
	Name            string
	PluralKey       string
	RequiresContent bool
	Fields          []Field
	Comment         string
}

// DefaultProjectNodeTypes returns the project-scoped node types of the
// built-in default profile: requirement alone, its non-envelope properties
// declared as Fields — the type enumeration, the priority bounds, the
// single-value derivation enumeration, and depends_on (a reference field;
// the depends_on edge in DefaultProfile's edge list still carries the same
// kind, for every non-schema consumer of the profile's edge declarations).
// Composing the project schema from this set reproduces the shipped frame's
// requirements array and $defs/requirement.
func DefaultProjectNodeTypes() []ProjectNodeType {
	zero, four := 0, 4
	return []ProjectNodeType{
		{
			Name:      "requirement",
			PluralKey: "requirements",
			Comment:   "Project-level requirement. module.schema.json's copy carries preq_id in place of priority and derivation, since a module requirement derives by construction through its required preq_id. IDs are 12-char hex identity hashes computed by schema.IdentityHash.",
			Fields: []Field{
				{
					Name:        "type",
					Kind:        FieldKindText,
					Required:    true,
					Enum:        []string{"functional", "non_functional"},
					Description: "Requirement type.",
				},
				{
					Name:        "priority",
					Kind:        FieldKindInteger,
					Minimum:     &zero,
					Maximum:     &four,
					Description: "Requirement priority (0-4). Optional; enforced by validator when present.",
				},
				{
					Name:        "derivation",
					Kind:        FieldKindText,
					Enum:        []string{"pending"},
					Description: "Declares a requirement not yet derived into any module. Project-scoped only: a module requirement derives by construction through its required preq_id.",
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
// the shipped frame plus one array property and one $defs entry per given
// project-scoped node type, keyed by its plural key and its name. Every
// type — the built-in requirement included — gets a generic envelope
// definition composed by genericProjectNodeDef: an identity-hash id, a
// non-empty name, an optional description, and, only when RequiresContent
// is set, a required content path, exactly as the module schema's
// composition does for module-scoped types; the built-in $defs the shipped
// frame used to carry verbatim are frame-fixed no longer. Beyond the
// envelope, each entry definition carries one property per declared field —
// typed, bounded, enum-constrained, identity-hash-patterned for references —
// plus one array-of-identity-hash property per edge whose source names that
// type; a field and an edge sharing a name compose the same property twice,
// the field's shape winning since its loop runs last. The requirement type's
// own non-envelope properties are the default profile's field declarations
// materialized this way (DefaultProjectNodeTypes), so composing with them
// and DefaultProfile's edges reproduces the shipped project.schema.json's
// $defs/requirement exactly. The array property at the root, by contrast,
// is restored unchanged from the frame for a built-in type name — its
// shipped wording is hand-authored, not synthesized — and only synthesized
// for a name the frame does not already carry. additionalProperties:false
// at the root, inherited unchanged from the frame, is what rejects any
// array no passed type declares; the same constraint at the entry level,
// applied by genericProjectNodeDef, is what rejects any property that is
// neither envelope, declared edge, nor declared field.
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
	scopedEdges := projectScopedEdges(edges)

	for _, t := range types {
		defs[t.Name] = genericProjectNodeDef(t, edgesSourcedAt(scopedEdges, t.Name))

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
		return nil, fmt.Errorf("schema: compose project schema: marshal: %w", err)
	}
	return out, nil
}

// projectScopedEdges drops the preq_id edge before project-scope
// composition sources reference fields from an edge list: preq_id's From
// names "requirement" the same bare way depends_on's does, but requirement
// is declared once per scope (project and module) and preq_id links a
// module requirement to its parent project requirement — module-scope-only,
// the one property project.schema.json's requirement definition has always
// rejected (E8). Every other edge kind whose From names "requirement" is
// genuinely project-scoped. The module side never hits this ambiguity: its
// requirement $defs entry composes preq_id by hand, not through an edge.
func projectScopedEdges(edges []Edge) []Edge {
	out := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if e.Kind == "preq_id" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// projectSchemaFrame loads the shipped project.schema.json and strips the
// profile-supplied array properties and the built-in project-scoped $defs
// entries (today, requirement alone), leaving the envelope fields (name,
// description, version, modules, sections), the identity-hash pattern,
// additionalProperties:false, and the $defs entries no profile-declared
// type composes (identityHash, module, section) — the frame a resolved
// profile composes against. No built-in $defs entry for a project-scoped
// node type remains in it: ComposeProjectSchema resynthesizes every one,
// requirement included, from the given types. It also returns the stripped
// array properties, keyed by their original plural key, so a built-in
// type's array property — whose wording is hand-authored, not synthesized —
// can be restored unchanged rather than overwritten with a generic one.
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
	if defs, ok := frame["$defs"].(map[string]any); ok {
		for _, t := range DefaultProjectNodeTypes() {
			delete(defs, t.Name)
		}
	}
	return frame, original, nil
}

// genericProjectNodeDef synthesizes the JSON Schema definition for a
// project-scoped node type: an identity-hash id, a non-empty name, an
// optional description, and — only when the type requires a content leaf —
// a required content path. Mirrors the module schema's generic envelope
// (module_compose.go's genericNodeDef): both are every type's path through
// composition — built-in and profile-declared alike, since neither schema
// keeps a frame-fixed $defs entry. edges supplies
// one additional property per profile-declared edge kind sourced at this
// type — an optional array of identity-hash values, matching the shape
// every built-in edge field (depends_on, requires_module) already carries —
// and t.Fields supplies one further property per declared field, shaped by
// composeFieldSchema; a field wins over an edge of the same name, since its
// loop runs after. additionalProperties:false makes declaring an edge or a
// field the only way to open a property on the type beyond the envelope:
// any other property is rejected. required lists id first, then the
// declared fields marked Required in field order, then name (and content,
// when RequiresContent is set) — the order the requirement type's own
// required set (id, type, name) needs to reproduce the shipped frame. When
// t.Comment is set, it becomes the entry definition's own "$comment" — the
// requirement type's two scope declarations each name the other's file
// (arch_project_schema.md's Design Rationale), materialized the same way
// Description reaches a property.
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
