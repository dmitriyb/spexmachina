package merkle

import "strconv"

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
	Module string // module name; empty for project-level changes
}

// Classify assigns an impact level and owning module to each change based on
// node metadata (NodeType, Module) carried by each Change from the DiffEngine.
// The moduleNames map resolves module IDs to human-readable names. If nil,
// the module ID string is used as-is.
func Classify(changes []Change, moduleNames map[int]string) []ClassifiedChange {
	result := make([]ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = ClassifiedChange{
			Change: c,
			Impact: classifyNodeType(c.NodeType),
			Module: resolveModule(c.Module, moduleNames),
		}
	}
	return result
}

// classifyNodeType determines the impact level from node metadata.
func classifyNodeType(nodeType string) ImpactLevel {
	switch nodeType {
	case "impl_section", "data_flow", "test_section":
		return ImplOnly
	case "component":
		return ArchImpl
	case "meta":
		return Structural
	default:
		return 0
	}
}

// resolveModule maps a module ID to a name. Returns "" for project-level nodes
// (module ID 0).
func resolveModule(moduleID int, moduleNames map[int]string) string {
	if moduleID == 0 {
		return ""
	}
	if moduleNames != nil {
		if name, ok := moduleNames[moduleID]; ok {
			return name
		}
	}
	return strconv.Itoa(moduleID)
}
