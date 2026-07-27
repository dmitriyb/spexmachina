package validator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckIDs validates identity-hash uniqueness within each array and
// cross-reference integrity across the spec. It runs uniqueness checks
// first — if IDs are duplicated, cross-reference checks may be
// misleading because references cannot be unambiguously resolved.
//
// Every check uses string set membership (map[string]bool). There is no
// integer parsing, no path decomposition, and no comparison across
// types — references are identity hashes looked up against per-array
// sets.
func CheckIDs(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "id")
	if len(errs) > 0 {
		return errs
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
	result = append(result, checkAPINameUniqueness(modNames, modules)...)
	result = append(result, checkNameRecoverability(modNames, modules)...)

	if len(result) > 0 {
		return result
	}

	result = append(result, checkProjectRefs(project)...)
	result = append(result, checkProjectPriority(project)...)
	for _, modName := range modNames {
		result = append(result, checkModuleRefs(modName, modules[modName], project)...)
	}

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
	errs = append(errs, checkDuplicateIDs(prefix+"/impl_sections", implIDs(mod.ImplSections))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/data_flows", flowIDs(mod.DataFlows))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/test_sections", testSectionIDs(mod.TestSections))...)
	errs = append(errs, checkDuplicateIDs(prefix+"/apis", apiIDs(mod.APIs))...)

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

// checkNameRecoverability enforces the shape an api or component name must
// have for the removal-time sweep to be able to find it again.
//
// The sweep recovers a removed node's name by hashing corpus phrases against
// its key (CheckRemovedNames), and every phrase it builds is a join of
// tokenizeCorpus tokens with single spaces. So the set of findable names is
// exactly the set of names that survive that tokenization unchanged, and this
// check is that predicate applied at the point of declaration: declarableName,
// the same function stated in terms of the same tokenizer the sweep uses.
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
func checkNameRecoverability(modNames []string, modules map[string]*schema.ModuleSpec) []ValidationError {
	var errs []ValidationError
	for _, modName := range modNames {
		mod := modules[modName]
		if mod == nil {
			continue
		}
		prefix := modName + "/module.json:"
		for _, comp := range mod.Components {
			errs = append(errs, nameShapeError(prefix, "components", "component", comp.Name, comp.ID)...)
		}
		for _, api := range mod.APIs {
			errs = append(errs, nameShapeError(prefix, "apis", "api", api.Name, api.ID)...)
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

	for _, sec := range mod.ImplSections {
		for _, descID := range sec.Describes {
			if !compSet[descID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("%s/impl_sections/%s", prefix, sec.ID),
					Message:  fmt.Sprintf("describes references non-existent component %s", descID),
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

func implIDs(secs []schema.ImplSection) []string {
	ids := make([]string, len(secs))
	for i, s := range secs {
		ids[i] = s.ID
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
