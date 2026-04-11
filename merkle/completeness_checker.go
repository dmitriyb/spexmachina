package merkle

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// TODO(bead:spexmachina-stp): review after spexmachina-kdb changed Module from int to identity hash string

// DiffError represents a completeness validation error in the diff output.
type DiffError struct {
	Type    string   `json:"type"`
	Message string   `json:"message"`
	Path    string   `json:"path"`
	Related []string `json:"related"`
}

// CheckCompleteness validates that requirement leaf changes in the diff are
// accompanied by corresponding component content leaf changes. It reads the
// current spec graph (project.json, module.json) to resolve requirement-to-component
// edges via implements arrays. Returns nil if all changes are complete.
func CheckCompleteness(changes []ClassifiedChange, specDir string) []DiffError {
	if len(changes) == 0 {
		return nil
	}

	// Build set of changed paths for O(1) lookup.
	changedPaths := make(map[string]bool, len(changes))
	for _, c := range changes {
		changedPaths[c.Path] = true
	}

	// Collect requirement leaf changes and meta changes by module.
	var moduleReqChanges []ClassifiedChange
	var projectReqChanges []ClassifiedChange
	// TODO(bead:spexmachina-stp): review after spexmachina-kdb changed Module from int to string
	metaModules := make(map[string]bool)    // modules with meta changes
	reqModules := make(map[string]bool)     // modules with requirement changes

	for _, c := range changes {
		switch {
		case c.NodeType == "requirement" && c.Change.Module != "":
			moduleReqChanges = append(moduleReqChanges, c)
			reqModules[c.Change.Module] = true
		case c.NodeType == "requirement" && c.Change.Module == "":
			projectReqChanges = append(projectReqChanges, c)
		case c.NodeType == "meta" && c.Change.Module != "":
			metaModules[c.Change.Module] = true
		}
	}

	// If there are no requirement changes, meta changes, or project requirement
	// changes, there is nothing to check.
	if len(moduleReqChanges) == 0 && len(projectReqChanges) == 0 && len(metaModules) == 0 {
		return nil
	}

	// Read spec graph.
	proj, err := readProject(specDir)
	if err != nil {
		return nil
	}

	// Build module identity hash → path map.
	modulePathMap := make(map[string]string, len(proj.Modules))
	for _, m := range proj.Modules {
		modulePathMap[schema.IdentityHash("module", m.Name)] = m.Path
	}

	// Cache loaded module specs.
	moduleSpecCache := make(map[string]*schema.ModuleSpec)
	loadModule := func(moduleHash string) *schema.ModuleSpec {
		if spec, ok := moduleSpecCache[moduleHash]; ok {
			return spec
		}
		modPath, ok := modulePathMap[moduleHash]
		if !ok {
			return nil
		}
		spec, err := readModuleSpec(filepath.Join(specDir, modPath, "module.json"))
		if err != nil {
			return nil
		}
		moduleSpecCache[moduleHash] = spec
		return spec
	}

	var errs []DiffError

	// TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	// Check module-level requirement changes.
	for _, c := range moduleReqChanges {
		reqID := parseLastSegment(c.Path)
		modSpec := loadModule(c.Change.Module)
		if modSpec == nil {
			continue
		}

		switch c.Type {
		case Modified:
			errs = append(errs, checkModifiedRequirement(c, reqID, modSpec, changedPaths)...)
		case Added:
			errs = append(errs, checkAddedRequirement(c, reqID, modSpec, changedPaths)...)
		case Removed:
			errs = append(errs, checkRemovedRequirement(c, reqID, modSpec)...)
		}
	}

	// Check project-level requirement changes.
	for _, c := range projectReqChanges {
		reqID := parseLastID(c.Path)

		switch c.Type {
		case Modified, Added:
			errs = append(errs, checkProjectRequirementModifiedOrAdded(c, reqID, proj, loadModule, changedPaths)...)
		case Removed:
			errs = append(errs, checkProjectRequirementRemoved(c, reqID, proj, loadModule)...)
		}
	}

	// Check meta-only changes: modules with meta changed but no requirement changes.
	for modID := range metaModules {
		if reqModules[modID] {
			continue // requirement changes exist in this module, skip meta-only check
		}
		modSpec := loadModule(modID)
		if modSpec == nil {
			continue
		}
		for _, comp := range modSpec.Components {
			compPath := fmt.Sprintf("%s", comp.ID)
			if !changedPaths[compPath] {
				errs = append(errs, DiffError{
					Type:    "incomplete_change",
					Message: fmt.Sprintf("module %s meta changed but component %s content leaf unchanged", modID, comp.Name),
					Path:    "meta/" + modID,
					Related: []string{compPath},
				})
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// checkModifiedRequirement checks that all components implementing the given
// requirement also have their content leaf in the diff.
func checkModifiedRequirement(c ClassifiedChange, reqID string, modSpec *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	var errs []DiffError
	reqTitle := findRequirementTitle(modSpec, reqID)
	for _, comp := range modSpec.Components {
		if !implementsReq(comp, reqID) {
			continue
		}
		compPath := comp.ID
		if !changedPaths[compPath] {
			errs = append(errs, DiffError{
				Type:    "incomplete_change",
				Message: fmt.Sprintf("requirement %s (%s) description changed but component %s content leaf unchanged", reqID, reqTitle, comp.Name),
				Path:    c.Path,
				Related: []string{compPath},
			})
		}
	}
	return errs
}

// checkAddedRequirement checks that the added requirement is implemented by at
// least one component and that those components' content leaves changed.
func checkAddedRequirement(c ClassifiedChange, reqID string, modSpec *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	var errs []DiffError
	implementors := findImplementors(modSpec, reqID)
	if len(implementors) == 0 {
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("requirement %s added but not implemented by any component", reqID),
			Path:    c.Path,
		})
		return errs
	}
	for _, comp := range implementors {
		compPath := comp.ID
		if !changedPaths[compPath] {
			errs = append(errs, DiffError{
				Type:    "incomplete_change",
				Message: fmt.Sprintf("requirement %s added but component %s content leaf unchanged", reqID, comp.Name),
				Path:    c.Path,
				Related: []string{compPath},
			})
		}
	}
	return errs
}

// checkRemovedRequirement checks that no component still references the removed requirement.
func checkRemovedRequirement(c ClassifiedChange, reqID string, modSpec *schema.ModuleSpec) []DiffError {
	var errs []DiffError
	for _, comp := range modSpec.Components {
		if implementsReq(comp, reqID) {
			errs = append(errs, DiffError{
				Type:    "incomplete_change",
				Message: fmt.Sprintf("component %s still implements removed requirement %s", comp.Name, reqID),
				Path:    c.Path,
				Related: []string{comp.ID},
			})
		}
	}
	return errs
}

// checkProjectRequirementModifiedOrAdded checks the project → module → component chain
// for modified or added project-level requirements.
func checkProjectRequirementModifiedOrAdded(c ClassifiedChange, reqID int, proj *schema.Project, loadModule func(string) *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	var errs []DiffError
	derivedReqs := findDerivedModuleRequirements(reqID, proj, loadModule)
	if len(derivedReqs) == 0 {
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("no module requirement derives from project requirement %d", reqID),
			Path:    c.Path,
		})
		return errs
	}
	for _, dr := range derivedReqs {
		for _, comp := range findImplementors(dr.modSpec, dr.reqID) {
			compPath := comp.ID
			if !changedPaths[compPath] {
				errs = append(errs, DiffError{
					Type:    "incomplete_change",
					Message: fmt.Sprintf("project requirement %d changed but component %s content leaf unchanged", reqID, comp.Name),
					Path:    c.Path,
					Related: []string{compPath},
				})
			}
		}
	}
	return errs
}

// checkProjectRequirementRemoved checks that no module requirement still derives
// from the removed project requirement.
func checkProjectRequirementRemoved(c ClassifiedChange, reqID int, proj *schema.Project, loadModule func(string) *schema.ModuleSpec) []DiffError {
	var errs []DiffError
	derivedReqs := findDerivedModuleRequirements(reqID, proj, loadModule)
	for _, dr := range derivedReqs {
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("module requirement %s still derives from removed project requirement %d", dr.reqID, reqID),
			Path:    c.Path,
			Related: []string{dr.reqID},
		})
	}
	return errs
}

// TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs from int to identity hash strings
type derivedRequirement struct {
	moduleHash string
	reqID      string
	modSpec    *schema.ModuleSpec
}

// findDerivedModuleRequirements finds all module requirements with preq_id == projectReqID.
func findDerivedModuleRequirements(projectReqID int, proj *schema.Project, loadModule func(string) *schema.ModuleSpec) []derivedRequirement {
	projReqStr := strconv.Itoa(projectReqID)
	var result []derivedRequirement
	for _, m := range proj.Modules {
		mHash := schema.IdentityHash("module", m.Name)
		modSpec := loadModule(mHash)
		if modSpec == nil {
			continue
		}
		for _, req := range modSpec.Requirements {
			if req.PreqID == projReqStr {
				result = append(result, derivedRequirement{
					moduleHash: mHash,
					reqID:      req.ID,
					modSpec:    modSpec,
				})
			}
		}
	}
	return result
}

// findImplementors returns all components that implement the given requirement.
func findImplementors(modSpec *schema.ModuleSpec, reqID string) []schema.Component {
	var result []schema.Component
	for _, comp := range modSpec.Components {
		if implementsReq(comp, reqID) {
			result = append(result, comp)
		}
	}
	return result
}

// implementsReq checks whether a component implements the given requirement ID.
func implementsReq(comp schema.Component, reqID string) bool {
	for _, id := range comp.Implements {
		if id == reqID {
			return true
		}
	}
	return false
}

// findRequirementTitle returns the title for a requirement ID, or the ID string as fallback.
func findRequirementTitle(modSpec *schema.ModuleSpec, reqID string) string {
	for _, r := range modSpec.Requirements {
		if r.ID == reqID {
			return r.Title
		}
	}
	return reqID
}

// parseLastID extracts the last numeric segment from a path like "module/1/requirement/2".
func parseLastID(path string) int {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0
	}
	id, _ := strconv.Atoi(parts[len(parts)-1])
	return id
}

// TODO(bead:spexmachina-stp): fix after spexmachina-e8t changed module IDs from int to identity hash strings
// parseLastSegment extracts the last path segment (identity hash or numeric string).
func parseLastSegment(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
