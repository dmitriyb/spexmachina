package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckRequirementCoverage verifies that every project requirement is derived
// into at least one module requirement (via preq_id), and every module
// requirement is implemented by at least one component (via implements).
func CheckRequirementCoverage(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "requirement_coverage")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError

	// TODO(bead:spexmachina-yxg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	// Phase 1: project requirement → module requirement coverage (preq_id).
	coveredProjectReqs := make(map[string]bool)
	for _, mod := range modules {
		for _, req := range mod.Requirements {
			if req.PreqID != "" {
				coveredProjectReqs[req.PreqID] = true
			}
		}
	}
	for _, req := range project.Requirements {
		if !coveredProjectReqs[fmt.Sprintf("%d", req.ID)] {
			result = append(result, ValidationError{
				Check:    "requirement_coverage",
				Severity: "error",
				Path:     "project.json",
				Message:  fmt.Sprintf("project requirement %d %q is not derived into any module requirement", req.ID, req.Title),
			})
		}
	}

	// Phase 2: module requirement → component coverage (implements).
	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		mod := modules[modName]
		result = append(result, detectUncoveredRequirements(modName, mod)...)
	}

	return result
}

// TODO(bead:spexmachina-yxg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// detectUncoveredRequirements finds module requirements not referenced by any
// component's implements array within the module.
func detectUncoveredRequirements(modName string, mod *schema.ModuleSpec) []ValidationError {
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
				Message:  fmt.Sprintf("%s requirement %s %q is not implemented by any component", modName, req.ID, req.Title),
			})
		}
	}
	return errs
}
