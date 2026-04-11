package mapping

// SpecGraph provides read access to the spec dependency structure.
type SpecGraph interface {
	ModuleByName(name string) (ModuleInfo, error)
	ModuleByID(id int) (ModuleInfo, error)
	NodeHash(specNodeID string) (string, error)
}

// ModuleInfo describes a module's identity and dependencies.
type ModuleInfo struct {
	ID             int
	Name           string
	RequiresModule []int
	Components     []ComponentInfo
}

// ComponentInfo describes a component within a module.
type ComponentInfo struct {
	ID   string
	Name string
	Uses []string
}
