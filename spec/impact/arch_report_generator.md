# ReportGenerator

[[cc987600950e|The impact report]] is written here: the actions [[76d72cbe00f3|ActionClassifier]] decided are sorted into two groups, counted, and emitted as one JSON document. Only two action types exist to group — create and obsolete.

## Responsibilities

- Format classified actions as a JSON report
- Include summary statistics (counts by action type)
- Write to stdout for piping to `spex emit`

## Interface

One call, over the classified actions and the stream to write them to. It splits the actions into the two groups, counts each group into the summary, writes the document, and reports a failure by returning it rather than printing it.

## Output Format

```json
{
  "creates": [
    {
      "type": "create",
      "module": "validator",
      "node": "ContentResolver",
      "node_type": "component",
      "spec_hash": "abc123",
      "old_bead_id": "spexmachina-77",
      "dep_spec_node_ids": ["a1b2c3d4e5f6", "0011223344aa"],
      "reason": "Spec node modified (new): validator/ContentResolver"
    }
  ],
  "obsoletes": [...],
  "summary": {
    "create_count": 5,
    "obsolete_count": 3
  }
}
```

The document is indented two spaces, so that a diff of one report against another is readable by the person reviewing it.

The `dep_spec_node_ids` field on a create action carries identity hashes — the values a spec author wrote in each depended-on node's `id` field — and never bead ids. Nothing here consults the mapping store, so no dependency has been resolved to a bead by the time the report is written; that is deliberately left to emit. The field is omitted when the create collected no spec-graph dependencies. Downstream, `spex emit` turns each hash into one of the changeset's dep refs, which the adapter turns into `--deps` flags on the tracker CLI.

When nothing changed, both arrays come out empty and both counts zero. That is a valid report rather than an error condition, and `spex emit` treats it as a no-op — the changeset it produces is empty, or carries the proposal epic and nothing else.

## Composability

The report is written to stdout as JSON, enabling piping:
```
spex diff | spex impact | spex emit
```
