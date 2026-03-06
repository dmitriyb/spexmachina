package merkle

import (
	"path"
	"strings"
)

// ImpactLevel represents the severity of a spec change.
type ImpactLevel int

const (
	ImplOnly   ImpactLevel = iota + 1
	ArchImpl
	Structural
)

func (il ImpactLevel) String() string {
	switch il {
	case ImplOnly:
		return "impl_only"
	case ArchImpl:
		return "arch_impl"
	case Structural:
		return "structural"
	default:
		return "unknown"
	}
}

// ClassifiedChange extends Change with impact classification metadata.
type ClassifiedChange struct {
	Change
	Impact ImpactLevel
	Module string // owning module name; empty for project-level changes
}

// Classify assigns an impact level and owning module to each change based on
// its path. Paths follow the merkle tree convention: project/Module/group/file.
func Classify(changes []Change) []ClassifiedChange {
	result := make([]ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = ClassifiedChange{
			Change: c,
			Impact: classifyPath(c.Path),
			Module: extractModule(c.Path),
		}
	}
	return result
}

// classifyPath determines the impact level from a leaf path.
func classifyPath(p string) ImpactLevel {
	name := path.Base(p)

	if name == "project.json" || name == "module.json" {
		return Structural
	}
	if strings.HasPrefix(name, "arch_") {
		return ArchImpl
	}
	if strings.HasPrefix(name, "impl_") || strings.HasPrefix(name, "flow_") {
		return ImplOnly
	}
	return 0
}

// extractModule returns the module name from a merkle tree path.
// Paths are structured as "project/Module/group/file" — the module is the
// second segment. Project-level leaves like "project/project.json" have no
// module (returns "").
func extractModule(path string) string {
	parts := strings.Split(path, "/")
	// project-level leaf: "project/project.json"
	if len(parts) <= 2 {
		return ""
	}
	return parts[1]
}
