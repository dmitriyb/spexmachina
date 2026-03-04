// Package validator validates spec directories against JSON Schemas and
// structural rules. Each checker produces []ValidationError; the caller
// aggregates results from all checkers.
package validator

// ValidationError represents a single validation violation found by any checker.
type ValidationError struct {
	Check    string `json:"check"`    // which checker produced this: "schema", "content", "dag", "orphan", "id"
	Severity string `json:"severity"` // "error" or "warning"
	Path     string `json:"path"`     // location in the spec, e.g. "project.json:/modules/0/name"
	Message  string `json:"message"`  // human-readable description
}
