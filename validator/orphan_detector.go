package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckOrphans finds requirements not implemented by any component and
// components not described by any impl_section. Orphans are warnings, not
// errors — a spec in active development may have requirements awaiting
// component assignment.
func CheckOrphans(specDir string) []ValidationError {
	_, modules, errs := loadSpec(specDir, "orphan")
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
		result = append(result, detectOrphanRequirements(modName, mod)...)
		result = append(result, detectOrphanComponents(modName, mod)...)
	}

	return result
}

// TODO(bead:spexmachina-wei): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// detectOrphanRequirements finds requirements not referenced by any component's
// implements array within the module.
func detectOrphanRequirements(modName string, mod *schema.ModuleSpec) []ValidationError {
	implReqs := make(map[string]bool)
	for _, comp := range mod.Components {
		for _, reqID := range comp.Implements {
			implReqs[reqID] = true
		}
	}

	var errs []ValidationError
	for _, req := range mod.Requirements {
		if !implReqs[req.ID] {
			errs = append(errs, ValidationError{
				Check:    "orphan",
				Severity: "warning",
				Path:     fmt.Sprintf("%s/module.json:/requirements/%s", modName, req.ID),
				Message:  fmt.Sprintf("requirement %s (%s) is not implemented by any component", req.ID, req.Title),
			})
		}
	}
	return errs
}

// TODO(bead:spexmachina-wei): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// detectOrphanComponents finds components not referenced by any impl_section's
// describes array within the module.
func detectOrphanComponents(modName string, mod *schema.ModuleSpec) []ValidationError {
	describedComps := make(map[string]bool)
	for _, sec := range mod.ImplSections {
		for _, compID := range sec.Describes {
			describedComps[compID] = true
		}
	}

	var errs []ValidationError
	for _, comp := range mod.Components {
		if !describedComps[comp.ID] {
			errs = append(errs, ValidationError{
				Check:    "orphan",
				Severity: "warning",
				Path:     fmt.Sprintf("%s/module.json:/components/%s", modName, comp.ID),
				Message:  fmt.Sprintf("component %s (%s) is not described by any impl_section", comp.ID, comp.Name),
			})
		}
	}
	return errs
}
