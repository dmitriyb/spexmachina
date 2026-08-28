package merkle

import "github.com/dmitriyb/spexmachina/schema"

// ImpactLevel represents the severity of a spec change. Ordering (lowest to
// highest) is unknown < impl_only < contract < arch_impl < structural — the
// int value reflects this so a numeric max yields the aggregate module
// impact.
type ImpactLevel int

const (
	Unknown ImpactLevel = iota
	ImplOnly
	Contract
	ArchImpl
	Structural
)

func (il ImpactLevel) String() string {
	switch il {
	case ImplOnly:
		return "impl_only"
	case Contract:
		return "contract"
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

// Classify assigns an impact level and owning module name to each change,
// one classification per change, in the order handed in. The level comes
// from node metadata (NodeType) each Change already carries from the
// DiffEngine — never a path — read against profile's type-to-level mapping,
// joined by the one fixed rule: a "meta" node type is always Structural,
// regardless of what the profile declares. A node type that is neither
// "meta" nor declared by the profile classifies Unknown. moduleNames
// resolves a module identity hash to its name; a hash the map does not
// cover, or a nil map, passes through as the hash itself, and a
// project-level change (empty Module) stays empty. Classify has no failure
// mode: resolving profile is the caller's job, done before this call.
func Classify(changes []Change, moduleNames map[string]string, profile *schema.Profile) []ClassifiedChange {
	result := make([]ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = ClassifiedChange{
			Change: c,
			Impact: classifyNodeType(c.NodeType, profile),
			Module: resolveModule(c.Module, moduleNames),
		}
	}
	return result
}

// classifyNodeType determines the impact level for a node type: the fixed
// meta rule first, then the resolved profile's declared mapping, and
// Unknown for a type neither covers.
func classifyNodeType(nodeType string, profile *schema.Profile) ImpactLevel {
	if nodeType == "meta" {
		return Structural
	}
	levelName, ok := profile.ImpactLevels[nodeType]
	if !ok {
		return Unknown
	}
	return parseImpactLevel(levelName)
}

// parseImpactLevel converts a profile-declared impact_levels value into its
// ImpactLevel constant. Unknown covers a malformed declaration the same way
// it covers an undeclared node type — Classify itself never fails.
func parseImpactLevel(name string) ImpactLevel {
	switch name {
	case "impl_only":
		return ImplOnly
	case "contract":
		return Contract
	case "arch_impl":
		return ArchImpl
	case "structural":
		return Structural
	default:
		return Unknown
	}
}

// resolveModule maps a module ID to a name. Returns "" for project-level nodes
// (module ID 0).
func resolveModule(moduleHash string, moduleNames map[string]string) string {
	if moduleHash == "" {
		return ""
	}
	if moduleNames != nil {
		if name, ok := moduleNames[moduleHash]; ok {
			return name
		}
	}
	return moduleHash
}
