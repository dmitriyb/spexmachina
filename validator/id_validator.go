package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// builtinModuleTypeNames are the module-scoped node types schema.ModuleSpec
// carries as dedicated typed fields. A profile-declared type outside this
// set has no such field and is read generically off raw JSON instead.
var builtinModuleTypeNames = map[string]bool{
	"requirement": true, "component": true, "data_flow": true,
	"test_section": true, "api": true,
}

// builtinEdgeCoverage is the (kind, from-type) pairs checkProjectRefs and
// checkModuleRefs already resolve by name — the eight pairs DefaultProfile's
// seven edge kinds expand to ("uses" carries two: component and data_flow).
// A profile-declared edge covers a (kind, from-type) pair outside this set —
// whether its kind is altogether new or shares a name with a built-in kind
// but is declared from a type the built-in checks don't enumerate — and is
// resolved generically by checkExtraModuleEdges or checkExtraProjectEdges
// instead, depending on the from-type's scope. Partitioning on
// kind name alone would miss the second case: a profile can declare
// "uses" from a type neither checkModuleRefs loop reads, and that edge
// would go unchecked by both the hardcoded path and the generic one.
var builtinEdgeCoverage = map[string]map[string]bool{
	"implements":      {"component": true},
	"uses":            {"component": true, "data_flow": true},
	"describes":       {"test_section": true},
	"provided_by":     {"api": true},
	"depends_on":      {"requirement": true},
	"requires_module": {"module": true},
	"preq_id":         {"requirement": true},
}

// CheckIDs validates identity-hash uniqueness within each array and
// cross-reference integrity across the spec. It runs uniqueness checks
// first — if IDs are duplicated, cross-reference checks may be
// misleading because references cannot be unambiguously resolved.
//
// Every check uses string set membership (map[string]bool). There is no
// integer parsing, no path decomposition, and no comparison across
// types — references are identity hashes looked up against per-array
// sets.
//
// The checked arrays and edges are the resolved profile's declaration. The
// five built-in module-scoped types (requirement, component, data_flow,
// test_section, api) and the eight built-in (edge kind, from-type) pairs are
// checked via schema.ModuleSpec's typed fields, unchanged from before
// profiles existed. Anything the resolved profile declares beyond that — a
// node type with no dedicated Go field, or an (edge kind, from-type) pair
// beyond the built-in eight, including a profile-declared edge that reuses a
// built-in kind name from a type the hardcoded checks don't enumerate, or
// one declared from the fixed "module" concept (legal per Profile.Validate
// though "module" is not itself a declared node type) — is checked
// generically, by parsing the raw JSON array the profile names, or, for
// "module", project.json's fixed "modules" array.
func CheckIDs(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "id")
	if len(errs) > 0 {
		return errs
	}

	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "id",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}
	}

	var result []ValidationError

	result = append(result, checkProjectUniqueness(project)...)

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		result = append(result, checkModuleUniqueness(modName, modules[modName])...)
	}
	result = append(result, checkExtraModuleUniqueness(specDir, modNames, project, extraModuleNodeTypes(profile))...)
	result = append(result, checkExtraProjectUniqueness(specDir, extraProjectNodeTypes(profile))...)
	result = append(result, checkAPINameUniqueness(modNames, modules)...)
	result = append(result, checkNameRecoverability(specDir, modNames, modules, project, profile)...)

	if len(result) > 0 {
		return result
	}

	result = append(result, checkProjectRefs(project)...)
	result = append(result, checkProjectPriority(project)...)
	for _, modName := range modNames {
		result = append(result, checkModuleRefs(modName, modules[modName], project)...)
	}
	result = append(result, checkExtraModuleEdges(specDir, modNames, project, modules, profile)...)
	result = append(result, checkExtraProjectEdges(specDir, project, profile)...)

	return result
}

func checkProjectUniqueness(project *schema.Project) []ValidationError {
	var errs []ValidationError

	errs = append(errs, checkDuplicateIDs("project.json:/requirements", reqIDs(project.Requirements))...)
	errs = append(errs, checkDuplicateIDs("project.json:/modules", moduleIDs(project.Modules))...)
	errs = append(errs, checkDuplicateIDs("project.json:/sections", sectionIDs(project.Sections))...)

	return errs
}

func checkModuleUniqueness(modName string, mod *schema.ModuleSpec) []ValidationError {
	var errs []ValidationError
	prefix := modName + "/module.json:"

	errs = append(errs, checkDuplicateIDs(prefix+"/requirements", moduleReqIDs(mod.Requirements))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/components", compIDs(mod.Components))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/data_flows", flowIDs(mod.DataFlows))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/test_sections", testSectionIDs(mod.TestSections))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/apis", apiIDs(mod.APIs))...)

	return errs
}

// extraModuleNodeTypes returns the module-scoped node types the resolved
// profile declares beyond the five built into schema.ModuleSpec.
func extraModuleNodeTypes(profile *schema.Profile) []schema.NodeType {
	var out []schema.NodeType
	for _, nt := range profile.NodeTypes {
		if nt.Scope == "module" && !builtinModuleTypeNames[nt.Name] {
			out = append(out, nt)
		}
	}
	return out
}

// extraProjectNodeTypes returns the project-scoped node types the resolved
// profile declares beyond "requirement", the one project.json carries a
// typed field for.
func extraProjectNodeTypes(profile *schema.Profile) []schema.NodeType {
	var out []schema.NodeType
	for _, nt := range profile.NodeTypes {
		if nt.Scope == "project" && nt.Name != "requirement" {
			out = append(out, nt)
		}
	}
	return out
}

// moduleScopedNodeTypes returns every module-scoped node type the resolved
// profile declares, built-in and profile-declared alike, in declared order.
func moduleScopedNodeTypes(profile *schema.Profile) []schema.NodeType {
	var out []schema.NodeType
	for _, nt := range profile.NodeTypes {
		if nt.Scope == "module" {
			out = append(out, nt)
		}
	}
	return out
}

// checkExtraModuleUniqueness checks ID uniqueness for module-scoped node
// types the resolved profile declares beyond the five built into
// schema.ModuleSpec — reached only by parsing module.json generically, since
// those types have no dedicated Go field.
func checkExtraModuleUniqueness(specDir string, modNames []string, project *schema.Project, extraTypes []schema.NodeType) []ValidationError {
	if len(extraTypes) == 0 {
		return nil
	}
	var errs []ValidationError
	for _, modName := range modNames {
		modPath := modulePathByName(project, modName)
		prefix := modName + "/module.json:"
		for _, nt := range extraTypes {
			entries, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
			if err != nil {
				continue
			}
			ids := make([]string, len(entries))
			for i, e := range entries {
				ids[i] = e.str("id")
			}
			errs = append(errs, checkDuplicateIDs(prefix+"/"+nt.PluralKey, ids)...)
		}
	}
	return errs
}

// checkExtraProjectUniqueness checks ID uniqueness for project-scoped node
// types the resolved profile declares beyond "requirement".
func checkExtraProjectUniqueness(specDir string, extraTypes []schema.NodeType) []ValidationError {
	if len(extraTypes) == 0 {
		return nil
	}
	var errs []ValidationError
	for _, nt := range extraTypes {
		entries, err := rawProjectEntries(specDir, nt.PluralKey)
		if err != nil {
			continue
		}
		ids := make([]string, len(entries))
		for i, e := range entries {
			ids[i] = e.str("id")
		}
		errs = append(errs, checkDuplicateIDs("project.json:/"+nt.PluralKey, ids)...)
	}
	return errs
}

// checkAPINameUniqueness enforces the one uniqueness rule in the spec that is
// not per-array: an api name is the external surface string callers type, so
// two modules claiming the same one describe the same surface twice. Every
// other uniqueness check in this file is scoped to a single array in a single
// file; this one spans the project.
func checkAPINameUniqueness(modNames []string, modules map[string]*schema.ModuleSpec) []ValidationError {
	declarers := map[string][]string{}
	var order []string
	for _, modName := range modNames {
		mod := modules[modName]
		if mod == nil {
			continue
		}
		for _, api := range mod.APIs {
			if _, seen := declarers[api.Name]; !seen {
				order = append(order, api.Name)
			}
			declarers[api.Name] = append(declarers[api.Name], modName)
		}
	}

	var errs []ValidationError
	for _, name := range order {
		mods := declarers[name]
		if len(mods) < 2 {
			continue
		}
		errs = append(errs, ValidationError{
			Check:    "id",
			Severity: "error",
			Path:     mods[0] + "/module.json:/apis",
			Message: fmt.Sprintf("duplicate api name %q; api names are globally unique, declared by: %s",
				name, strings.Join(mods, ", ")),
		})
	}
	return errs
}

// checkNameRecoverability enforces the shape a name must have for the
// removal-time sweep to be able to find it again, for every node type the
// resolved profile marks name-declarable — the same per-type NameDeclarable
// flag nameDeclarableNodeTypes reads for CheckRemovedNames. Under the
// default profile that is exactly component and api, matching this check's
// original hardcoded pair.
//
// The two halves cannot drift, because there is only one half. Before this,
// the validator enforced strings.Fields normalization and the word bound while
// the sweep additionally ran normalizeToken over every token, and every name
// in the gap — `spex validate [--json]`, `Validator (core)`, `Widget.`,
// `_private`, `Bob's` — validated clean and was permanently unsweepable.
//
// Enforcing the bound here rather than raising it in the scan is what keeps
// the two in agreement: the constant they share means an unsweepable name
// cannot be authored in the first place.
func checkNameRecoverability(specDir string, modNames []string, modules map[string]*schema.ModuleSpec, project *schema.Project, profile *schema.Profile) []ValidationError {
	declarable := nameDeclarableNodeTypes(profile)
	var errs []ValidationError
	for _, modName := range modNames {
		mod := modules[modName]
		if mod == nil {
			continue
		}
		prefix := modName + "/module.json:"
		for _, nt := range declarable {
			entries, ok := moduleTypedEntries(mod, nt.Name)
			if !ok {
				raw, err := rawModuleEntries(specDir, modulePathByName(project, modName), nt.PluralKey)
				if err != nil {
					continue
				}
				entries = namedEntriesFromRaw(raw)
			}
			for _, e := range entries {
				errs = append(errs, nameShapeError(prefix, nt.PluralKey, nt.Name, e.name, e.id)...)
			}
		}
	}
	return errs
}

// nameShapeError applies declarableName to one declared name. The verdict
// comes from that one function; the branching here only chooses which of the
// three ways a name can fail it to describe.
func nameShapeError(prefix, array, nodeType, name, id string) []ValidationError {
	phrase, ok := declarableName(name)
	if ok {
		return nil
	}

	fail := func(format string, args ...any) []ValidationError {
		return []ValidationError{{
			Check:    "id",
			Severity: "error",
			Path:     fmt.Sprintf("%s/%s/%s", prefix, array, id),
			Message:  fmt.Sprintf(format, args...),
		}}
	}

	switch words := len(nameTokens(name)); {
	case words == 0:
		return fail("%s name %q reduces to nothing once corpus tokenization strips its punctuation; the removal-time name check rebuilds a removed node's name by tokenizing corpus prose, and a name that tokenizes to no words can never be rebuilt",
			nodeType, name)
	case phrase != name:
		return fail("%s name %q is not its own corpus tokenization (it reduces to %q); the removal-time name check only ever builds candidate phrases out of corpus tokens, so a name that differs from its tokenization is one no phrase can equal and the removal could never be swept — declare it as %q",
			nodeType, name, phrase, phrase)
	default:
		return fail("%s name %q has %d words; at most %d are allowed, because the removal-time name check scans corpus phrases of at most %d words and a longer name could never be swept",
			nodeType, name, words, maxNameWords, maxNameWords)
	}
}

// checkDuplicateIDs reports any identity hashes that appear more than once.
func checkDuplicateIDs(path string, ids []string) []ValidationError {
	seen := make(map[string]int, len(ids))
	for _, id := range ids {
		seen[id]++
	}

	var errs []ValidationError
	for id, count := range seen {
		if count > 1 {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("duplicate ID %s", id),
			})
		}
	}
	return errs
}

func checkProjectRefs(project *schema.Project) []ValidationError {
	var errs []ValidationError

	projReqSet := idSet(reqIDs(project.Requirements))
	modIDSet := idSet(moduleIDs(project.Modules))

	for _, req := range project.Requirements {
		for _, depID := range req.DependsOn {
			if !projReqSet[depID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/requirements/%s", req.ID),
					Message:  fmt.Sprintf("depends_on references non-existent requirement %s", depID),
				})
			}
		}
	}

	for _, mod := range project.Modules {
		for _, depID := range mod.RequiresModule {
			if !modIDSet[depID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/modules/%s", mod.ID),
					Message:  fmt.Sprintf("requires_module references non-existent module %s", depID),
				})
			}
		}
	}

	return errs
}

func checkProjectPriority(project *schema.Project) []ValidationError {
	var errs []ValidationError
	for _, req := range project.Requirements {
		if req.Priority == nil {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("project.json:/requirements/%s", req.ID),
				Message:  fmt.Sprintf("project requirement %s missing priority", req.ID),
			})
		} else if *req.Priority < 0 || *req.Priority > 4 {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("project.json:/requirements/%s", req.ID),
				Message:  fmt.Sprintf("project requirement %s priority %d out of range (must be 0-4)", req.ID, *req.Priority),
			})
		}
	}
	return errs
}

func checkModuleRefs(modName string, mod *schema.ModuleSpec, project *schema.Project) []ValidationError {
	var errs []ValidationError

	reqSet := idSet(moduleReqIDs(mod.Requirements))
	compSet := idSet(compIDs(mod.Components))
	projReqSet := idSet(reqIDs(project.Requirements))
	prefix := modName + "/module.json:"

	for _, comp := range mod.Components {
		for _, implID := range comp.Implements {
			if !reqSet[implID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/components/%s", prefix, comp.ID),
					Message:  fmt.Sprintf("implements references non-existent requirement %s", implID),
				})
			}
		}
		for _, useID := range comp.Uses {
			if !compSet[useID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/components/%s", prefix, comp.ID),
					Message:  fmt.Sprintf("uses references non-existent component %s", useID),
				})
			}
		}
	}

	for _, flow := range mod.DataFlows {
		for _, useID := range flow.Uses {
			if !compSet[useID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/data_flows/%s", prefix, flow.ID),
					Message:  fmt.Sprintf("uses references non-existent component %s", useID),
				})
			}
		}
	}

	// provided_by is module-local by design: an api belongs to the module
	// owning its entry point, and any other module's involvement is already
	// carried by component `uses` edges. Checking against this module's
	// compSet is therefore the whole rule — a real component id from
	// another module is as wrong as an id that exists nowhere.
	for _, api := range mod.APIs {
		for _, provID := range api.ProvidedBy {
			if !compSet[provID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/apis/%s", prefix, api.ID),
					Message:  fmt.Sprintf("provided_by references non-existent component %s (provided_by is module-local)", provID),
				})
			}
		}
	}

	for _, ts := range mod.TestSections {
		for _, descID := range ts.Describes {
			if !compSet[descID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/test_sections/%s", prefix, ts.ID),
					Message:  fmt.Sprintf("describes references non-existent component %s", descID),
				})
			}
		}
	}

	for _, req := range mod.Requirements {
		for _, depID := range req.DependsOn {
			if !reqSet[depID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/requirements/%s", prefix, req.ID),
					Message:  fmt.Sprintf("depends_on references non-existent requirement %s", depID),
				})
			}
		}
		if req.PreqID == "" {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("%s/requirements/%s", prefix, req.ID),
				Message:  fmt.Sprintf("requirement %s missing preq_id", req.ID),
			})
		} else if !projReqSet[req.PreqID] {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("%s/requirements/%s", prefix, req.ID),
				Message:  fmt.Sprintf("preq_id references non-existent project requirement %s", req.PreqID),
			})
		}
	}

	return errs
}

// checkExtraModuleEdges checks cross-reference integrity for (edge kind,
// from-type) pairs the resolved profile declares beyond the built-in eight,
// where the from-type is module-scoped — e.g. a project profile declaring a
// "serves" edge from a custom "endpoint" type to components, or reusing the
// built-in "uses" kind from a type the hardcoded checkModuleRefs loops don't
// enumerate. checkExtraProjectEdges is this function's project-scoped
// counterpart, and together the two cover every from-type extraEdgeKinds can
// return: a from-type is either module-scoped (resolved here via
// findModuleNodeType) or not — and "not module-scoped" means either a
// profile-declared project-scoped NodeType or the fixed "module" concept,
// which Profile.Validate accepts as a legal from-type even though it is not
// itself a NodeTypes entry; checkExtraProjectEdges' projectEdgeSourceKey
// resolves both. Resolution uses the same string set-membership machinery as the
// built-in edges; the source array is read generically regardless of
// whether its node type is built-in, because a profile-declared (kind,
// from-type) pair has no dedicated Go loop. The target set prefers a
// module-local array — every edge among today's declared kinds that is not
// project-wide (requires_module) or cross-file (preq_id) resolves within the
// same module, and both of those stay hardcoded in checkProjectRefs and
// checkModuleRefs above rather than going through this generic path.
func checkExtraModuleEdges(specDir string, modNames []string, project *schema.Project, modules map[string]*schema.ModuleSpec, profile *schema.Profile) []ValidationError {
	edges := extraEdgeKinds(profile)
	if len(edges) == 0 {
		return nil
	}

	var errs []ValidationError
	for _, modName := range modNames {
		mod := modules[modName]
		if mod == nil {
			continue
		}
		modPath := modulePathByName(project, modName)
		prefix := modName + "/module.json:"

		for _, edge := range edges {
			for _, fromName := range edge.From {
				nt, ok := findModuleNodeType(profile, fromName)
				if !ok {
					continue
				}
				sources, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
				if err != nil || len(sources) == 0 {
					continue
				}

				targets := map[string]bool{}
				for _, toName := range edge.To {
					set, err := edgeTargetSet(specDir, modPath, mod, project, profile, toName)
					if err != nil {
						continue
					}
					for id := range set {
						targets[id] = true
					}
				}

				errs = append(errs, checkEdgeSources(prefix, nt.PluralKey, edge.Kind, edge.To, sources, targets)...)
			}
		}
	}
	return errs
}

// checkExtraProjectEdges is checkExtraModuleEdges' project-scoped
// counterpart: it checks cross-reference integrity for (edge kind,
// from-type) pairs the resolved profile declares beyond the built-in eight,
// where the from-type is not module-scoped — e.g. a profile declaring a
// "groups" edge from a custom project-scoped "milestone" type to
// requirements, or an edge declared from the fixed "module" concept itself
// (projectEdgeSourceKey resolves both). A project-scoped source has no
// owning module, so its sources are read once from project.json rather than
// once per module, and its targets resolve at project scope only via
// projectEdgeTargetSet — there is no module to scope a module-local target
// within.
func checkExtraProjectEdges(specDir string, project *schema.Project, profile *schema.Profile) []ValidationError {
	edges := extraEdgeKinds(profile)
	if len(edges) == 0 {
		return nil
	}

	var errs []ValidationError
	for _, edge := range edges {
		for _, fromName := range edge.From {
			pluralKey, ok := projectEdgeSourceKey(profile, fromName)
			if !ok {
				continue
			}
			sources, err := rawProjectEntries(specDir, pluralKey)
			if err != nil || len(sources) == 0 {
				continue
			}

			targets := map[string]bool{}
			for _, toName := range edge.To {
				set, err := projectEdgeTargetSet(specDir, project, profile, toName)
				if err != nil {
					continue
				}
				for id := range set {
					targets[id] = true
				}
			}

			errs = append(errs, checkEdgeSources("project.json:", pluralKey, edge.Kind, edge.To, sources, targets)...)
		}
	}
	return errs
}

// projectEdgeSourceKey resolves an edge's project-scoped from-type name to
// the project.json array key its source entries are read from generically:
// a profile-declared project-scoped NodeType's own PluralKey, or the fixed
// "modules" key for the "module" concept. "module" is legal as an edge
// from-type per Profile.Validate's validRef even though it is not itself a
// profile.NodeTypes entry, so findProjectNodeType cannot resolve it — and
// schema.Module's typed struct carries no generic reference fields, so a
// profile-declared edge naming "module" as its source (e.g. "owns") must
// still be read generically here, exactly as a profile-declared type would
// be, rather than through the typed field projectTypeIDSet uses to resolve
// "module" on the target side.
func projectEdgeSourceKey(profile *schema.Profile, fromName string) (string, bool) {
	if fromName == "module" {
		return "modules", true
	}
	nt, ok := findProjectNodeType(profile, fromName)
	if !ok {
		return "", false
	}
	return nt.PluralKey, true
}

// checkEdgeSources reports each source entry's edge.Kind reference field
// values that are not present in targets. Shared by checkExtraModuleEdges
// and checkExtraProjectEdges — the two differ only in where sources and
// targets are resolved from, never in how a resolved (sources, targets) pair
// is turned into errors.
func checkEdgeSources(prefix, pluralKey, edgeKind string, to []string, sources []rawEntry, targets map[string]bool) []ValidationError {
	var errs []ValidationError
	for _, src := range sources {
		srcID := src.str("id")
		for _, targetID := range src.strSlice(edgeKind) {
			if !targets[targetID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/%s/%s", prefix, pluralKey, srcID),
					Message: fmt.Sprintf("%s references non-existent %s %s",
						edgeKind, strings.Join(to, "/"), targetID),
				})
			}
		}
	}
	return errs
}

// extraEdgeKinds returns the resolved profile's declared edges reduced to
// the (kind, from-type) pairs checkProjectRefs and checkModuleRefs do not
// already resolve. An edge whose kind matches a built-in one is not
// automatically covered: it is only covered for the specific from-types
// builtinEdgeCoverage lists, so a profile that adds a from-type to a
// built-in kind (a second "uses" edge declared from a profile-declared
// type, say) still surfaces that from-type here even though the kind name
// itself is not new.
func extraEdgeKinds(profile *schema.Profile) []schema.Edge {
	var out []schema.Edge
	for _, e := range profile.Edges {
		covered := builtinEdgeCoverage[e.Kind]
		var from []string
		for _, f := range e.From {
			if !covered[f] {
				from = append(from, f)
			}
		}
		if len(from) == 0 {
			continue
		}
		extra := e
		extra.From = from
		out = append(out, extra)
	}
	return out
}

// findModuleNodeType returns the module-scoped NodeType named name, if the
// resolved profile declares one.
func findModuleNodeType(profile *schema.Profile, name string) (schema.NodeType, bool) {
	for _, nt := range profile.NodeTypes {
		if nt.Name == name && nt.Scope == "module" {
			return nt, true
		}
	}
	return schema.NodeType{}, false
}

// findProjectNodeType returns the project-scoped NodeType named name, if the
// resolved profile declares one.
func findProjectNodeType(profile *schema.Profile, name string) (schema.NodeType, bool) {
	for _, nt := range profile.NodeTypes {
		if nt.Name == name && nt.Scope == "project" {
			return nt, true
		}
	}
	return schema.NodeType{}, false
}

// edgeTargetSet returns the set of identity hashes an edge's "to" type name
// resolves against for one module: that module's own array when the type is
// module-scoped (typed if it is one of the five built-ins, generic
// otherwise), else project.json's array when the type is project-scoped.
func edgeTargetSet(specDir, modPath string, mod *schema.ModuleSpec, project *schema.Project, profile *schema.Profile, typeName string) (map[string]bool, error) {
	if set, ok := moduleTypeIDSet(mod, typeName); ok {
		return set, nil
	}
	if nt, ok := findModuleNodeType(profile, typeName); ok {
		entries, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
		if err != nil {
			return nil, err
		}
		return entriesIDSet(entries), nil
	}
	if set, ok := projectTypeIDSet(project, typeName); ok {
		return set, nil
	}
	if nt, ok := findProjectNodeType(profile, typeName); ok {
		entries, err := rawProjectEntries(specDir, nt.PluralKey)
		if err != nil {
			return nil, err
		}
		return entriesIDSet(entries), nil
	}
	return nil, fmt.Errorf("no array declared for node type %q", typeName)
}

// projectEdgeTargetSet is edgeTargetSet's project-scoped counterpart, used
// by checkExtraProjectEdges: a project-scoped source has no owning module,
// so a target can only resolve against project.json — the built-in
// requirement/module concepts typed via projectTypeIDSet, or a
// profile-declared project-scoped type read generically. Unlike
// edgeTargetSet, there is no module-scoped array to fall back to.
func projectEdgeTargetSet(specDir string, project *schema.Project, profile *schema.Profile, typeName string) (map[string]bool, error) {
	if set, ok := projectTypeIDSet(project, typeName); ok {
		return set, nil
	}
	if nt, ok := findProjectNodeType(profile, typeName); ok {
		entries, err := rawProjectEntries(specDir, nt.PluralKey)
		if err != nil {
			return nil, err
		}
		return entriesIDSet(entries), nil
	}
	return nil, fmt.Errorf("no array declared for node type %q", typeName)
}

// moduleTypeIDSet returns the set of identity hashes for one of the five
// built-in module-scoped node types, read from schema.ModuleSpec's typed
// fields.
func moduleTypeIDSet(mod *schema.ModuleSpec, typeName string) (map[string]bool, bool) {
	switch typeName {
	case "component":
		return idSet(compIDs(mod.Components)), true
	case "data_flow":
		return idSet(flowIDs(mod.DataFlows)), true
	case "test_section":
		return idSet(testSectionIDs(mod.TestSections)), true
	case "api":
		return idSet(apiIDs(mod.APIs)), true
	case "requirement":
		return idSet(moduleReqIDs(mod.Requirements)), true
	}
	return nil, false
}

// projectTypeIDSet returns the set of identity hashes for a built-in
// project-scoped concept: "requirement" (project.json's typed field) or
// "module" (the fixed interior-node concept requires_module points at).
func projectTypeIDSet(project *schema.Project, typeName string) (map[string]bool, bool) {
	switch typeName {
	case "requirement":
		return idSet(reqIDs(project.Requirements)), true
	case "module":
		return idSet(moduleIDs(project.Modules)), true
	}
	return nil, false
}

// modulePathByName looks up a module's directory path from project.json's
// module list.
func modulePathByName(project *schema.Project, modName string) string {
	for _, m := range project.Modules {
		if m.Name == modName {
			return m.Path
		}
	}
	return modName
}

// namedEntry is one node's (id, name) pair, the input CheckIDDerivation
// hashes and nameShapeError inspects, sourced either from a typed
// schema.ModuleSpec field or generically from raw JSON for a
// profile-declared type schema.ModuleSpec has no field for.
type namedEntry struct {
	id   string
	name string
}

// moduleTypedEntries returns the (id, name) pairs schema.ModuleSpec already
// carries typed for one of the five built-in module-scoped node types. A
// requirement's declaration name is its title — the one built-in type with
// no Name field of its own — every other type uses its Name field directly.
// Shared by checkNameRecoverability (which reads name-declarable types) and
// CheckIDDerivation (which reads every module-scoped type): both need one
// (id, name) pair per node, from the field carrying its declared identity.
func moduleTypedEntries(mod *schema.ModuleSpec, typeName string) ([]namedEntry, bool) {
	switch typeName {
	case "requirement":
		out := make([]namedEntry, len(mod.Requirements))
		for i, r := range mod.Requirements {
			out[i] = namedEntry{id: r.ID, name: r.Title}
		}
		return out, true
	case "component":
		out := make([]namedEntry, len(mod.Components))
		for i, c := range mod.Components {
			out[i] = namedEntry{id: c.ID, name: c.Name}
		}
		return out, true
	case "data_flow":
		out := make([]namedEntry, len(mod.DataFlows))
		for i, f := range mod.DataFlows {
			out[i] = namedEntry{id: f.ID, name: f.Name}
		}
		return out, true
	case "test_section":
		out := make([]namedEntry, len(mod.TestSections))
		for i, t := range mod.TestSections {
			out[i] = namedEntry{id: t.ID, name: t.Name}
		}
		return out, true
	case "api":
		out := make([]namedEntry, len(mod.APIs))
		for i, a := range mod.APIs {
			out[i] = namedEntry{id: a.ID, name: a.Name}
		}
		return out, true
	}
	return nil, false
}

// rawEntry is one array element read directly off a module.json or
// project.json document, used for a node type the resolved profile declares
// beyond the ones schema.ModuleSpec and schema.Project carry dedicated Go
// fields for.
type rawEntry map[string]json.RawMessage

// str reads field as a JSON string, returning "" if absent or not a string.
func (e rawEntry) str(field string) string {
	v, ok := e[field]
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(v, &s)
	return s
}

// strSlice reads field as a JSON array of strings, returning nil if absent
// or not shaped that way.
func (e rawEntry) strSlice(field string) []string {
	v, ok := e[field]
	if !ok {
		return nil
	}
	var s []string
	_ = json.Unmarshal(v, &s)
	return s
}

// namedEntriesFromRaw converts generically-read entries to namedEntry pairs
// using each entry's "id" and "name" fields.
func namedEntriesFromRaw(raw []rawEntry) []namedEntry {
	out := make([]namedEntry, len(raw))
	for i, e := range raw {
		out[i] = namedEntry{id: e.str("id"), name: e.str("name")}
	}
	return out
}

// entriesIDSet collects the "id" field of a slice of generically-read
// entries into a set.
func entriesIDSet(entries []rawEntry) map[string]bool {
	set := make(map[string]bool, len(entries))
	for _, e := range entries {
		set[e.str("id")] = true
	}
	return set
}

// rawModuleEntries reads one module's module.json array at pluralKey
// generically, for a node type the resolved profile declares beyond the
// five built into schema.ModuleSpec.
func rawModuleEntries(specDir, modPath, pluralKey string) ([]rawEntry, error) {
	data, err := os.ReadFile(filepath.Join(specDir, modPath, "module.json"))
	if err != nil {
		return nil, fmt.Errorf("read module.json: %w", err)
	}
	return rawArrayEntries(data, pluralKey)
}

// rawProjectEntries reads project.json's array at pluralKey generically, for
// a project-scoped node type the resolved profile declares beyond
// "requirement".
func rawProjectEntries(specDir, pluralKey string) ([]rawEntry, error) {
	data, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	return rawArrayEntries(data, pluralKey)
}

// rawArrayEntries decodes the array at pluralKey in a JSON document into
// generic entries. A document carrying no such array yields no entries and
// no error — the array is simply empty or the type has no instances yet.
func rawArrayEntries(doc []byte, pluralKey string) ([]rawEntry, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(doc, &top); err != nil {
		return nil, err
	}
	arr, ok := top[pluralKey]
	if !ok {
		return nil, nil
	}
	var entries []rawEntry
	if err := json.Unmarshal(arr, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ID extraction helpers. All return slices of identity-hash strings.

func reqIDs(reqs []schema.Requirement) []string {
	ids := make([]string, len(reqs))
	for i, r := range reqs {
		ids[i] = r.ID
	}
	return ids
}

func moduleIDs(mods []schema.Module) []string {
	ids := make([]string, len(mods))
	for i, m := range mods {
		ids[i] = m.ID
	}
	return ids
}

func sectionIDs(secs []schema.Section) []string {
	ids := make([]string, len(secs))
	for i, s := range secs {
		ids[i] = s.ID
	}
	return ids
}

func moduleReqIDs(reqs []schema.ModuleRequirement) []string {
	ids := make([]string, len(reqs))
	for i, r := range reqs {
		ids[i] = r.ID
	}
	return ids
}

func compIDs(comps []schema.Component) []string {
	ids := make([]string, len(comps))
	for i, c := range comps {
		ids[i] = c.ID
	}
	return ids
}

func flowIDs(flows []schema.DataFlow) []string {
	ids := make([]string, len(flows))
	for i, f := range flows {
		ids[i] = f.ID
	}
	return ids
}

func apiIDs(apis []schema.API) []string {
	ids := make([]string, len(apis))
	for i, a := range apis {
		ids[i] = a.ID
	}
	return ids
}

func testSectionIDs(sections []schema.TestSection) []string {
	ids := make([]string, len(sections))
	for i, s := range sections {
		ids[i] = s.ID
	}
	return ids
}

// idSet converts a slice of identity-hash strings to a set for O(1) lookup.
func idSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
