package proposal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BeadRecord represents a bead from the bead CLI JSON output.
type BeadRecord struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Metadata map[string]string `json:"metadata"`
}

// BeadListOutput is the top-level JSON output from `br list --json`.
type BeadListOutput struct {
	Issues []BeadRecord `json:"issues"`
}

// ProposalEntry represents a single proposal in the history output.
type ProposalEntry struct {
	Proposal string      `json:"proposal"`
	Type     string      `json:"type"`
	Date     string      `json:"date"`
	Beads    []BeadEntry `json:"beads"`
}

// BeadEntry represents a bead linked to a proposal.
type BeadEntry struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Module    string `json:"module"`
	Component string `json:"component"`
}

// BeadLister is the interface for listing beads. It allows testing without
// requiring the real br CLI.
type BeadLister interface {
	ListBeads(ctx context.Context) ([]BeadRecord, error)
}

// CLIBeadLister lists beads by executing a bead CLI binary.
type CLIBeadLister struct {
	Bin string // e.g. "br"
}

// ListBeads runs `<bin> list --json` and parses the output.
func (c *CLIBeadLister) ListBeads(ctx context.Context) ([]BeadRecord, error) {
	out, err := execCommand(ctx, c.Bin, "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("proposal: bead CLI unavailable: %w", err)
	}

	var result BeadListOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("proposal: parse bead list: %w", err)
	}
	return result.Issues, nil
}

// ShowHistory lists proposals and their linked beads, writing to w.
// If jsonMode is true, output is JSON. Otherwise human-readable.
func ShowHistory(ctx context.Context, specDir string, lister BeadLister, w io.Writer, jsonMode bool) error {
	proposalsDir := filepath.Join(specDir, "proposals")
	entries, err := os.ReadDir(proposalsDir)
	if err != nil {
		if os.IsNotExist(err) {
			if jsonMode {
				_, err := io.WriteString(w, "[]\n")
				return err
			}
			return nil
		}
		return fmt.Errorf("proposal: read proposals dir: %w", err)
	}

	// Collect proposal filenames.
	var proposals []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			proposals = append(proposals, e.Name())
		}
	}
	sort.Strings(proposals)

	if len(proposals) == 0 {
		if jsonMode {
			_, err := io.WriteString(w, "[]\n")
			return err
		}
		return nil
	}

	// Query beads.
	beads, err := lister.ListBeads(ctx)
	if err != nil {
		return err
	}

	// Build proposal → beads index.
	beadsByProposal := make(map[string][]BeadRecord)
	for _, b := range beads {
		if ref, ok := b.Metadata["spec_proposal"]; ok {
			// spec_proposal stores the stem (no .md), so match both forms.
			key := ref
			if !strings.HasSuffix(key, ".md") {
				key = key + ".md"
			}
			beadsByProposal[key] = append(beadsByProposal[key], b)
		}
	}

	// Build entries.
	result := make([]ProposalEntry, 0, len(proposals))
	for _, p := range proposals {
		ptype := detectProposalType(filepath.Join(proposalsDir, p))
		date := extractDate(p)

		var beadEntries []BeadEntry
		if matched, ok := beadsByProposal[p]; ok {
			for _, b := range matched {
				module, component := parseBeadTitle(b.Title)
				action := b.Metadata["action"]
				if action == "" {
					action = "created"
				}
				beadEntries = append(beadEntries, BeadEntry{
					ID:        b.ID,
					Action:    action,
					Module:    module,
					Component: component,
				})
			}
		}
		if beadEntries == nil {
			beadEntries = []BeadEntry{}
		}

		result = append(result, ProposalEntry{
			Proposal: p,
			Type:     ptype,
			Date:     date,
			Beads:    beadEntries,
		})
	}

	if jsonMode {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Human-readable output.
	for _, entry := range result {
		fmt.Fprintf(w, "%s (%s proposal)\n", entry.Proposal, entry.Type)
		for _, b := range entry.Beads {
			label := strings.ToUpper(b.Action[:1]) + b.Action[1:]
			fmt.Fprintf(w, "  %s: %s (%s: %s)\n", label, b.ID, b.Module, b.Component)
		}
	}
	return nil
}

// detectProposalType reads a proposal file and detects its type.
// Returns "project", "change", or "unknown".
func detectProposalType(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	ptype, err := detectType(string(content))
	if err != nil {
		return "unknown"
	}
	return ptype
}

// extractDate extracts the YYYY-MM-DD date prefix from a proposal filename.
func extractDate(filename string) string {
	// Expected format: YYYY-MM-DD-slug.md
	if len(filename) >= 10 {
		return filename[:10]
	}
	return ""
}

// parseBeadTitle parses "Module: Component" from a bead title.
// Falls back to empty strings if the format doesn't match.
func parseBeadTitle(title string) (module, component string) {
	if idx := strings.Index(title, ": "); idx >= 0 {
		return title[:idx], title[idx+2:]
	}
	return "", title
}
