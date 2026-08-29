package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckTestCoverage verifies the resolved profile's declared coverage chain
// covering test sections — the default profile's component-describes-
// test_section link: every node of the chain's covered type must be the
// target of at least one edge of the chain's declared kind from some node
// of test_section. The chain is located by its covering type,
// "test_section", rather than by its covered type's name: test_section is
// this checker's own fixed domain, while the covered type's declared name is
// free for a profile to rename. A profile declaring no chain covering
// test_section drops the check entirely.
func CheckTestCoverage(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "test_coverage")
	if len(errs) > 0 {
		return errs
	}

	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "test_coverage",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}
	}

	chain, ok := testCoverageChain(profile)
	if !ok {
		return nil
	}

	var result []ValidationError

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		modPath := modulePathByName(project, modName)
		result = append(result, detectUncoveredComponents(specDir, modPath, modName, modules[modName], profile, chain)...)
	}

	return result
}

// testCoverageChain returns the resolved profile's declared coverage chain
// whose covering type is "test_section" — the default profile's
// component-describes-test_section link. Not found — the profile declares
// no chain covering test_section — and the check simply does not run: a
// dropped chain drops its check.
func testCoverageChain(profile *schema.Profile) (schema.CoverageChain, bool) {
	for _, c := range profile.CoverageChains {
		if c.CoveringType == "test_section" {
			return c, true
		}
	}
	return schema.CoverageChain{}, false
}

// detectUncoveredComponents finds the coverage chain's covered-side nodes
// not referenced by any test_section's edge field within the module —
// mod.Components against mod.TestSections[].Describes as a typed fast path
// when the chain still names the default profile's component/describes
// pair, generically off the resolved covered NodeType's plural_key and the
// chain's edge field otherwise, exactly as detectUncoveredRequirements reads
// a profile-renamed coverage chain.
func detectUncoveredComponents(specDir, modPath, modName string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) []ValidationError {
	covered, err := coveredComponentEntries(specDir, modPath, mod, profile, chain)
	if err != nil {
		return []ValidationError{{
			Check:    "test_coverage",
			Severity: "error",
			Path:     fmt.Sprintf("%s/module.json", modName),
			Message:  err.Error(),
		}}
	}

	coveredIDs, err := testSectionEdgeTargets(specDir, modPath, mod, profile, chain)
	if err != nil {
		return []ValidationError{{
			Check:    "test_coverage",
			Severity: "error",
			Path:     fmt.Sprintf("%s/module.json", modName),
			Message:  err.Error(),
		}}
	}

	var errs []ValidationError
	for _, comp := range covered {
		if !coveredIDs[comp.id] {
			errs = append(errs, ValidationError{
				Check:    "test_coverage",
				Severity: "error",
				Path:     fmt.Sprintf("%s/module.json:/components/%s", modName, comp.id),
				Message:  fmt.Sprintf("%s %s (id:%s) has no %s coverage", chain.CoveredType, comp.name, comp.id, chain.CoveringType),
			})
		}
	}
	return errs
}

// coveredComponentEntries returns the coverage chain's covered-side nodes
// for one module: mod.Components's typed (id, name) pairs when the chain
// still names one of schema.ModuleSpec's built-in types ("component" under
// the default profile), generically off the resolved covered NodeType's
// plural_key otherwise.
func coveredComponentEntries(specDir, modPath string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) ([]namedEntry, error) {
	if entries, ok := moduleTypedEntries(mod, chain.CoveredType); ok {
		return entries, nil
	}

	nt, ok := findModuleNodeType(profile, chain.CoveredType)
	if !ok {
		return nil, nil
	}
	raw, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
	if err != nil {
		return nil, err
	}
	return namedEntriesFromRaw(raw), nil
}

// testSectionEdgeTargets returns the set of ids one module's test_sections
// reference via the coverage chain's declared edge field: mod.TestSections'
// typed Describes field as a fast path when the chain still names the
// default profile's edge ("describes"), generically off test_section's
// resolved plural_key and the chain's edge field otherwise, so a profile
// renaming the edge kind is still read correctly.
func testSectionEdgeTargets(specDir, modPath string, mod *schema.ModuleSpec, profile *schema.Profile, chain schema.CoverageChain) (map[string]bool, error) {
	set := make(map[string]bool)

	if chain.Edge == "describes" {
		for _, ts := range mod.TestSections {
			for _, id := range ts.Describes {
				set[id] = true
			}
		}
		return set, nil
	}

	nt, ok := findModuleNodeType(profile, chain.CoveringType)
	if !ok {
		return set, nil
	}
	raw, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
	if err != nil {
		return nil, err
	}
	for _, e := range raw {
		for _, id := range e.edgeTargets(chain.Edge) {
			set[id] = true
		}
	}
	return set, nil
}
