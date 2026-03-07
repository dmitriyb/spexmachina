package impact

import (
	"encoding/json"
	"fmt"
	"io"
)

// ImpactReport is the structured output of impact analysis, grouping
// classified actions by type with summary counts.
type ImpactReport struct {
	Creates []Action `json:"creates"`
	Closes  []Action `json:"closes"`
	Reviews []Action `json:"reviews"`
	Summary Summary  `json:"summary"`
}

// Summary holds counts of actions by type.
type Summary struct {
	CreateCount int `json:"create_count"`
	CloseCount  int `json:"close_count"`
	ReviewCount int `json:"review_count"`
}

// GenerateReport groups classified actions into an ImpactReport and writes
// it as 2-space-indented JSON to w.
func GenerateReport(actions []Action, w io.Writer) error {
	report := ImpactReport{
		Creates: make([]Action, 0),
		Closes:  make([]Action, 0),
		Reviews: make([]Action, 0),
	}

	for _, a := range actions {
		switch a.Type {
		case "create":
			report.Creates = append(report.Creates, a)
		case "close":
			report.Closes = append(report.Closes, a)
		case "review":
			report.Reviews = append(report.Reviews, a)
		}
	}

	report.Summary = Summary{
		CreateCount: len(report.Creates),
		CloseCount:  len(report.Closes),
		ReviewCount: len(report.Reviews),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&report); err != nil {
		return fmt.Errorf("impact: encode report: %w", err)
	}
	return nil
}
