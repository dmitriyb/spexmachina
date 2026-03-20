# ReportGenerator

Produces the structured JSON impact report from classified actions. Reports two action types: create and obsolete.

## Responsibilities

- Format classified actions as a JSON report
- Include summary statistics (counts by action type)
- Write to stdout for piping to `spex apply`

## Interface

```go
type ImpactReport struct {
    Creates   []Action `json:"creates"`
    Obsoletes []Action `json:"obsoletes"`
    Summary   Summary  `json:"summary"`
}

type Summary struct {
    CreateCount   int `json:"create_count"`
    ObsoleteCount int `json:"obsolete_count"`
}

func GenerateReport(actions []Action, w io.Writer) error
```

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
      "dep_bead_ids": ["spex-050", "spex-060"],
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

The `dep_bead_ids` field on create actions carries resolved spec-graph dependency bead IDs. It may be empty or omitted when no spec-graph dependencies exist. Downstream, `spex apply` reads this field to pass `--deps depends:<bead-id>` flags to the bead CLI.

## Composability

The report is written to stdout as JSON, enabling piping:
```
spex diff | spex impact | spex apply
```
