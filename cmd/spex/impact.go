package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
)

func runImpact(args []string) int {
	fs := flag.NewFlagSet("impact", flag.ContinueOnError)
	diffFile := fs.String("diff", "", "path to diff JSON file (default: stdin)")
	beadCLI := fs.String("bead-cli", "br", "bead CLI binary name")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	// Read diff input.
	var diffData []byte
	var err error
	if *diffFile != "" {
		diffData, err = os.ReadFile(*diffFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex impact: read diff: %v\n", err)
			return 1
		}
	} else {
		diffData, err = readStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex impact: read stdin: %v\n", err)
			return 1
		}
	}

	var diffInput diffOutput
	if err := json.Unmarshal(diffData, &diffInput); err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: parse diff: %v\n", err)
		return 1
	}

	// Convert diffOutput changes to merkle.ClassifiedChange.
	classified := parseDiffChanges(diffInput.Changes)

	// Read bead metadata.
	ctx := context.Background()
	beads, err := impact.ReadBeads(ctx, *beadCLI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	// Run the impact pipeline.
	matched, unmatched := impact.MatchNodes(classified, beads)
	actions := impact.ClassifyActions(matched, unmatched)
	if err := impact.GenerateReport(actions, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}
	return 0
}

// readStdin reads all of stdin. Returns an error if stdin is a terminal
// (no piped input).
func readStdin() ([]byte, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat stdin: %w", err)
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("no input: pipe diff JSON or use --diff flag")
	}
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

// parseDiffChanges converts the diff command's JSON output into
// merkle.ClassifiedChange values for the impact pipeline.
func parseDiffChanges(changes []diffChange) []merkle.ClassifiedChange {
	result := make([]merkle.ClassifiedChange, len(changes))
	for i, c := range changes {
		result[i] = merkle.ClassifiedChange{
			Change: merkle.Change{
				Path:    c.Path,
				Type:    parseChangeType(c.Type),
				OldHash: c.OldHash,
				NewHash: c.NewHash,
			},
			Impact: parseImpactLevel(c.Impact),
			Module: c.Module,
		}
	}
	return result
}

func parseChangeType(s string) merkle.ChangeType {
	switch s {
	case "added":
		return merkle.Added
	case "removed":
		return merkle.Removed
	case "modified":
		return merkle.Modified
	default:
		return 0
	}
}

func parseImpactLevel(s string) merkle.ImpactLevel {
	switch s {
	case "impl_only":
		return merkle.ImplOnly
	case "arch_impl":
		return merkle.ArchImpl
	case "structural":
		return merkle.Structural
	default:
		return 0
	}
}
