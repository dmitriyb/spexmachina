package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NodeType declares one node type in the resolved profile's vocabulary: its
// name, the plural key naming its array property in project.json or
// module.json, whether it is project- or module-scoped, whether it requires
// a content leaf, and its two per-type role flags. "requirement" is declared
// twice — once per scope — because the project-level and module-level
// requirement arrays carry different envelope constraints even though they
// share a type name and both trigger the completeness rules.
type NodeType struct {
	Name                string `json:"name"`
	PluralKey           string `json:"plural_key"`
	Scope               string `json:"scope"` // "project" or "module"
	RequiresContent     bool   `json:"requires_content,omitempty"`
	CompletenessTrigger bool   `json:"completeness_trigger,omitempty"`
	NameDeclarable      bool   `json:"name_declarable,omitempty"`
}

// Edge declares one legal edge kind: the reference field name, the node
// types that may carry it, and the node types it may point at. "module" is a
// legal From/To entry even though module is not a declarable node type — it
// is the fixed interior-node concept requires_module points at. Cyclic
// exempts the edge kind from the DAG cycle check; an omitted (false) value
// means the edge kind is cycle-checked.
type Edge struct {
	Kind   string   `json:"kind"`
	From   []string `json:"from"`
	To     []string `json:"to"`
	Cyclic bool     `json:"cyclic,omitempty"`
}

// CoverageChain declares one coverage rule: every node of CoveredType must
// be the target of at least one Edge-kind edge from some node of
// CoveringType. The Scope fields disambiguate CoveredType/CoveringType only
// when the type name is shared across scopes (requirement); they are empty
// when the type has only one scope.
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
// node types, the legal edges, and the graph rules (coverage chains, the
// plan-relevant set, the per-type impact-level mapping, the hashed field
// allowlists, and refresh's absorbable directions). It carries no behaviour
// — every consumer reads its own policy off this document rather than
// branching on type-name string literals.
//
// HashedFields is keyed by "<scope>:<name>" rather than by name alone,
// because project-scoped and module-scoped requirement declare different
// allowlists (a project requirement hashes priority, a module requirement
// hashes preq_id) even though they share a name. A content-bearing type has
// no entry: its leaf hashes from its content file, not its JSON fields.
type Profile struct {
	NodeTypes      []NodeType                  `json:"node_types"`
	Edges          []Edge                      `json:"edges"`
	CoverageChains []CoverageChain             `json:"coverage_chains"`
	PlanRelevant   []string                    `json:"plan_relevant"`
	ImpactLevels   map[string]string           `json:"impact_levels"`
	HashedFields   map[string][]string         `json:"hashed_fields"`
	Absorbable     map[string]AbsorbDirections `json:"absorbable"`
}

// ResolveProfile resolves the profile for the project rooted at specDir: it
// reads "profile.json" beside "project.json" when present, or returns the
// built-in default profile when the file is absent — absence is the
// supported default, never an error. A present file is decoded strictly (no
// unknown top-level fields, so an attempt to declare a fixed point is
// rejected) and validated before it is returned, so a malformed profile
// fails once, early, naming the file and the defect, rather than surfacing
// downstream as a cascade of schema-conformance errors.
func ResolveProfile(specDir string) (*Profile, error) {
	path := filepath.Join(specDir, "profile.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultProfile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("schema: resolve profile: read %s: %w", path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p Profile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("schema: resolve profile: parse %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("schema: resolve profile: %s: %w", path, err)
	}
	return &p, nil
}

// Validate checks the profile document for internal consistency: every node
// type carries a name and a plural key and a legal scope, no two node types
// declare the same (scope, name) pair, and every edge, coverage chain, and
// graph-rule entry names only node types (or, for edges, the fixed "module"
// concept) the profile itself declares. All violations are collected and
// returned together via errors.Join, so a malformed profile is reported in
// one pass rather than one field at a time across repeated runs.
func (p *Profile) Validate() error {
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

	validRef := func(name string) bool {
		return name == "module" || declared[name]
	}

	frameEdgeKinds := builtinEdgeKinds()
	frameTypeNames := builtinTypeNames()

	edgeKinds := map[string]bool{}
	for i, e := range p.Edges {
		path := fmt.Sprintf("edges[%d]", i)
		check(e.Kind != "", path, "kind: required")
		check(len(e.From) > 0, path, "from: at least one node type required")
		check(len(e.To) > 0, path, "to: at least one node type required")
		for _, f := range e.From {
			check(validRef(f), path, fmt.Sprintf("from: undeclared node type %q", f))
		}
		for _, tt := range e.To {
			check(validRef(tt), path, fmt.Sprintf("to: undeclared node type %q", tt))
		}
		if e.Kind != "" && !frameEdgeKinds[e.Kind] {
			for _, f := range e.From {
				if frameTypeNames[f] {
					errs = append(errs, fmt.Errorf("%s: edge kind %q: sourced at built-in type %q but the frame does not already carry this edge kind — built-in definitions are frame-fixed and gain no new reference fields in composition", path, e.Kind, f))
					break
				}
			}
		}
		if e.Kind != "" {
			edgeKinds[e.Kind] = true
		}
	}

	for i, c := range p.CoverageChains {
		path := fmt.Sprintf("coverage_chains[%d]", i)
		check(declared[c.CoveredType], path, fmt.Sprintf("covered_type: undeclared node type %q", c.CoveredType))
		check(declared[c.CoveringType], path, fmt.Sprintf("covering_type: undeclared node type %q", c.CoveringType))
		check(edgeKinds[c.Edge], path, fmt.Sprintf("edge: undeclared edge kind %q", c.Edge))
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
	for key := range p.HashedFields {
		scope, name, ok := strings.Cut(key, ":")
		if !ok {
			errs = append(errs, fmt.Errorf("hashed_fields: key %q: must be \"<scope>:<name>\"", key))
			continue
		}
		check(scope == "project" || scope == "module", "hashed_fields", fmt.Sprintf("key %q: scope must be \"project\" or \"module\"", key))
		check(seenScoped[scope+":"+name], "hashed_fields", fmt.Sprintf("key %q: undeclared node type %q for scope %q", key, name, scope))
	}

	return errors.Join(errs...)
}

// builtinEdgeKinds returns the edge kinds the shipped frame already carries
// as reference-field properties on its built-in node-type definitions — the
// set Validate checks a profile-declared edge's Kind against when the
// edge's source names a built-in type.
func builtinEdgeKinds() map[string]bool {
	kinds := make(map[string]bool)
	for _, e := range DefaultProfile().Edges {
		kinds[e.Kind] = true
	}
	return kinds
}

// builtinTypeNames returns the node type names the shipped frame already
// defines as fixed $defs entries — requirement, component, data_flow,
// test_section, api. ComposeProjectSchema and ComposeModuleSchema restore
// these types' original array property and $defs entry unchanged from the
// frame, so a profile-declared edge kind the frame does not already carry
// could never reach one of these types' reference fields in composition;
// Validate refuses such a declaration outright rather than silently
// accepting an edge that composition would then drop.
func builtinTypeNames() map[string]bool {
	names := make(map[string]bool)
	for _, t := range DefaultProjectNodeTypes() {
		names[t.Name] = true
	}
	for _, t := range DefaultModuleNodeTypes() {
		names[t.Name] = true
	}
	return names
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
		})
	}
	return out
}

// DefaultProfile returns the built-in default profile: the golden record of
// the policy previously spread across seven modules. It declares today's
// ontology exactly — the five node types (requirement declared once per
// scope), today's seven edge kinds with the cyclic flag omitted on every
// one, the three coverage chains, the plan-relevant set, the per-type
// impact-level mapping, the hashed field allowlists, and refresh's
// absorbable directions. It is built from DefaultProjectNodeTypes and
// DefaultModuleNodeTypes — the same node-type declarations
// ComposeProjectSchema and ComposeModuleSchema already use to reproduce the
// shipped schemas — so a profile-driven composition of the default profile
// is guaranteed to match them.
func DefaultProfile() *Profile {
	p := &Profile{}

	for _, t := range DefaultProjectNodeTypes() {
		p.NodeTypes = append(p.NodeTypes, NodeType{
			Name:                t.Name,
			PluralKey:           t.PluralKey,
			Scope:               "project",
			RequiresContent:     t.RequiresContent,
			CompletenessTrigger: t.Name == "requirement",
		})
	}
	for _, t := range DefaultModuleNodeTypes() {
		p.NodeTypes = append(p.NodeTypes, NodeType{
			Name:                t.Name,
			PluralKey:           t.PluralKey,
			Scope:               "module",
			RequiresContent:     t.RequiresContent,
			CompletenessTrigger: t.Name == "requirement",
			NameDeclarable:      t.Name == "component" || t.Name == "api",
		})
	}

	p.Edges = []Edge{
		{Kind: "preq_id", From: []string{"requirement"}, To: []string{"requirement"}},
		{Kind: "implements", From: []string{"component"}, To: []string{"requirement"}},
		{Kind: "uses", From: []string{"component", "data_flow"}, To: []string{"component"}},
		{Kind: "provided_by", From: []string{"api"}, To: []string{"component"}},
		{Kind: "describes", From: []string{"test_section"}, To: []string{"component"}},
		{Kind: "depends_on", From: []string{"requirement"}, To: []string{"requirement"}},
		{Kind: "requires_module", From: []string{"module"}, To: []string{"module"}},
	}

	p.CoverageChains = []CoverageChain{
		{
			CoveredType: "requirement", CoveredScope: "project",
			Edge:         "preq_id",
			CoveringType: "requirement", CoveringScope: "module",
		},
		{
			CoveredType: "requirement", CoveredScope: "module",
			Edge:         "implements",
			CoveringType: "component",
		},
		{
			CoveredType:  "component",
			Edge:         "describes",
			CoveringType: "test_section",
		},
	}

	p.PlanRelevant = []string{"component", "data_flow", "test_section"}

	p.ImpactLevels = map[string]string{
		"test_section": "impl_only",
		"data_flow":    "contract",
		"api":          "contract",
		"component":    "arch_impl",
		"requirement":  "structural",
	}

	p.HashedFields = map[string][]string{
		"project:requirement": {"depends_on", "description", "id", "name", "priority", "type"},
		"module:requirement":  {"depends_on", "description", "id", "name", "preq_id", "type"},
		"module:api":          {"description", "group", "id", "name", "provided_by"},
	}

	p.Absorbable = map[string]AbsorbDirections{
		"requirement":  {Added: true, Removed: true},
		"api":          {Added: true, Removed: true},
		"component":    {Added: false, Removed: true},
		"data_flow":    {Added: false, Removed: false},
		"test_section": {Added: false, Removed: false},
	}

	return p
}
