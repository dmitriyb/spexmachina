package impact

import (
	"encoding/json"
	"fmt"
	"io"
)

// ImpactReport is the structured output of impact analysis, grouping
// classified actions by type with summary counts.
type ImpactReport struct {
	Creates   []Action `json:"creates"`
	Obsoletes []Action `json:"obsoletes"`
	Summary   Summary  `json:"summary"`
}

// Summary holds counts of actions by type.
type Summary struct {
	CreateCount   int `json:"create_count"`
	ObsoleteCount int `json:"obsolete_count"`
}

// GenerateReport groups classified actions into an ImpactReport and writes
// it as 2-space-indented JSON to w.
func GenerateReport(actions []Action, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("impact: nil writer")
	}

	report := ImpactReport{
		Creates:   make([]Action, 0),
		Obsoletes: make([]Action, 0),
	}

	for _, a := range actions {
		switch a.Type {
		case "create":
			report.Creates = append(report.Creates, a)
		case "obsolete":
			report.Obsoletes = append(report.Obsoletes, a)
		}
	}

	report.Summary = Summary{
		CreateCount:   len(report.Creates),
		ObsoleteCount: len(report.Obsoletes),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&report); err != nil {
		return fmt.Errorf("impact: encode report: %w", err)
	}
	return nil
}
