package proposal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// specProposalLabelPrefix is the bead-label prefix that links a bead to a
// proposal stem (filename without .md). The first such label on a bead
// determines the bead's proposal grouping; additional matching labels are
// ignored.
const specProposalLabelPrefix = "spec_proposal:"

// missingProposalLabel is the human-readable suffix shown when a bead's
// spec_proposal label points at a file that does not exist under
// spec/proposals/. The bead is still surfaced so its provenance remains
// visible.
const missingProposalLabel = "proposal file missing"

// BeadRecord is the shape HistoryViewer consumes. The caller (ProposalCommands)
// parses tracker output (typically `br list --json`) into a slice of these
// records and hands it to the viewer; HistoryViewer never performs subprocess
// invocation of its own.
type BeadRecord struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Labels    []string `json:"labels"`
	Title     string   `json:"title"`
	CreatedAt string   `json:"created_at,omitempty"`
	ClosedAt  string   `json:"closed_at,omitempty"`
}

// HistoryViewer renders proposal-grouped bead history.
//
// SpecDir is the spec root (proposals are read from SpecDir/proposals).
// Out is the destination writer. JSON toggles between human-readable text
// (the default) and the JSON envelope documented in
// spec/proposal/arch_history_viewer.md.
type HistoryViewer struct {
	SpecDir string
	Out     io.Writer
	JSON    bool
}

// proposalEntry is the JSON shape per proposal group.
type proposalEntry struct {
	Filename string      `json:"filename"`
	Title    string      `json:"title"`
	Beads    []beadEntry `json:"beads"`
}

// beadEntry is the JSON shape for a single bead within a proposal group.
type beadEntry struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
}

// ShowHistory groups beads by their first spec_proposal:<stem> label, resolves
// each group's proposal file under SpecDir/proposals, and writes the rendered
// history to Out. Beads without a spec_proposal label are silently skipped.
// Groups whose proposal files are missing are still rendered, with a "proposal
// file missing" annotation, so the bead's provenance stays visible.
func (h *HistoryViewer) ShowHistory(beads []BeadRecord) error {
	groups := groupBeadsByProposal(beads)

	stems := make([]string, 0, len(groups))
	for stem := range groups {
		stems = append(stems, stem)
	}
	sort.Strings(stems)

	entries := make([]proposalEntry, 0, len(stems))
	humanMeta := make([]proposalDisplay, 0, len(stems))

	proposalsDir := filepath.Join(h.SpecDir, "proposals")
	for _, stem := range stems {
		filename := stem + ".md"
		path := filepath.Join(proposalsDir, filename)
		title, ptype, exists := resolveProposalFile(path)

		entry := proposalEntry{
			Filename: filename,
			Title:    title,
			Beads:    make([]beadEntry, 0, len(groups[stem])),
		}
		for _, b := range groups[stem] {
			entry.Beads = append(entry.Beads, beadEntry{
				ID:      b.ID,
				Status:  b.Status,
				Action:  deriveAction(b),
				Summary: b.Title,
			})
		}
		entries = append(entries, entry)
		humanMeta = append(humanMeta, proposalDisplay{
			filename: filename,
			label:    proposalLabel(ptype, exists),
		})
	}

	if h.JSON {
		return h.writeJSON(entries)
	}
	return h.writeText(entries, humanMeta)
}

// proposalDisplay carries the per-group metadata only the human-readable
// renderer needs (the JSON envelope omits the type label entirely).
type proposalDisplay struct {
	filename string
	label    string
}

// groupBeadsByProposal walks beads and returns a map keyed by proposal stem
// (no .md). A bead with multiple spec_proposal labels is placed under the
// first one — defensive handling per arch_history_viewer.md.
func groupBeadsByProposal(beads []BeadRecord) map[string][]BeadRecord {
	groups := make(map[string][]BeadRecord)
	for _, b := range beads {
		stem, ok := firstProposalStem(b.Labels)
		if !ok {
			continue
		}
		groups[stem] = append(groups[stem], b)
	}
	return groups
}

// firstProposalStem returns the stem from the first spec_proposal:<stem>
// label, accepting either a bare stem or a value with a trailing .md.
func firstProposalStem(labels []string) (string, bool) {
	for _, lbl := range labels {
		if !strings.HasPrefix(lbl, specProposalLabelPrefix) {
			continue
		}
		ref := strings.TrimPrefix(lbl, specProposalLabelPrefix)
		ref = strings.TrimSuffix(ref, ".md")
		if ref == "" {
			continue
		}
		return ref, true
	}
	return "", false
}

// resolveProposalFile reads the proposal at path. It returns the H1 title, the
// detected proposal type (project|change|""), and whether the file exists.
// A missing file returns ("", "", false). A present-but-typeless file (no H1
// or unrecognized headings) returns ("", "", true) so the caller can render
// the filename without a title.
func resolveProposalFile(path string) (title, ptype string, exists bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	title = firstH1(string(content))
	if t, err := detectType(string(content)); err == nil {
		ptype = t
	}
	return title, ptype, true
}

// firstH1 returns the first markdown H1 heading text, with the leading "# "
// stripped. Empty string if none is found.
func firstH1(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// proposalLabel formats the parenthetical label for human-readable output.
func proposalLabel(ptype string, exists bool) string {
	if !exists {
		return missingProposalLabel
	}
	switch ptype {
	case "project":
		return "project proposal"
	case "change":
		return "change proposal"
	}
	return "proposal"
}

// deriveAction maps a bead's lifecycle to the human-facing action label.
// Closed beads are rendered as "closed" (the proposal closed them); all
// others are rendered as "created" (the proposal created and still owns them).
func deriveAction(b BeadRecord) string {
	if strings.EqualFold(b.Status, "closed") {
		return "closed"
	}
	return "created"
}

// writeJSON emits the {"proposals": [...]} envelope.
func (h *HistoryViewer) writeJSON(entries []proposalEntry) error {
	payload := struct {
		Proposals []proposalEntry `json:"proposals"`
	}{Proposals: entries}
	enc := json.NewEncoder(h.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// writeText emits the human-readable rendering. Format:
//
//	2026-02-23-spex-machina.md (project proposal)
//	  Created: spexmachina-abc (open)     schema: ProjectSchema
//	  Closed:  spexmachina-old (closed)   apply: BeadCreator
func (h *HistoryViewer) writeText(entries []proposalEntry, meta []proposalDisplay) error {
	for i, entry := range entries {
		if _, err := fmt.Fprintf(h.Out, "%s (%s)\n", entry.Filename, meta[i].label); err != nil {
			return err
		}
		for _, b := range entry.Beads {
			action := strings.ToUpper(b.Action[:1]) + b.Action[1:]
			if _, err := fmt.Fprintf(h.Out, "  %s: %s (%s)\t%s\n", action, b.ID, b.Status, b.Summary); err != nil {
				return err
			}
		}
	}
	return nil
}
