package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckRequirementCoverage verifies the resolved profile's two declared
// coverage chains covering its completeness-trigger types: every project
// requirement derived into at least one module requirement, and every
// module requirement implemented by at least one component. Which chains
// these are — the covered/covering type names, per the default profile
// "requirement"/"requirement" and "requirement"/"component" — is read off
// the profile rather than fixed in the checker; a profile that drops one of
// the two chains drops the check it enforces, and a profile that renames a
// type sees its declared name interpolated into the same message shapes. A
// project requirement declaring derivation "pending" is exempt from the
// first link: its underived state is reported as a disclosure note instead
// of an error. The module-requirement-to-component link admits no such
// exemption.
func CheckRequirementCoverage(specDir string) ([]ValidationError, []ValidationNote) {
	project, modules, errs := loadSpec(specDir, "requirement_coverage")
	if len(errs) > 0 {
		return errs, nil
	}

	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}, nil
	}

	var result []ValidationError
	var notes []ValidationNote

	if chain, ok := projectDerivationChain(profile); ok {
		errs, ns := checkProjectRequirementCoverage(specDir, profile, project, modules, chain)
		result = append(result, errs...)
		notes = append(notes, ns...)
	}

	if chain, ok := moduleImplementationChain(profile); ok {
		modNames := make([]string, 0, len(modules))
		for name := range modules {
			modNames = append(modNames, name)
		}
		slices.Sort(modNames)

		for _, modName := range modNames {
			modPath := modulePathByName(project, modName)
			result = append(result, detectUncoveredRequirements(specDir, modPath, modName, modules[modName], profile, chain)...)
		}
	}

	return result, notes
}

// projectDerivationChain resolves the declared coverage chain covering the
// profile's project-scoped completeness-trigger type — the default
// profile's project-requirement-to-module-requirement (preq_id) link. Not
// found — the type undeclared at project scope, or no chain covers it — the
// checker's first link simply does not run: a dropped chain drops its check.
func projectDerivationChain(profile *schema.Profile) (schema.CoverageChain, bool) {
	nt, ok := completenessTriggerType(profile, "project")
	if !ok {
		return schema.CoverageChain{}, false
	}
	return findCoverageChain(profile, nt.Name, "project")
}

// moduleImplementationChain is projectDerivationChain's module-scoped
// counterpart: the declared chain covering the profile's module-scoped
// completeness-trigger type — the default profile's
// module-requirement-to-component (implements) link.
func moduleImplementationChain(profile *schema.Profile) (schema.CoverageChain, bool) {
	nt, ok := completenessTriggerType(profile, "module")
	if !ok {
		return schema.CoverageChain{}, false
	}
	return findCoverageChain(profile, nt.Name, "module")
}

// completenessTriggerType returns the resolved profile's node type flagged
// completeness_trigger at the given scope — the type name whose coverage
// chain this checker looks for, not a compiled-in "requirement" literal.
func completenessTriggerType(profile *schema.Profile, scope string) (schema.NodeType, bool) {
	for _, t := range profile.NodeTypes {
		if t.Scope == scope && t.CompletenessTrigger {
			return t, true
		}
	}
	return schema.NodeType{}, false
}

// findCoverageChain returns the resolved profile's declared coverage chain
// whose covered type and scope match, or false when the profile declares no
// such chain.
func findCoverageChain(profile *schema.Profile, coveredType, coveredScope string) (schema.CoverageChain, bool) {
	for _, c := range profile.CoverageChains {
		if c.CoveredType == coveredType && c.CoveredScope == coveredScope {
			return c, true
		}
	}
	return schema.CoverageChain{}, false
}

// checkProjectRequirementCoverage finds project requirements with no module
// requirement deriving from them (via chain.Edge, "preq_id" under the
// default profile), reporting each as an error unless it declares
// derivation "pending", in which case a disclosure note stands in the
// error's place. chain's declared type and scope names are interpolated
// into both message shapes. Both the covered side (project.json's array)
// and the covering side (every module's array) are read off the chain's
// declared plural keys and edge field — the typed schema.Project/
// schema.ModuleSpec fields are used as a fast path only when the chain
// still names the default profile's type and edge, generically off raw JSON
// otherwise, so a profile renaming either type is still scanned correctly.
func checkProjectRequirementCoverage(specDir string, profile *schema.Profile, project *schema.Project, modules map[string]*schema.ModuleSpec, chain schema.CoverageChain) ([]ValidationError, []ValidationNote) {
	covered, err := projectCoveredEntries(specDir, profile, project, chain)
	if err != nil {
		return []ValidationError{{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     "project.json",
			Message:  err.Error(),
		}}, nil
	}

	coveredProjectReqs := make(map[string]bool)
	for modName, mod := range modules {
		modPath := modulePathByName(project, modName)
		targets, err := coveringEdgeTargets(specDir, modPath, mod, profile, chain)
		if err != nil {
			continue
		}
		for _, id := range targets {
			coveredProjectReqs[id] = true
		}
	}

	coveredLabel := coverageLabel(chain.CoveredScope, chain.CoveredType)
	coveringLabel := coverageLabel(chain.CoveringScope, chain.CoveringType)

	var result []ValidationError
	var notes []ValidationNote
	for _, req := range covered {
		if coveredProjectReqs[req.id] {
			continue
		}
		if req.derivation == "pending" {
			notes = append(notes, ValidationNote{
				Type:    "pending_derivation",
				Message: fmt.Sprintf("%s %s %q declares derivation pending and is not derived into any %s", coveredLabel, req.id, req.title, coveringLabel),
				Related: []string{req.id},
			})
			continue
		}
		result = append(result, ValidationError{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("%s %s %q is not derived into any %s", coveredLabel, req.id, req.title, coveringLabel),
		})
	}
	return result, notes
}

// coverageNode is one covered-side node's (id, title) pair, plus —
// project-scope only — its declared derivation state, read either from a
// typed schema field or generically off raw JSON for a chain naming a
// profile-renamed type.
type coverageNode struct {
	id         string
	title      string
	derivation string
}

// projectCoveredEntries returns the coverage chain's covered-side nodes at
// project scope: schema.Project's typed Requirements field when the chain
// still names the default profile's project-scoped completeness-trigger
// type ("requirement"), generically off project.json's array at the
// resolved NodeType's plural_key otherwise.
func projectCoveredEntries(specDir string, profile *schema.Profile, project *schema.Project, chain schema.CoverageChain) ([]coverageNode, error) {
	if chain.CoveredType == "requirement" {
		out := make([]coverageNode, len(project.Requirements))
		for i, r := range project.Requirements {
			out[i] = coverageNode{id: r.ID, title: r.Title, derivation: r.Derivation}
		}
		return out, nil
	}

	nt, ok := findProjectNodeType(profile, chain.CoveredType)
	if !ok {
		return nil, nil
	}
	entries, err := rawProjectEntries(specDir, nt.PluralKey)
	if err != nil {
		return nil, err
	}
	out := make([]coverageNode, len(entries))
	for i, e := range entries {
		out[i] = coverageNode{id: e.str("id"), title: e.str("title"), derivation: e.str("derivation")}
	}
	return out, nil
}

// moduleCoveredEntries is projectCoveredEntries' module-scoped counterpart,
// used by the module-requirement-to-component link: schema.ModuleSpec's
// typed Requirements field when the chain still names the default profile's
// module-scoped completeness-trigger type ("requirement"), generically off
// module.json's array at the resolved NodeType's plural_key otherwise. The
// module-level link admits no derivation exemption, so no derivation field
// is read here.
func moduleCoveredEntries(specDir, modPath string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) ([]coverageNode, error) {
	if chain.CoveredType == "requirement" {
		out := make([]coverageNode, len(mod.Requirements))
		for i, r := range mod.Requirements {
			out[i] = coverageNode{id: r.ID, title: r.Title}
		}
		return out, nil
	}

	nt, ok := findModuleNodeType(profile, chain.CoveredType)
	if !ok {
		return nil, nil
	}
	entries, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
	if err != nil {
		return nil, err
	}
	out := make([]coverageNode, len(entries))
	for i, e := range entries {
		out[i] = coverageNode{id: e.str("id"), title: e.str("title")}
	}
	return out, nil
}

// coveringEdgeTargets returns the ids one module's covering-side entries
// reference via the coverage chain's declared edge field — component's
// Implements or module requirement's PreqID via the typed schema.ModuleSpec
// fields as a fast path, used only when the chain still names the default
// profile's covering type and edge for that link (component/implements,
// requirement/preq_id); generically off the resolved covering NodeType's
// plural_key and the chain's edge field otherwise, exactly as
// checkExtraModuleDAGEdges reads a profile-declared edge kind. Both links'
// covering side is always module-scoped, so the covering type is always
// resolved via findModuleNodeType regardless of chain.CoveringScope, which
// only exists to disambiguate a shared type name in messages.
func coveringEdgeTargets(specDir, modPath string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) ([]string, error) {
	switch {
	case chain.CoveringType == "component" && chain.Edge == "implements":
		var out []string
		for _, c := range mod.Components {
			out = append(out, c.Implements...)
		}
		return out, nil
	case chain.CoveringType == "requirement" && chain.Edge == "preq_id":
		var out []string
		for _, r := range mod.Requirements {
			if r.PreqID != "" {
				out = append(out, r.PreqID)
			}
		}
		return out, nil
	}

	nt, ok := findModuleNodeType(profile, chain.CoveringType)
	if !ok {
		return nil, nil
	}
	entries, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.edgeTargets(chain.Edge)...)
	}
	return out, nil
}

// edgeTargets reads a raw entry's edge field generically as a slice of
// target ids: a scalar string field (e.g. "preq_id") yields one entry, a
// string-array field (e.g. "implements") yields all of them, and an absent
// or differently-shaped field yields none. str and strSlice each fail
// silently on the other's shape, so trying both covers either encoding
// without needing to know up front which one a given edge uses.
func (e rawEntry) edgeTargets(field string) []string {
	if s := e.str(field); s != "" {
		return []string{s}
	}
	return e.strSlice(field)
}

// coverageLabel joins a coverage chain's scope and type name into the
// phrase the project-level message shape interpolates, e.g. "project" +
// "requirement" -> "project requirement". A chain that declares no scope
// (the type is unambiguous without one) yields the bare type name.
func coverageLabel(scope, typeName string) string {
	if scope == "" {
		return typeName
	}
	return scope + " " + typeName
}

// detectUncoveredRequirements finds module requirements not referenced by
// any covering entry's edge field within the module — comp.Implements under
// the default profile, or a profile-renamed covering type's edge field read
// generically. chain's declared covered/covering type names are
// interpolated into the message; the module name itself — not a scope word
// — leads it, per arch_requirement_coverage_checker.md.
func detectUncoveredRequirements(specDir, modPath, modName string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) []ValidationError {
	covered, err := moduleCoveredEntries(specDir, modPath, mod, profile, chain)
	if err != nil {
		return []ValidationError{{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     fmt.Sprintf("%s/module.json", modName),
			Message:  err.Error(),
		}}
	}
	targets, err := coveringEdgeTargets(specDir, modPath, mod, profile, chain)
	if err != nil {
		return []ValidationError{{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     fmt.Sprintf("%s/module.json", modName),
			Message:  err.Error(),
		}}
	}

	coveredReqs := make(map[string]bool, len(targets))
	for _, id := range targets {
		coveredReqs[id] = true
	}

	var errs []ValidationError
	for _, req := range covered {
		if !coveredReqs[req.id] {
			errs = append(errs, ValidationError{
				Check:    "requirement_coverage",
				Severity: "error",
				Path:     fmt.Sprintf("%s/module.json", modName),
				Message:  fmt.Sprintf("%s %s %s %q is not implemented by any %s", modName, chain.CoveredType, req.id, req.title, chain.CoveringType),
			})
		}
	}
	return errs
}
