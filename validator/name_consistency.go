package validator

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckNameConsistency verifies that module names in project.json match the
// corresponding module.json name field exactly. Mismatches are reported with
// both values. Case-insensitive comparison detects likely matches and suggests
// fixes. Lowercase convention is enforced.
func CheckNameConsistency(specDir string) []ValidationError {
	return CheckNameConsistencyFS(os.DirFS(specDir))
}

// CheckNameConsistencyFS is CheckNameConsistency's in-memory-tree
// counterpart: it validates reading fsys rather than a directory on disk.
func CheckNameConsistencyFS(fsys fs.FS) []ValidationError {
	project, modules, errs := loadSpec(fsys, "name_consistency")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError
	for _, mod := range project.Modules {
		result = append(result, checkModuleName(mod, modules[mod.Name])...)
	}
	return result
}

// checkModuleName compares a single module's project.json name against its
// module.json name and enforces the lowercase convention.
func checkModuleName(mod schema.Module, modSpec *schema.ModuleSpec) []ValidationError {
	var errs []ValidationError
	projName := mod.Name
	modName := modSpec.Name

	if projName != modName {
		if strings.EqualFold(projName, modName) {
			errs = append(errs, ValidationError{
				Check:    "name_consistency",
				Severity: "error",
				Path:     mod.Path + "/module.json",
				Message:  fmt.Sprintf("name mismatch: project.json has %q, module.json has %q (case differs); change module.json name to %q", projName, modName, projName),
			})
		} else {
			errs = append(errs, ValidationError{
				Check:    "name_consistency",
				Severity: "error",
				Path:     mod.Path + "/module.json",
				Message:  fmt.Sprintf("name conflict: project.json has %q, module.json has %q", projName, modName),
			})
		}
	} else if projName != strings.ToLower(projName) {
		errs = append(errs, ValidationError{
			Check:    "name_consistency",
			Severity: "error",
			Path:     mod.Path + "/module.json",
			Message:  fmt.Sprintf("module name %q violates lowercase convention", projName),
		})
	}

	return errs
}
