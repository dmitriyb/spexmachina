package validator

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// ValidationReport is the structured output of a validation run.
type ValidationReport struct {
	Valid        bool              `json:"valid"`
	ErrorCount   int               `json:"error_count"`
	WarningCount int               `json:"warning_count"`
	Errors       []ValidationError `json:"errors"`
}

// Report aggregates validation errors, sorts them (errors before warnings,
// then by path), and writes a JSON ValidationReport to w.
// When isTTY is true, output is pretty-printed with indentation.
func Report(errors []ValidationError, w io.Writer, isTTY bool) error {
	sorted := make([]ValidationError, len(errors))
	copy(sorted, errors)
	slices.SortFunc(sorted, func(a, b ValidationError) int {
		// errors before warnings: "error" < "warning" lexicographically
		if c := cmp.Compare(a.Severity, b.Severity); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})

	var errCount, warnCount int
	for _, e := range sorted {
		switch e.Severity {
		case "error":
			errCount++
		case "warning":
			warnCount++
		}
	}

	report := ValidationReport{
		Valid:        errCount == 0,
		ErrorCount:   errCount,
		WarningCount: warnCount,
		Errors:       sorted,
	}

	enc := json.NewEncoder(w)
	if isTTY {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(&report); err != nil {
		return fmt.Errorf("validator: encode report: %w", err)
	}
	return nil
}
