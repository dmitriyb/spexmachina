package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckTestCoverage verifies that every component in each module is described
// by at least one test_section. Uncovered components are reported as errors.
func CheckTestCoverage(specDir string) []ValidationError {
	_, modules, errs := loadSpec(specDir, "test_coverage")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		mod := modules[modName]
		result = append(result, detectUncoveredComponents(modName, mod)...)
	}

	return result
}

// TODO(bead:spexmachina-2yf): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// detectUncoveredComponents finds components not referenced by any
// test_section's describes array within the module.
func detectUncoveredComponents(modName string, mod *schema.ModuleSpec) []ValidationError {
	coveredComps := make(map[string]bool)
	for _, ts := range mod.TestSections {
		for _, compID := range ts.Describes {
			coveredComps[compID] = true
		}
	}

	var errs []ValidationError
	for _, comp := range mod.Components {
		if !coveredComps[comp.ID] {
			errs = append(errs, ValidationError{
				Check:    "test_coverage",
				Severity: "error",
				Path:     fmt.Sprintf("%s/module.json:/components/%s", modName, comp.ID),
				Message:  fmt.Sprintf("component %s (id:%s) has no test_section coverage", comp.Name, comp.ID),
			})
		}
	}
	return errs
}
