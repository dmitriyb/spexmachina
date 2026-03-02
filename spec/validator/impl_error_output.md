# Error Aggregation and Output Implementation

## Aggregation

Each checker returns a `[]ValidationError` slice. The main validation function concatenates all slices, sorts by severity (errors first, then warnings), then by path.

## JSON Serialization

```go
type ValidationReport struct {
    Valid        bool              `json:"valid"`
    ErrorCount   int               `json:"error_count"`
    WarningCount int               `json:"warning_count"`
    Errors       []ValidationError `json:"errors"`
}
```

Serialize with `json.NewEncoder(w).Encode(&report)`. Use 2-space indentation for human readability when writing to a terminal, compact JSON when piping.

## Exit Code

The `spex validate` subcommand sets the process exit code:
- 0 if `report.ErrorCount == 0`
- 1 if `report.ErrorCount > 0`

Warnings do not affect the exit code.

## TTY Detection

When stdout is a terminal, pretty-print with indentation. When piped, output compact JSON. Use `golang.org/x/term.IsTerminal` for detection, consistent with the logging approach.
