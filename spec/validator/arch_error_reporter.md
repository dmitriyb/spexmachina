# ErrorReporter

Aggregates validation errors from all checkers and produces structured JSON output.

## Responsibilities

- Collect errors from every checker: SchemaChecker, ContentResolver, IDValidator, DAGChecker, NameConsistencyChecker, TestCoverageChecker, RequirementCoverageChecker and CoupledSectionChecker
- Order the aggregated entries by path
- Format as a structured JSON report for machine consumption
- Write the JSON report to the writer the caller supplies; `spex validate` always supplies stdout. Whether that writer is a terminal changes the indentation only, never the destination — nothing is written to stderr

## Interface

Takes the aggregated list of validation entries, a writer, and a flag stating whether that
writer is a terminal. It writes the JSON report to the writer and returns the report it
wrote, so a caller derives its exit status from the same value it serialized instead of
re-deriving validity from the entry list — the two cannot drift apart. A serialization
failure is returned to the caller alongside the report.

Every validation entry carries:

- `check` — which checker produced it
- `severity` — always `"error"`; no checker can emit any other value
- `path` — location in the spec, e.g. `project.json:/modules/0/name`
- `message` — human-readable description
- `schema_path` — the JSON Schema path that was violated, present only on schema entries

## Output Format

```json
{
  "valid": false,
  "error_count": 3,
  "warning_count": 0,
  "errors": [
    {
      "check": "schema",
      "severity": "error",
      "path": "project.json:/modules/0/name",
      "message": "required field missing"
    }
  ]
}
```

`warning_count` is a stable part of the JSON contract and is always `0` — there are no
warning-producing checks. It stays in the output because gates and CI assert on
`warning_count == 0`.

## Exit Code

- 0: `valid` is true — the report carries no entries
- 1: `valid` is false — one or more entries were reported
