package merkle

import (
	"fmt"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/schema"
)

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
// edges via implements arrays. All cross-references are compared as identity
// hashes (the 12-char hex strings used throughout the merkle tree). Returns
// nil if all changes are complete.
func CheckCompleteness(changes []ClassifiedChange, specDir string) []DiffError {
	if len(changes) == 0 {
		return nil
	}

	changedPaths := make(map[string]bool, len(changes))
	for _, c := range changes {
		changedPaths[c.Key] = true
	}

	var moduleReqChanges []ClassifiedChange
	var projectReqChanges []ClassifiedChange
	metaModules := make(map[string]bool)
	reqModules := make(map[string]bool)

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

	if len(moduleReqChanges) == 0 && len(projectReqChanges) == 0 && len(metaModules) == 0 {
		return nil
	}

	proj, err := readProject(specDir)
	if err != nil {
		return nil
	}

	moduleByHash := make(map[string]schema.Module, len(proj.Modules))
	for _, m := range proj.Modules {
		moduleByHash[m.ID] = m
	}

	// Index project requirements by id — the same value TreeBuilder uses as
	// the tree key for project-requirement leaves.
	projReqByID := make(map[string]schema.Requirement, len(proj.Requirements))
	for _, req := range proj.Requirements {
		projReqByID[req.ID] = req
	}

	moduleSpecCache := make(map[string]*schema.ModuleSpec)
	loadModule := func(moduleHash string) *schema.ModuleSpec {
		if spec, ok := moduleSpecCache[moduleHash]; ok {
			return spec
		}
		m, ok := moduleByHash[moduleHash]
		if !ok {
			return nil
		}
		spec, err := readModuleSpec(filepath.Join(specDir, m.Path, "module.json"))
		if err != nil {
			return nil
		}
		moduleSpecCache[moduleHash] = spec
		return spec
	}

	var errs []DiffError

	for _, c := range moduleReqChanges {
		modSpec := loadModule(c.Change.Module)
		if modSpec == nil {
			continue
		}
		reqHash := c.Key

		switch c.Type {
		case Modified:
			errs = append(errs, checkModifiedRequirement(reqHash, modSpec, changedPaths)...)
		case Added:
			errs = append(errs, checkAddedRequirement(reqHash, modSpec, changedPaths)...)
		case Removed:
			errs = append(errs, checkRemovedRequirement(reqHash, modSpec)...)
		}
	}

	for _, c := range projectReqChanges {
		// c.Key is the requirement's id. The lookup only supplies the title;
		// for a requirement removed from the current spec it simply misses.
		reqHash := c.Key
		var reqTitle string
		if req, ok := projReqByID[c.Key]; ok {
			reqTitle = req.Title
		}

		switch c.Type {
		case Modified, Added:
			errs = append(errs, checkProjectRequirementModifiedOrAdded(reqHash, reqTitle, proj, loadModule, changedPaths)...)
		case Removed:
			errs = append(errs, checkProjectRequirementRemoved(reqHash, reqTitle, proj, loadModule)...)
		}
	}

	for modHash := range metaModules {
		if reqModules[modHash] {
			continue
		}
		modSpec := loadModule(modHash)
		if modSpec == nil {
			continue
		}
		for _, comp := range modSpec.Components {
			if changedPaths[comp.ID] {
				continue
			}
			errs = append(errs, DiffError{
				Type:    "incomplete_change",
				Message: fmt.Sprintf("module %s meta changed but component %s content leaf unchanged", modSpec.Name, comp.Name),
				Path:    "meta/" + modHash,
				Related: []string{comp.ID},
			})
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// checkModifiedRequirement emits errors for components that implement the
// modified requirement but whose content leaf did not change.
func checkModifiedRequirement(reqHash string, modSpec *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	var errs []DiffError
	label := requirementLabel(findRequirementTitle(modSpec, reqHash), reqHash)
	for _, comp := range modSpec.Components {
		if !implementsReq(comp, reqHash) {
			continue
		}
		if changedPaths[comp.ID] {
			continue
		}
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("requirement %s (%s) description changed but component %s content leaf unchanged", label, modSpec.Name, comp.Name),
			Path:    reqHash,
			Related: []string{comp.ID},
		})
	}
	return errs
}

// checkAddedRequirement emits errors when an added requirement has no
// implementor, or when implementing components' content leaves did not change.
func checkAddedRequirement(reqHash string, modSpec *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	label := requirementLabel(findRequirementTitle(modSpec, reqHash), reqHash)
	implementors := findImplementors(modSpec, reqHash)
	if len(implementors) == 0 {
		return []DiffError{{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("requirement %s (%s) added but not implemented by any component", label, modSpec.Name),
			Path:    reqHash,
		}}
	}
	var errs []DiffError
	for _, comp := range implementors {
		if changedPaths[comp.ID] {
			continue
		}
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("requirement %s (%s) added but component %s content leaf unchanged", label, modSpec.Name, comp.Name),
			Path:    reqHash,
			Related: []string{comp.ID},
		})
	}
	return errs
}

// checkRemovedRequirement emits errors when a component still references the
// removed requirement via its implements array. The removed requirement is
// gone from modSpec.Requirements, so the title is unavailable — the hash
// carries the identity.
func checkRemovedRequirement(reqHash string, modSpec *schema.ModuleSpec) []DiffError {
	var errs []DiffError
	for _, comp := range modSpec.Components {
		if !implementsReq(comp, reqHash) {
			continue
		}
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("component %s still implements removed requirement %s (%s)", comp.Name, reqHash, modSpec.Name),
			Path:    reqHash,
			Related: []string{comp.ID},
		})
	}
	return errs
}

// checkProjectRequirementModifiedOrAdded walks the project → module → component
// chain and emits errors when derivation is missing or component content leaves
// did not change. projReqHash is the identity hash (req.ID) when the project
// requirement could be resolved from proj.Requirements; otherwise it falls
// back to the tree key.
func checkProjectRequirementModifiedOrAdded(projReqHash, projReqTitle string, proj *schema.Project, loadModule func(string) *schema.ModuleSpec, changedPaths map[string]bool) []DiffError {
	label := requirementLabel(projReqTitle, projReqHash)
	derived := findDerivedModuleRequirements(projReqHash, proj, loadModule)
	if len(derived) == 0 {
		return []DiffError{{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("no module requirement derives from project requirement %s", label),
			Path:    projReqHash,
		}}
	}
	var errs []DiffError
	for _, dr := range derived {
		for _, comp := range findImplementors(dr.modSpec, dr.reqHash) {
			if changedPaths[comp.ID] {
				continue
			}
			errs = append(errs, DiffError{
				Type:    "incomplete_change",
				Message: fmt.Sprintf("project requirement %s changed but component %s (%s) content leaf unchanged", label, comp.Name, dr.modSpec.Name),
				Path:    projReqHash,
				Related: []string{comp.ID},
			})
		}
	}
	return errs
}

// checkProjectRequirementRemoved emits errors for module requirements that
// still declare the removed project requirement as their preq_id.
func checkProjectRequirementRemoved(projReqHash, projReqTitle string, proj *schema.Project, loadModule func(string) *schema.ModuleSpec) []DiffError {
	label := requirementLabel(projReqTitle, projReqHash)
	var errs []DiffError
	for _, dr := range findDerivedModuleRequirements(projReqHash, proj, loadModule) {
		errs = append(errs, DiffError{
			Type:    "incomplete_change",
			Message: fmt.Sprintf("module requirement %s (%s) still derives from removed project requirement %s", dr.reqHash, dr.modSpec.Name, label),
			Path:    projReqHash,
			Related: []string{dr.reqHash},
		})
	}
	return errs
}

// requirementLabel renders a requirement identifier for an error message. If a
// title is available the label is "'<title>'"; otherwise the hash is shown raw
// so it still matches an identity hash in project.json / module.json.
func requirementLabel(title, hash string) string {
	if title != "" {
		return fmt.Sprintf("'%s'", title)
	}
	return hash
}

type derivedRequirement struct {
	moduleHash string
	reqHash    string
	modSpec    *schema.ModuleSpec
}

// findDerivedModuleRequirements returns all module requirements whose preq_id
// matches projReqHash (the project requirement's id, which is also its tree key).
func findDerivedModuleRequirements(projReqHash string, proj *schema.Project, loadModule func(string) *schema.ModuleSpec) []derivedRequirement {
	var result []derivedRequirement
	for _, m := range proj.Modules {
		mHash := m.ID
		modSpec := loadModule(mHash)
		if modSpec == nil {
			continue
		}
		for _, req := range modSpec.Requirements {
			if req.PreqID == projReqHash {
				result = append(result, derivedRequirement{
					moduleHash: mHash,
					reqHash:    req.ID,
					modSpec:    modSpec,
				})
			}
		}
	}
	return result
}

// findImplementors returns all components that declare reqHash in their
// implements array.
func findImplementors(modSpec *schema.ModuleSpec, reqHash string) []schema.Component {
	var result []schema.Component
	for _, comp := range modSpec.Components {
		if implementsReq(comp, reqHash) {
			result = append(result, comp)
		}
	}
	return result
}

func implementsReq(comp schema.Component, reqHash string) bool {
	for _, id := range comp.Implements {
		if id == reqHash {
			return true
		}
	}
	return false
}

func findRequirementTitle(modSpec *schema.ModuleSpec, reqHash string) string {
	for _, r := range modSpec.Requirements {
		if r.ID == reqHash {
			return r.Title
		}
	}
	return reqHash
}
