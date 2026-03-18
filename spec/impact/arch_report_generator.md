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
  "creates": [...],
  "obsoletes": [...],
  "summary": {
    "create_count": 5,
    "obsolete_count": 3
  }
}
```

## Composability

The report is written to stdout as JSON, enabling piping:
```
spex diff | spex impact | spex apply
```
