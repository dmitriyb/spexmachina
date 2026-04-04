# Impact command implementation

## Structure

`cmd/spex/impact.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, read diff JSON from stdin or file
2. Parse the diff JSON — extract both `changes` array and `errors` array
3. If `errors` is non-empty: print each error to stderr, exit 1
4. Call `NodeMatcher.Match(changes, records)` to correlate changes with bead-map records
5. Call `ActionClassifier.Classify(matches)` to determine actions
6. Call `ReportGenerator.Generate(actions)` to produce JSON report
7. Output report to stdout

## Error rejection

The diff JSON format includes an optional `errors` array:

```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "...",
      "path": "module/4/meta",
      "related": ["module/4/component/2"]
    }
  ]
}
```

When `errors` is present and non-empty, impact refuses to proceed. This prevents bead creation from an incomplete spec edit. The user must fix the spec (update the missing content leaves) and re-run `spex diff` to get a clean diff before impact will accept it.
