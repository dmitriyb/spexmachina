package validator

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// ValidationReport is the structured output of a validation run.
//
// WarningCount is a stable part of the JSON contract and is always 0: no
// checker emits a severity other than "error". It is kept — and kept emitted —
// because downstream gates and CI assert on `.warning_count == 0`.
type ValidationReport struct {
	Valid        bool              `json:"valid"`
	ErrorCount   int               `json:"error_count"`
	WarningCount int               `json:"warning_count"`
	Errors       []ValidationError `json:"errors"`
}

// Report aggregates validation errors, sorts them by path, and writes a JSON
// ValidationReport to w. It returns the report it wrote, so that a caller's exit
// status derives from the same value it serialized instead of re-deriving
// validity from the error slice — the two cannot drift apart.
// When isTTY is true, output is pretty-printed with indentation.
func Report(errors []ValidationError, w io.Writer, isTTY bool) (ValidationReport, error) {
	sorted := make([]ValidationError, len(errors))
	copy(sorted, errors)
	slices.SortFunc(sorted, func(a, b ValidationError) int {
		return cmp.Compare(a.Path, b.Path)
	})

	report := ValidationReport{
		Valid:        len(sorted) == 0,
		ErrorCount:   len(sorted),
		WarningCount: 0,
		Errors:       sorted,
	}

	enc := json.NewEncoder(w)
	if isTTY {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(&report); err != nil {
		return report, fmt.Errorf("validator: encode report: %w", err)
	}
	return report, nil
}
