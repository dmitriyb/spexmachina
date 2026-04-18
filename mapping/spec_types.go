package mapping

// SpecGraph provides read access to the spec dependency structure.
type SpecGraph interface {
	ModuleByName(name string) (ModuleInfo, error)
	ModuleByID(id string) (ModuleInfo, error)
	NodeHash(specNodeID string) (string, error)
}

// ModuleInfo describes a module's identity and dependencies.
// IDs are 12-character hex identity hash strings.
type ModuleInfo struct {
	ID             string
	Name           string
	RequiresModule []string
	Components     []ComponentInfo
}

// ComponentInfo describes a component within a module.
type ComponentInfo struct {
	ID   string
	Name string
	Uses []string
}
