package mapping

// SpecGraph provides read access to the spec dependency structure.
type SpecGraph interface {
	ModuleByName(name string) (ModuleInfo, error)
	ModuleByID(id string) (ModuleInfo, error)
}

// ModuleInfo describes a module's identity and dependencies.
// IDs are 12-character hex identity hash strings.
type ModuleInfo struct {
	ID             string
	Name           string
	RequiresModule []string
	Components     []ComponentInfo
	DataFlows      []DataFlowInfo
	TestSections   []TestSectionInfo
}

// ComponentInfo describes a component within a module.
type ComponentInfo struct {
	ID   string
	Name string
	Uses []string
}

// DataFlowInfo describes a data flow within a module. Uses lists the
// identity hashes of the components participating in the flow.
type DataFlowInfo struct {
	ID   string
	Name string
	Uses []string
}

// TestSectionInfo describes a test section within a module. Describes
// lists the identity hashes of the components the test section covers.
type TestSectionInfo struct {
	ID        string
	Name      string
	Describes []string
}
