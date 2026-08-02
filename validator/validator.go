// Package validator validates spec directories against JSON Schemas and
// structural rules. Each checker produces []ValidationError; the caller
// aggregates results from all checkers.
package validator

// ValidationError represents a single validation violation found by any checker.
//
// Every checker emits violations at severity "error"; there are no warning
// sites left in this package. Severity is retained as a stable part of the
// `spex validate` JSON contract, not as a way to downgrade a violation.
type ValidationError struct {
	Check      string `json:"check"`                 // which checker produced this: "schema", "content", "link", "id", "id_derivation", "dag", "name_consistency", "test_coverage", "requirement_coverage", "coupled_section"
	Severity   string `json:"severity"`              // always "error"
	Path       string `json:"path"`                  // location in the spec, e.g. "project.json:/modules/0/name"
	Message    string `json:"message"`               // human-readable description
	SchemaPath string `json:"schema_path,omitempty"` // JSON Schema path that was violated
}
