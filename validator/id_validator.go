package validator

import (
	"fmt"
	"slices"

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
	errs = append(errs, checkDuplicateIDs("project.json:/milestones", milestoneIDs(project.Milestones))...)
	if project.TestPlan != nil {
		errs = append(errs, checkDuplicateIDs("project.json:/test_plan/scenarios", scenarioIDs(project.TestPlan.Scenarios))...)
	}
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

	return errs
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

	for _, ms := range project.Milestones {
		for _, groupID := range ms.Groups {
			if !modIDSet[groupID] {
				errs = append(errs, ValidationError{
					Check:    "id",
					Severity: "error",
					Path:     fmt.Sprintf("project.json:/milestones/%s", ms.ID),
					Message:  fmt.Sprintf("groups references non-existent module %s", groupID),
				})
			}
		}
	}

	if project.TestPlan != nil {
		for _, sc := range project.TestPlan.Scenarios {
			for _, modID := range sc.Modules {
				if !modIDSet[modID] {
					errs = append(errs, ValidationError{
						Check:    "id",
						Severity: "error",
						Path:     fmt.Sprintf("project.json:/test_plan/scenarios/%s", sc.ID),
						Message:  fmt.Sprintf("modules references non-existent module %s", modID),
					})
				}
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

func milestoneIDs(mss []schema.Milestone) []string {
	ids := make([]string, len(mss))
	for i, ms := range mss {
		ids[i] = ms.ID
	}
	return ids
}

func scenarioIDs(scs []schema.TestScenario) []string {
	ids := make([]string, len(scs))
	for i, s := range scs {
		ids[i] = s.ID
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
