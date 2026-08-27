package merkle

import "github.com/dmitriyb/spexmachina/schema"

// ImpactLevel represents the severity of a spec change. Ordering (lowest to
// highest) is impl_only < contract < arch_impl < structural — the int value
// reflects this so a numeric max yields the aggregate module impact.
type ImpactLevel int

const (
	ImplOnly ImpactLevel = iota + 1
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

// Classify assigns an impact level and owning module to each change based on
// node metadata (NodeType, Module) carried by each Change from the DiffEngine.
// The moduleNames map resolves module IDs to human-readable names. If nil,
// the module ID string is used as-is.
//
// The node-type-to-level mapping comes from the resolved profile's
// ImpactLevels declaration, passed as the optional profile argument; the
// default profile is used when it is omitted. This lets every existing call
// site keep calling Classify(changes, moduleNames) unchanged while a
// profile-aware caller can supply the resolved profile explicitly.
func Classify(changes []Change, moduleNames map[string]string, profile ...*schema.Profile) []ClassifiedChange {
	p := resolveProfileArg(profile)
	result := make([]ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = ClassifiedChange{
			Change: c,
			Impact: classifyNodeType(c.NodeType, p),
			Module: resolveModule(c.Module, moduleNames),
		}
	}
	return result
}

// resolveProfileArg extracts the optional profile argument, defaulting to
// the built-in profile when none was supplied.
func resolveProfileArg(profile []*schema.Profile) *schema.Profile {
	if len(profile) > 0 && profile[0] != nil {
		return profile[0]
	}
	return schema.DefaultProfile()
}

// classifyNodeType determines the impact level from node metadata. "meta" is
// the frame's fixed rule — always structural, regardless of profile — since
// module.json/project.json envelopes are not a profile-declarable node type.
// Every other node type is classified by the resolved profile's
// ImpactLevels mapping; a type the profile does not declare (or maps to a
// string outside the four fixed levels) gets no level at all, reported as
// "unknown".
func classifyNodeType(nodeType string, profile *schema.Profile) ImpactLevel {
	if nodeType == "meta" {
		return Structural
	}
	level, ok := profile.ImpactLevels[nodeType]
	if !ok {
		return 0
	}
	switch level {
	case "impl_only":
		return ImplOnly
	case "contract":
		return Contract
	case "arch_impl":
		return ArchImpl
	case "structural":
		return Structural
	default:
		return 0
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
