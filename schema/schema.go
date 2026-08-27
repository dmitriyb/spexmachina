// Package schema defines JSON Schema documents and Go types for Spex Machina
// spec files (project.json and module.json).
//
// The JSON Schema files are embedded and accessible via [ProjectSchema] and
// [ModuleSchema]. The Go types mirror the schema structure for unmarshaling.
//
// Node types: requirement, component, data_flow, api, module, test_section.
// Edge types: implements, uses, describes, described_in, provided_by, depends_on, requires_module.
package schema

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"strings"
)

//go:embed project.schema.json module.schema.json bead-map.schema.json
var schemaFS embed.FS

// identityHashPattern is the pattern every node id and cross-reference field
// carries: a 12-character lowercase hex string, matching what [IdentityHash]
// produces.
const identityHashPattern = "^[a-f0-9]{12}$"

// ProjectSchema returns the raw JSON Schema bytes for project.json.
func ProjectSchema() ([]byte, error) {
	return schemaFS.ReadFile("project.schema.json")
}

// ModuleSchema returns the raw JSON Schema bytes for module.json.
func ModuleSchema() ([]byte, error) {
	return schemaFS.ReadFile("module.schema.json")
}

// BeadMapSchema returns the raw JSON Schema bytes for one line of the
// task journal linking spec nodes to tasks, independent of where the
// journal file lives.
func BeadMapSchema() ([]byte, error) {
	return schemaFS.ReadFile("bead-map.schema.json")
}

// IdentityHash joins parts with "/" and returns the first 6 bytes of
// SHA256 as a 12-character lowercase hex string. The result is the
// canonical spec node ID for the given identity string.
func IdentityHash(parts ...string) string {
	identity := strings.Join(parts, "/")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:6])
}

// Project represents a project.json file.
type Project struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Version      string        `json:"version,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Modules      []Module      `json:"modules"`
	Sections     []Section     `json:"sections,omitempty"`
}

// Section represents a project-level section with a typed envelope and
// freeform content preserved as raw JSON. Renderers iterate sections
// generically and access freeform fields via Raw without knowing the
// coupled module's schema.
type Section struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

// UnmarshalJSON populates both the typed envelope fields and the full
// raw entry so renderers can access freeform content without losing data.
func (s *Section) UnmarshalJSON(data []byte) error {
	type envelope struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	s.ID = env.ID
	s.Name = env.Name
	s.Type = env.Type
	s.Raw = append(s.Raw[:0], data...)
	return nil
}

// Module represents a module declaration in project.json.
// IDs are 12-character hex identity hash strings.
type Module struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Path           string   `json:"path"`
	Description    string   `json:"description,omitempty"`
	RequiresModule []string `json:"requires_module,omitempty"`
}

// Requirement represents a requirement node (used in both project.json and module.json).
// PreqID is only used in module.json to trace derivation from project-level requirements.
// Priority is only used in project.json (integer 0-4, optional).
// Derivation is project-scoped only: "pending" declares a requirement not yet
// derived into any module. A module requirement derives by construction
// through its required preq_id, so it carries no such field.
// IDs are 12-character hex identity hash strings.
type Requirement struct {
	ID          string   `json:"id"`
	PreqID      string   `json:"preq_id,omitempty"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Derivation  string   `json:"derivation,omitempty"`
}

// ModuleSpec represents a module.json file.
// All IDs are 12-character hex identity hash strings.
type ModuleSpec struct {
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Requirements []ModuleRequirement `json:"requirements,omitempty"`
	Components   []Component         `json:"components,omitempty"`
	DataFlows    []DataFlow          `json:"data_flows,omitempty"`
	TestSections []TestSection       `json:"test_sections,omitempty"`
	APIs         []API               `json:"apis,omitempty"`
}

// ModuleRequirement represents a requirement in module.json.
// Unlike the project-level Requirement, all IDs are identity hash strings
// and preq_id is required.
type ModuleRequirement struct {
	ID          string   `json:"id"`
	PreqID      string   `json:"preq_id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// TestSection represents a test section in a module.
type TestSection struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Content   string   `json:"content,omitempty"`
	Describes []string `json:"describes,omitempty"`
}

// Component represents an architecture component in a module.
type Component struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Content     string   `json:"content,omitempty"`
	Implements  []string `json:"implements,omitempty"`
	Uses        []string `json:"uses,omitempty"`
}

// API represents an external surface entry point exposed by a module: a CLI
// subcommand, an HTTP route, or a library entry point. Name is the exact
// surface string as callers write it ("spex diff", "GET /v1/specs/{id}",
// "schema.IdentityHash") — never a signature. APIs have no content file; like
// project requirements they hash from their JSON fields alone. Group is
// freeform and spex never branches on it; it exists so renderers can group a
// project's surface. ProvidedBy is module-local.
type API struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ProvidedBy  []string `json:"provided_by,omitempty"`
	Group       string   `json:"group,omitempty"`
}

// DataFlow represents a data flow in a module.
type DataFlow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Content     string   `json:"content,omitempty"`
	Uses        []string `json:"uses,omitempty"`
}
