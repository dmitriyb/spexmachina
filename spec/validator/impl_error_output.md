# Error Aggregation and Output Implementation

## Aggregation

Each checker returns a `[]ValidationError` slice. The main validation function concatenates all slices and sorts by path. Path is the only sort key: every entry has severity `"error"`, because no checker can produce any other severity.

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

`Report` returns the `ValidationReport` it serialized. The `spex validate` subcommand reads the exit code off that returned value, so the process status can never disagree with the `valid` field on stdout:
- 0 if `report.Valid`
- 1 otherwise

`ErrorCount` is the length of the aggregated slice, so every entry counts toward it. `WarningCount` is always 0.

## TTY Detection

When stdout is a terminal, pretty-print with indentation. When piped, output compact JSON. Use `golang.org/x/term.IsTerminal` for detection, consistent with the logging approach.
