package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// NodeType declares one node type in the resolved profile's vocabulary: its
// name, the plural key naming its array property in project.json or
// module.json, whether it is project- or module-scoped, whether it requires
// a content leaf, its two per-type role flags, and the fields it carries
// beyond the fixed envelope. "requirement" is declared twice — once per
// scope — because the project-level and module-level requirement arrays
// carry different envelope constraints even though they share a type name
// and both trigger the completeness rules. A reference-kind field in Fields
// is the edge declaration: the separate "edges" section of the earlier
// profile format is unified into this list, so one declaration per field
// answers what the type may carry, what it may point at, and whether it
// counts as a semantic change. Comment is project-scope-only: it becomes
// the composed $defs entry's own "$comment" (ComposeProjectSchema, via
// ProjectNodeType.Comment) — a module-scoped built-in's $defs entry carries
// its $comment from the shipped document instead, so no module-scoped type
// needs one here.
type NodeType struct {
	Name                string  `json:"name"`
	PluralKey           string  `json:"plural_key"`
	Scope               string  `json:"scope"` // "project" or "module"
	RequiresContent     bool    `json:"requires_content,omitempty"`
	CompletenessTrigger bool    `json:"completeness_trigger,omitempty"`
	NameDeclarable      bool    `json:"name_declarable,omitempty"`
	Fields              []Field `json:"fields,omitempty"`
	Comment             string  `json:"comment,omitempty"`
}

// Edge is a derived view of one legal edge kind: the reference-field name,
// the node types that declare it (From), and the node types it may point at
// (To). It is no longer part of the profile wire format — a reference-kind
// [Field] declared on a type is the edge declaration now — but every
// consumer written against the earlier edges section keeps working
// unchanged, since [Profile.Edges] is populated with an equivalent view
// once resolution succeeds. "module" is a legal From/To entry even though
// module is not a declarable node type: requires_module is the fixed
// interior-node concept a profile can never declare, so it is always
// present in this derived view regardless of what the profile says.
// CyclicFrom exempts one declaring type's occurrence of the edge kind from
// the DAG cycle check — 91270a8a2b57's per-field exemption flag, kept per
// declaring type since two types can declare a reference field of the same
// name with different Cyclic values (e.g. "uses" cyclic on data_flow but not
// on component); a name absent from the map, or mapped to false, is
// cycle-checked.
type Edge struct {
	Kind       string
	From       []string
	To         []string
	CyclicFrom map[string]bool
}

// CyclicForType reports whether fromType's own declaration of this edge kind
// is exempt from the DAG cycle check.
func (e Edge) CyclicForType(fromType string) bool {
	return e.CyclicFrom[fromType]
}

// CoverageChain declares one coverage rule: every node of CoveredType must
// be the target of at least one Edge-named field from some node of
// CoveringType — Edge names a reference-kind field declared on CoveringType
// (at CoveringScope, when given), exactly as it used to name a globally
// declared edge kind. The Scope fields disambiguate CoveredType/CoveringType
// only when the type name is shared across scopes (requirement); they are
// empty when the type has only one scope.
type CoverageChain struct {
	CoveredType   string `json:"covered_type"`
	CoveredScope  string `json:"covered_scope,omitempty"`
	Edge          string `json:"edge"`
	CoveringType  string `json:"covering_type"`
	CoveringScope string `json:"covering_scope,omitempty"`
}

// AbsorbDirections declares, for one node type, whether refresh may absorb
// an addition and whether it may absorb a removal without bead work.
type AbsorbDirections struct {
	Added   bool `json:"added"`
	Removed bool `json:"removed"`
}

// Profile is the resolved declaration of a project's spec vocabulary: the
// node types (with their fields, reference kinds included), and the graph
// rules — coverage chains, the plan-relevant set, the per-type impact-level
// mapping, and refresh's absorbable directions. It carries no behaviour —
// every consumer reads its own policy off this document rather than
// branching on type-name string literals.
//
// Edges and HashedFields are not part of the wire format: they are derived,
// once resolution succeeds, from the reference-kind and hash-participating
// fields NodeTypes declares. A profile document no longer authors an
// "edges" or "hashed_fields" section directly — declaring either as a
// top-level key is rejected as an unknown field, the same way any other
// pre-versioning document in the retired format is — but every package that
// reads Profile.Edges or Profile.HashedFields keeps working against these
// derived views unchanged.
type Profile struct {
	// ProfileVersion is nil when the field is absent from the document,
	// which means version 1 — the same as an explicit 1. A pointer is what
	// lets Validate tell an absent field apart from an explicit
	// "profile_version": 0, which is out of the supported range and must be
	// rejected rather than silently treated as absent.
	ProfileVersion *int                        `json:"profile_version,omitempty"`
	NodeTypes      []NodeType                  `json:"node_types"`
	CoverageChains []CoverageChain             `json:"coverage_chains"`
	PlanRelevant   []string                    `json:"plan_relevant"`
	ImpactLevels   map[string]string           `json:"impact_levels"`
	Absorbable     map[string]AbsorbDirections `json:"absorbable"`

	Edges        []Edge              `json:"-"`
	HashedFields map[string][]string `json:"-"`
}

//go:embed defaultProfile.json
var defaultProfileFS embed.FS

// Supported profile format versions. Version 1 is the field-declaration
// format this contract ships; an absent profile_version means version 1.
// A document declaring a version outside this range — or a pre-versioning
// document in the retired edges/hashed_fields format, which carries no
// profile_version at all and fails ordinary validation as malformed — is
// rejected before any other check runs.
const (
	minProfileVersion = 1
	maxProfileVersion = 1
)

// SupportedSpecVersion is the spec format version this binary supports,
// declared by project.json's spec_version field ([Project.SpecVersion]). An
// absent spec_version means version 1. Spec format version 1 carries one
// deliberate break: the requirement type's title field is renamed to name.
// Unlike profile_version, there is no range to consult — this binary speaks
// exactly one spec format version.
const SupportedSpecVersion = 1

// JournalLineVersion is the journal-line format version a writer stamps
// into the optional per-line "v" field; an absent v means version 1. The
// journal is append-only and permanent, so — unlike SupportedSpecVersion —
// this is a floor rather than an exact match: readers accept every version
// from JournalLineVersion forward, forever.
const JournalLineVersion = 1

// envelopeFieldNames are the fixed envelope property names no declared
// field may reuse: id, name and description on every type. "content" is
// deliberately not in this set — it is only part of a type's envelope when
// RequiresContent is set (arch_profile_loader.md's "Fixed points": "content
// where the type is content-bearing"), so its collision is checked
// separately, conditioned on that flag, rather than unconditionally here.
// The profile is a description of a vocabulary and cannot reach the
// envelope minimum, so a field colliding with one of these names is
// rejected outright rather than silently shadowing it.
var envelopeFieldNames = map[string]bool{
	"id": true, "name": true, "description": true,
}

// ResolveProfile resolves the profile for the project rooted at specDir: it
// reads "profile.json" beside "project.json" when present, or returns the
// built-in default profile when the file is absent — absence is the
// supported default, never an error. A present file is decoded strictly (no
// unknown top-level fields, so an attempt to declare a fixed point, or a
// document in the retired edges/hashed_fields format, is rejected) and
// validated before it is returned, so a malformed profile fails once,
// early, naming the file and the defect, rather than surfacing downstream
// as a cascade of schema-conformance errors.
func ResolveProfile(specDir string) (*Profile, error) {
	path := filepath.Join(specDir, "profile.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultProfile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("schema: resolve profile: read %s: %w", path, err)
	}

	p, err := decodeProfile(data)
	if err != nil {
		return nil, fmt.Errorf("schema: resolve profile: parse %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("schema: resolve profile: %s: %w", path, err)
	}
	p.finalize()
	return p, nil
}

// DefaultProfile returns the built-in default profile: today's ontology,
// resolved from the embedded defaultProfile.json — a document in the
// source tree, identical in format to what a project may commit as
// spec/profile.json, not hand-authored Go values. It is not a privileged
// code path: it is decoded and validated through the exact same
// decodeProfile/Validate steps ResolveProfile runs over a file-backed
// profile, which is what P9 in test_schema_loading.md pins. The embedded
// document is checked at build time (it ships in this package's own tests),
// so a decode or validation failure here is an internal invariant
// violation, not a runtime condition callers should handle — DefaultProfile
// keeps its long-standing no-error signature and panics instead.
func DefaultProfile() *Profile {
	data, err := defaultProfileFS.ReadFile("defaultProfile.json")
	if err != nil {
		panic(fmt.Sprintf("schema: read embedded defaultProfile.json: %v", err))
	}
	p, err := decodeProfile(data)
	if err != nil {
		panic(fmt.Sprintf("schema: decode embedded defaultProfile.json: %v", err))
	}
	if err := p.Validate(); err != nil {
		panic(fmt.Sprintf("schema: embedded defaultProfile.json is invalid: %v", err))
	}
	p.finalize()
	return p
}

// decodeProfile strictly decodes one profile document: no unknown top-level
// or nested fields, so a document in the retired edges/hashed_fields format,
// or one attempting to declare a fixed point, fails here rather than being
// silently accepted with the unrecognized data dropped.
func decodeProfile(data []byte) (*Profile, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p Profile
	if err := dec.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// finalize populates the derived Edges and HashedFields views from
// NodeTypes' declared fields. Called once resolution succeeds — never on a
// profile that failed Validate — so every consumer that reads these two
// views can assume they reflect a valid profile.
func (p *Profile) finalize() {
	p.Edges = deriveEdges(p.NodeTypes)
	p.HashedFields = deriveHashedFields(p.NodeTypes)
}

// deriveEdges rebuilds the earlier profile format's "edges" view from every
// reference-kind field NodeTypes declares, grouped by field name: a field
// name shared by two types (e.g. "uses" declared on both component and
// data_flow) folds into one Edge whose From lists both source types, the
// way one hand-authored edge kind used to — and whose CyclicFrom records
// each declaring type's own Cyclic flag independently, so one type's
// exemption never overrides or is overridden by another's declaration of
// the same-named field. requires_module is not sourced by any declarable
// field — module is never a declarable node type, and requires_module is
// the frame's own fixed edge — so it is appended unconditionally, present
// in every resolved profile's derived view regardless of what NodeTypes
// declares, and always cycle-checked (module is absent from its CyclicFrom).
func deriveEdges(nodeTypes []NodeType) []Edge {
	order := make([]string, 0)
	byKind := map[string]*Edge{}
	for _, t := range nodeTypes {
		for _, f := range t.Fields {
			if f.Kind != FieldKindReference {
				continue
			}
			e, ok := byKind[f.Name]
			if !ok {
				e = &Edge{Kind: f.Name, CyclicFrom: map[string]bool{}}
				byKind[f.Name] = e
				order = append(order, f.Name)
			}
			if !slices.Contains(e.From, t.Name) {
				e.From = append(e.From, t.Name)
			}
			e.CyclicFrom[t.Name] = f.Cyclic
			for _, target := range f.Targets {
				if !slices.Contains(e.To, target) {
					e.To = append(e.To, target)
				}
			}
		}
	}

	edges := make([]Edge, 0, len(order)+1)
	for _, kind := range order {
		edges = append(edges, *byKind[kind])
	}
	edges = append(edges, Edge{Kind: "requires_module", From: []string{"module"}, To: []string{"module"}, CyclicFrom: map[string]bool{}})
	return edges
}

// deriveHashedFields rebuilds the earlier profile format's per-(scope,name)
// hashed-field allowlists: the envelope fields that hash today (id, name,
// description) plus every declared field that has not opted out via
// Hashed: false. A content-bearing type gets no entry — its leaf hashes
// from its content file, not its JSON fields, exactly as before.
func deriveHashedFields(nodeTypes []NodeType) map[string][]string {
	out := make(map[string][]string, len(nodeTypes))
	for _, t := range nodeTypes {
		if t.RequiresContent {
			continue
		}
		names := []string{"id", "name", "description"}
		for _, f := range t.Fields {
			if !f.HashParticipates() {
				continue
			}
			names = append(names, f.Name)
		}
		sort.Strings(names)
		out[t.Scope+":"+t.Name] = names
	}
	return out
}

// findField returns the field named fieldName declared on the node type
// named typeName — at the given scope when scope is non-empty, at either
// scope otherwise — the lookup CoverageChain.Edge resolves against, since a
// coverage chain now names a field on its declaring type rather than a
// globally listed edge kind.
func (p *Profile) findField(typeName, scope, fieldName string) (Field, bool) {
	for _, t := range p.NodeTypes {
		if t.Name != typeName {
			continue
		}
		if scope != "" && t.Scope != scope {
			continue
		}
		for _, f := range t.Fields {
			if f.Name == fieldName {
				return f, true
			}
		}
	}
	return Field{}, false
}

// Validate checks the profile document for internal consistency. The
// profile_version check runs first and alone: a version outside the
// supported range fails with one distinct message naming the version and
// the supported range, the migrate-before-using-this-spex signal, before
// any other check runs. Every remaining violation — a node type missing a
// name or plural key or legal scope, a duplicate (scope, name) pair, a
// field with an unknown kind, a reference field naming an undeclared target
// type ("module" is never declared, so a reference field naming it always
// fails here), an enumeration on a non-text field, bounds on a non-integer
// field, a duplicate field name within one type, a field name colliding
// with an envelope field, or a coverage/plan-relevant/impact-level/
// absorbable entry naming an undeclared type or a field that does not
// exist, is not a reference field, or does not target the covered type —
// is collected and returned together via errors.Join, so a malformed
// profile is reported in one pass rather than one field at a time across
// repeated runs.
func (p *Profile) Validate() error {
	if p.ProfileVersion != nil && (*p.ProfileVersion < minProfileVersion || *p.ProfileVersion > maxProfileVersion) {
		return fmt.Errorf("profile_version: %d: unsupported (supported: %d-%d)", *p.ProfileVersion, minProfileVersion, maxProfileVersion)
	}

	var errs []error
	check := func(cond bool, path, msg string) {
		if !cond {
			errs = append(errs, fmt.Errorf("%s: %s", path, msg))
		}
	}

	declared := map[string]bool{}
	seenScoped := map[string]bool{}
	for i, t := range p.NodeTypes {
		path := fmt.Sprintf("node_types[%d]", i)
		check(t.Name != "", path, "name: required")
		check(t.PluralKey != "", path, "plural_key: required")
		check(t.Scope == "project" || t.Scope == "module", path, `scope: must be "project" or "module"`)
		if t.Name != "" && (t.Scope == "project" || t.Scope == "module") {
			key := t.Scope + ":" + t.Name
			check(!seenScoped[key], path, fmt.Sprintf("duplicate node type %q declared for scope %q", t.Name, t.Scope))
			seenScoped[key] = true
		}
		if t.Name != "" {
			declared[t.Name] = true
		}
	}

	for i, t := range p.NodeTypes {
		typePath := fmt.Sprintf("node_types[%d]", i)
		seenFields := map[string]bool{}
		for j, f := range t.Fields {
			path := fmt.Sprintf("%s.fields[%d]", typePath, j)

			if envelopeFieldNames[f.Name] || (f.Name == "content" && t.RequiresContent) {
				errs = append(errs, fmt.Errorf("%s: field %q: collides with the envelope", path, f.Name))
			}
			if seenFields[f.Name] {
				errs = append(errs, fmt.Errorf("%s: field %q: duplicate field name on node type %q", path, f.Name, t.Name))
			}
			seenFields[f.Name] = true

			switch f.Kind {
			case FieldKindText, FieldKindInteger, FieldKindReference:
			default:
				errs = append(errs, fmt.Errorf("%s: field %q: unknown kind %q", path, f.Name, f.Kind))
				continue
			}

			if len(f.Enum) > 0 && f.Kind != FieldKindText {
				errs = append(errs, fmt.Errorf("%s: field %q: enum is only valid on a text field", path, f.Name))
			}
			if (f.Minimum != nil || f.Maximum != nil) && f.Kind != FieldKindInteger {
				errs = append(errs, fmt.Errorf("%s: field %q: minimum/maximum are only valid on an integer field", path, f.Name))
			}

			if f.Kind == FieldKindReference {
				check(len(f.Targets) > 0, path, fmt.Sprintf("field %q: targets: at least one node type required", f.Name))
				for _, target := range f.Targets {
					check(declared[target], path, fmt.Sprintf("field %q: targets: undeclared node type %q", f.Name, target))
				}
			} else if len(f.Targets) > 0 {
				errs = append(errs, fmt.Errorf("%s: field %q: targets is only valid on a reference field", path, f.Name))
			}
		}
	}

	for i, c := range p.CoverageChains {
		path := fmt.Sprintf("coverage_chains[%d]", i)
		check(declared[c.CoveredType], path, fmt.Sprintf("covered_type: undeclared node type %q", c.CoveredType))
		check(declared[c.CoveringType], path, fmt.Sprintf("covering_type: undeclared node type %q", c.CoveringType))
		if declared[c.CoveringType] {
			f, ok := p.findField(c.CoveringType, c.CoveringScope, c.Edge)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: edge: covering type %q declares no field named %q", path, c.CoveringType, c.Edge))
			} else {
				check(f.Kind == FieldKindReference, path, fmt.Sprintf("edge: field %q on %q is not a reference field", c.Edge, c.CoveringType))
				check(slices.Contains(f.Targets, c.CoveredType), path, fmt.Sprintf("edge: field %q on %q does not target %q", c.Edge, c.CoveringType, c.CoveredType))
			}
		}
	}

	for _, name := range p.PlanRelevant {
		check(declared[name], "plan_relevant", fmt.Sprintf("undeclared node type %q", name))
	}
	for name := range p.ImpactLevels {
		check(declared[name], "impact_levels", fmt.Sprintf("undeclared node type %q", name))
	}
	for name := range p.Absorbable {
		check(declared[name], "absorbable", fmt.Sprintf("undeclared node type %q", name))
	}

	return errors.Join(errs...)
}

// ProjectNodeTypes returns the profile's project-scoped node types,
// converted to the ProjectNodeType shape ComposeProjectSchema consumes.
func (p *Profile) ProjectNodeTypes() []ProjectNodeType {
	var out []ProjectNodeType
	for _, t := range p.NodeTypes {
		if t.Scope != "project" {
			continue
		}
		out = append(out, ProjectNodeType{
			Name:            t.Name,
			PluralKey:       t.PluralKey,
			RequiresContent: t.RequiresContent,
			Fields:          t.Fields,
			Comment:         t.Comment,
		})
	}
	return out
}

// ModuleNodeTypes returns the profile's module-scoped node types, converted
// to the ModuleNodeType shape ComposeModuleSchema consumes.
func (p *Profile) ModuleNodeTypes() []ModuleNodeType {
	var out []ModuleNodeType
	for _, t := range p.NodeTypes {
		if t.Scope != "module" {
			continue
		}
		out = append(out, ModuleNodeType{
			Name:            t.Name,
			PluralKey:       t.PluralKey,
			RequiresContent: t.RequiresContent,
			Fields:          t.Fields,
		})
	}
	return out
}
