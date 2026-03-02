# ReportGenerator

Produces the structured JSON impact report.

## Responsibilities

- Format classified actions as a JSON report
- Include summary statistics (counts by action type)
- Write to stdout for piping to `spex apply`

## Interface

```go
type ImpactReport struct {
    Creates []Action `json:"creates"`
    Closes  []Action `json:"closes"`
    Reviews []Action `json:"reviews"`
    Summary Summary  `json:"summary"`
}

type Summary struct {
    CreateCount int `json:"create_count"`
    CloseCount  int `json:"close_count"`
    ReviewCount int `json:"review_count"`
}

func GenerateReport(actions []Action, w io.Writer) error
```

## Output Format

```json
{
  "creates": [...],
  "closes": [...],
  "reviews": [...],
  "summary": {
    "create_count": 5,
    "close_count": 1,
    "review_count": 3
  }
}
```

## Composability

The report is written to stdout as JSON, enabling piping:
```
spex diff | spex impact | spex apply
```
