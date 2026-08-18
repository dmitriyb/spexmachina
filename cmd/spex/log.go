package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show proposal history and linked bead actions (reads bead JSON on stdin)",
		RunE:  runLogE,
	}
	cmd.Flags().Bool("json", false, "output in JSON format")
	cmd.Flags().String("proposal", "", "filter to a single proposal stem, e.g. 2026-04-18-decouple-spex-from-br")
	return cmd
}

func runLogE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	ref, _ := cmd.Flags().GetString("proposal")

	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("spex log: read stdin: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("spex log: no bead data on stdin; pipe 'br list --json' or equivalent")
	}

	beads, err := parseBeadRecords(data)
	if err != nil {
		return fmt.Errorf("spex log: %w", err)
	}

	if ref != "" {
		beads = filterByProposalRef(beads, ref)
	}

	hv := &proposal.HistoryViewer{
		SpecDir: specDir,
		Out:     cmd.OutOrStdout(),
		JSON:    asJSON,
	}
	return hv.ShowHistory(beads)
}

// parseBeadRecords accepts either the {"issues": [...]} envelope produced by
// `br list --json` or a bare JSON array of bead records and returns the slice
// in proposal.BeadRecord shape. Stays in lockstep with plan.ReadBeadsBytes
// so the same tracker output drives both pipelines.
func parseBeadRecords(data []byte) ([]proposal.BeadRecord, error) {
	var wrap struct {
		Issues []proposal.BeadRecord `json:"issues"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Issues != nil {
		return wrap.Issues, nil
	}
	var bare []proposal.BeadRecord
	if err := json.Unmarshal(data, &bare); err != nil {
		return nil, fmt.Errorf("parse bead JSON: %w", err)
	}
	return bare, nil
}

// filterByProposalRef keeps only beads carrying a spec_proposal:<stem> label
// matching ref. Both the bare stem and a trailing-".md" form are accepted, to
// match the tolerance HistoryViewer.firstProposalStem already gives writers.
func filterByProposalRef(beads []proposal.BeadRecord, ref string) []proposal.BeadRecord {
	stem := strings.TrimSuffix(ref, ".md")
	want := "spec_proposal:" + stem
	out := make([]proposal.BeadRecord, 0, len(beads))
	for _, b := range beads {
		for _, lbl := range b.Labels {
			if lbl == want || lbl == want+".md" {
				out = append(out, b)
				break
			}
		}
	}
	return out
}
