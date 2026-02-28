// Package schema defines JSON Schema documents and Go types for Spex Machina
// spec files (project.json and module.json).
//
// The JSON Schema files are embedded and accessible via [ProjectSchema] and
// [ModuleSchema]. The Go types mirror the schema structure for unmarshaling.
//
// Node types: requirement, component, impl_section, data_flow, milestone, module.
// Edge types: implements, uses, described_in, depends_on, groups, requires_module.
package schema

import "embed"

//go:embed project.schema.json module.schema.json
var schemaFS embed.FS

// ProjectSchema returns the raw JSON Schema bytes for project.json.
func ProjectSchema() ([]byte, error) {
	return schemaFS.ReadFile("project.schema.json")
}

// ModuleSchema returns the raw JSON Schema bytes for module.json.
func ModuleSchema() ([]byte, error) {
	return schemaFS.ReadFile("module.schema.json")
}

// Project represents a project.json file.
type Project struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Version      string        `json:"version,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Modules      []Module      `json:"modules"`
	Milestones   []Milestone   `json:"milestones,omitempty"`
}

// Module represents a module declaration in project.json.
type Module struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Description    string `json:"description,omitempty"`
	RequiresModule []int  `json:"requires_module,omitempty"`
}

// Requirement represents a requirement node (used in both project.json and module.json).
type Requirement struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	DependsOn   []int  `json:"depends_on,omitempty"`
}

// Milestone represents a milestone in project.json.
type Milestone struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Groups      []int  `json:"groups,omitempty"`
}

// ModuleSpec represents a module.json file.
type ModuleSpec struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Components   []Component   `json:"components,omitempty"`
	ImplSections []ImplSection `json:"impl_sections,omitempty"`
	DataFlows    []DataFlow    `json:"data_flows,omitempty"`
}

// Component represents an architecture component in a module.
type Component struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
	Implements  []int  `json:"implements,omitempty"`
	Uses        []int  `json:"uses,omitempty"`
}

// ImplSection represents an implementation section in a module.
type ImplSection struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Content     string `json:"content,omitempty"`
	DescribedIn []int  `json:"described_in,omitempty"`
}

// DataFlow represents a data flow in a module.
type DataFlow struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
	Uses        []int  `json:"uses,omitempty"`
}
