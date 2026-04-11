package validator

import (
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckIDs validates ID uniqueness within each array and cross-reference
// integrity across the spec. It runs uniqueness checks first — if IDs are
// duplicated, cross-reference checks may be misleading.
func CheckIDs(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "id")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError

	// Phase 1: ID uniqueness.
	result = append(result, checkProjectUniqueness(project)...)

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		result = append(result, checkModuleUniqueness(modName, modules[modName])...)
	}

	// If there are duplicate IDs, skip cross-reference checks.
	if len(result) > 0 {
		return result
	}

	// Phase 2: Cross-reference integrity and mandatory fields.
	result = append(result, checkProjectRefs(project)...)
	result = append(result, checkProjectPriority(project)...)
	for _, modName := range modNames {
		result = append(result, checkModuleRefs(modName, modules[modName], project)...)
	}

	return result
}

// checkProjectUniqueness checks for duplicate IDs in project-level arrays.
func checkProjectUniqueness(project *schema.Project) []ValidationError {
	var errs []ValidationError

	errs = append(errs, checkDuplicateIDs("project.json:/requirements", reqIDs(project.Requirements))...)
	errs = append(errs, checkDuplicateIDs("project.json:/modules", moduleIDs(project.Modules))...)
	errs = append(errs, checkDuplicateIDs("project.json:/milestones", milestoneIDs(project.Milestones))...)

	return errs
}

// TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// checkModuleUniqueness checks for duplicate IDs in module-level arrays.
func checkModuleUniqueness(modName string, mod *schema.ModuleSpec) []ValidationError {
	var errs []ValidationError
	prefix := modName + "/module.json:"

	errs = append(errs, checkDuplicateStringIDs(prefix+"/requirements", moduleReqIDs(mod.Requirements))...)
	errs = append(errs, checkDuplicateStringIDs(prefix+"/components", compIDs(mod.Components))...)
	errs = append(errs, checkDuplicateStringIDs(prefix+"/impl_sections", implIDs(mod.ImplSections))...)
	errs = append(errs, checkDuplicateStringIDs(prefix+"/data_flows", flowIDs(mod.DataFlows))...)

	return errs
}

// checkDuplicateIDs reports any IDs that appear more than once.
func checkDuplicateIDs(path string, ids []int) []ValidationError {
	seen := make(map[int]int, len(ids))
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
				Message:  fmt.Sprintf("duplicate ID %d", id),
			})
		}
	}
	return errs
}

// checkProjectRefs validates cross-references within project.json.
func checkProjectRefs(project *schema.Project) []ValidationError {
	var errs []ValidationError

	projReqSet := idSet(reqIDs(project.Requirements))
	modIDSet := idSet(moduleIDs(project.Modules))

	// requirement.depends_on → project requirement IDs.
	for _, req := range project.Requirements {
		for _, depID := range req.DependsOn {
			if !projReqSet[depID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/requirements/%d", req.ID),
					Message:  fmt.Sprintf("depends_on references non-existent requirement %d", depID),
				})
			}
		}
	}

	// module.requires_module → module IDs.
	for _, mod := range project.Modules {
		for _, depID := range mod.RequiresModule {
			if !modIDSet[depID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/modules/%d", mod.ID),
					Message:  fmt.Sprintf("requires_module references non-existent module %d", depID),
				})
			}
		}
	}

	// milestone.groups → module IDs.
	for _, ms := range project.Milestones {
		for _, groupID := range ms.Groups {
			if !modIDSet[groupID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/milestones/%d", ms.ID),
					Message:  fmt.Sprintf("groups references non-existent module %d", groupID),
				})
			}
		}
	}

	return errs
}

// checkProjectPriority validates that every project requirement has a priority in range 0-4.
func checkProjectPriority(project *schema.Project) []ValidationError {
	var errs []ValidationError
	for _, req := range project.Requirements {
		if req.Priority == nil {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("project.json:/requirements/%d", req.ID),
				Message:  fmt.Sprintf("project requirement %d missing priority", req.ID),
			})
		} else if *req.Priority < 0 || *req.Priority > 4 {
			errs = append(errs, ValidationError{
				Check:    "id",
				Severity: "error",
				Path:     fmt.Sprintf("project.json:/requirements/%d", req.ID),
				Message:  fmt.Sprintf("project requirement %d priority %d out of range (must be 0-4)", req.ID, *req.Priority),
			})
		}
	}
	return errs
}

// TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// checkModuleRefs validates cross-references within a single module.
func checkModuleRefs(modName string, mod *schema.ModuleSpec, project *schema.Project) []ValidationError {
	var errs []ValidationError

	reqSet := stringIDSet(moduleReqIDs(mod.Requirements))
	compSet := stringIDSet(compIDs(mod.Components))
	// TODO(bead:spexmachina-qg2): projReqSet needs string keys once project IDs are also identity hashes
	projReqSet := stringIDSet(projReqStringIDs(project.Requirements))
	prefix := modName + "/module.json:"

	// component.implements → requirement IDs within the same module.
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
		// component.uses → component IDs within the same module.
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

	// impl_section.describes → component IDs within the same module.
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

	// data_flow.uses → component IDs within the same module.
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

	// test_section.describes → component IDs within the same module.
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

	// requirement.depends_on → requirement IDs within the same module.
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
		// requirement.preq_id — mandatory, must reference a valid project requirement.
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

// ID extraction helpers.

func reqIDs(reqs []schema.Requirement) []int {
	ids := make([]int, len(reqs))
	for i, r := range reqs {
		ids[i] = r.ID
	}
	return ids
}

func moduleIDs(mods []schema.Module) []int {
	ids := make([]int, len(mods))
	for i, m := range mods {
		ids[i] = m.ID
	}
	return ids
}

func milestoneIDs(mss []schema.Milestone) []int {
	ids := make([]int, len(mss))
	for i, ms := range mss {
		ids[i] = ms.ID
	}
	return ids
}

// TODO(bead:spexmachina-qg2): remove once project IDs are also identity hashes
func projReqStringIDs(reqs []schema.Requirement) []string {
	ids := make([]string, len(reqs))
	for i, r := range reqs {
		ids[i] = fmt.Sprintf("%d", r.ID)
	}
	return ids
}

// TODO(bead:spexmachina-qg2): fix after spexmachina-e8t changed module IDs from int to identity hash strings
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

// idSet converts a slice of int IDs to a set for O(1) lookup.
func idSet(ids []int) map[int]bool {
	s := make(map[int]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// stringIDSet converts a slice of string IDs to a set for O(1) lookup.
func stringIDSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// checkDuplicateStringIDs reports any string IDs that appear more than once.
func checkDuplicateStringIDs(path string, ids []string) []ValidationError {
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
