# ErrorReporter

Aggregates validation errors from all checkers and produces structured JSON output.

## Responsibilities

- Collect errors from SchemaChecker, ContentResolver, DAGChecker, OrphanDetector, and IDValidator
- Assign severity levels (error, warning)
- Format as structured JSON array for machine consumption
- Write to stdout (for piping) or stderr (for human reading)

## Interface

```go
type ValidationError struct {
    Check    string `json:"check"`     // which checker produced this
    Severity string `json:"severity"`  // "error" or "warning"
    Path     string `json:"path"`      // location in the spec (e.g., "validator/module.json:components[1].implements")
    Message  string `json:"message"`   // human-readable description
}

func Report(errors []ValidationError, w io.Writer) error
```

## Output Format

```json
{
  "valid": false,
  "error_count": 3,
  "warning_count": 1,
  "errors": [
    {
      "check": "schema",
      "severity": "error",
      "path": "project.json:modules[0].name",
      "message": "required field missing"
    }
  ]
}
```

## Exit Code

- 0: no errors (warnings allowed)
- 1: one or more errors found
