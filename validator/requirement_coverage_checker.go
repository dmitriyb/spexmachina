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
		errs, ns := checkProjectRequirementCoverage(project, modules, chain)
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
			result = append(result, detectUncoveredRequirements(modName, modules[modName], chain)...)
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
// requirement deriving from them (via preq_id), reporting each as an error
// unless it declares derivation "pending", in which case a disclosure note
// stands in the error's place. chain's declared type and scope names are
// interpolated into both message shapes.
func checkProjectRequirementCoverage(project *schema.Project, modules map[string]*schema.ModuleSpec, chain schema.CoverageChain) ([]ValidationError, []ValidationNote) {
	coveredProjectReqs := make(map[string]bool)
	for _, mod := range modules {
		for _, req := range mod.Requirements {
			if req.PreqID != "" {
				coveredProjectReqs[req.PreqID] = true
			}
		}
	}

	coveredLabel := coverageLabel(chain.CoveredScope, chain.CoveredType)
	coveringLabel := coverageLabel(chain.CoveringScope, chain.CoveringType)

	var result []ValidationError
	var notes []ValidationNote
	for _, req := range project.Requirements {
		if coveredProjectReqs[req.ID] {
			continue
		}
		if req.Derivation == "pending" {
			notes = append(notes, ValidationNote{
				Type:    "pending_derivation",
				Message: fmt.Sprintf("%s %s %q declares derivation pending and is not derived into any %s", coveredLabel, req.ID, req.Title, coveringLabel),
				Related: []string{req.ID},
			})
			continue
		}
		result = append(result, ValidationError{
			Check:    "requirement_coverage",
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("%s %s %q is not derived into any %s", coveredLabel, req.ID, req.Title, coveringLabel),
		})
	}
	return result, notes
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
// any component's implements array within the module. chain's declared
// covered/covering type names are interpolated into the message; the module
// name itself — not a scope word — leads it, per arch_requirement_coverage_checker.md.
func detectUncoveredRequirements(modName string, mod *schema.ModuleSpec, chain schema.CoverageChain) []ValidationError {
	coveredReqs := make(map[string]bool)
	for _, comp := range mod.Components {
		for _, reqID := range comp.Implements {
			coveredReqs[reqID] = true
		}
	}

	var errs []ValidationError
	for _, req := range mod.Requirements {
		if !coveredReqs[req.ID] {
			errs = append(errs, ValidationError{
				Check:    "requirement_coverage",
				Severity: "error",
				Path:     fmt.Sprintf("%s/module.json", modName),
				Message:  fmt.Sprintf("%s %s %s %q is not implemented by any %s", modName, chain.CoveredType, req.ID, req.Title, chain.CoveringType),
			})
		}
	}
	return errs
}
