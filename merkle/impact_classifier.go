package merkle

import (
	"strconv"
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
	Module string // module name; empty for project-level changes
}

// Classify assigns an impact level and owning module to each change based on
// its spec-ID key. Keys follow the pattern: module/<id>/<node_type>/<node_id>.
// The moduleNames map resolves module IDs to human-readable names. If nil,
// the module ID string is used as-is.
func Classify(changes []Change, moduleNames map[int]string) []ClassifiedChange {
	result := make([]ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = ClassifiedChange{
			Change: c,
			Impact: classifyKey(c.Path),
			Module: resolveModuleName(c.Path, moduleNames),
		}
	}
	return result
}

// classifyKey determines the impact level from a spec-ID key.
func classifyKey(key string) ImpactLevel {
	// Meta nodes (project/meta, module/<id>/meta) are structural changes.
	if strings.HasSuffix(key, "/meta") {
		return Structural
	}

	// Parse key segments to determine node type.
	parts := strings.Split(key, "/")
	if len(parts) >= 3 {
		nodeType := parts[2]
		switch nodeType {
		case "component":
			return ArchImpl
		case "impl_section", "data_flow":
			return ImplOnly
		}
	}

	return 0
}

// resolveModuleName extracts the module ID from a spec-ID key and resolves it
// to a name using the provided map. Returns "" for project-level keys.
func resolveModuleName(key string, moduleNames map[int]string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 2 || parts[0] != "module" {
		return ""
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return parts[1]
	}
	if moduleNames != nil {
		if name, ok := moduleNames[id]; ok {
			return name
		}
	}
	return parts[1]
}
