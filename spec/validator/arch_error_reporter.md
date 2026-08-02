# ErrorReporter

Aggregates the validation entries a run produced and writes them out as one structured JSON report.

## Responsibilities

- Take the run's validation entries as a single list, in the order the command's checks appended them; this component runs no check of its own and reaches no file in the spec
- Order the aggregated entries by path
- Format as a structured JSON report for machine consumption
- Write the JSON report to the writer the caller supplies; `spex validate` always supplies stdout. Whether that writer is a terminal changes the indentation only — two spaces per nesting level for a terminal, compact otherwise — never the destination: nothing is written to stderr

## Interface

Takes the aggregated list of validation entries, a writer, and a flag stating whether that
writer is a terminal. It writes the JSON report to the writer and returns the report it
wrote, so a caller derives its exit status from the same value it serialized instead of
re-deriving validity from the entry list — the two cannot drift apart. Composing and writing
that report is this component's share of [[608f8ca2e1b0|structured error output]]. A
serialization failure is returned to the caller alongside the report.

Every validation entry carries:

- `check` — which checker produced it
- `severity` — always `"error"`; no checker can emit any other value
- `path` — location in the spec, e.g. `project.json:/modules/0/name`
- `message` — human-readable description
- `schema_path` — the JSON Schema path that was violated, set only by the two checks that judge a document against a JSON Schema: the schema check, and the coupled-section check on a content violation. Entries from those two checks that report a file they could not read or parse carry no `schema_path` either

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

The whole report is one JSON document.

`error_count` is the number of entries in `errors`, with no entry discounted, because an
entry is an error and nothing else. `warning_count` is a stable part of the JSON contract
and is always `0` — there are no warning-producing checks. It stays in the output because
gates and CI assert on `warning_count == 0`.

## Exit Code

- 0: `valid` is true — the report carries no entries
- 1: `valid` is false — one or more entries were reported
