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
		Short: "Show proposal history and linked task actions (reads task JSON on stdin)",
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
		return fmt.Errorf("spex log: no task data on stdin; pipe 'br list --json' or equivalent")
	}

	tasks, err := parseTaskRecords(data)
	if err != nil {
		return fmt.Errorf("spex log: %w", err)
	}

	if ref != "" {
		tasks = filterByProposalRef(tasks, ref)
	}

	hv := &proposal.HistoryViewer{
		SpecDir: specDir,
		Out:     cmd.OutOrStdout(),
		JSON:    asJSON,
	}
	return hv.ShowHistory(tasks)
}

// parseTaskRecords accepts either the {"issues": [...]} envelope produced by
// `br list --json` or a bare JSON array of task records and returns the slice
// in proposal.TaskRecord shape. Stays in lockstep with plan.ReadBeadsBytes
// so the same tracker output drives both pipelines.
func parseTaskRecords(data []byte) ([]proposal.TaskRecord, error) {
	var wrap struct {
		Issues []proposal.TaskRecord `json:"issues"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Issues != nil {
		return wrap.Issues, nil
	}
	var bare []proposal.TaskRecord
	if err := json.Unmarshal(data, &bare); err != nil {
		return nil, fmt.Errorf("parse task JSON: %w", err)
	}
	return bare, nil
}

// filterByProposalRef keeps only tasks carrying a spec_proposal:<stem> label
// matching ref. Both the bare stem and a trailing-".md" form are accepted, to
// match the tolerance HistoryViewer.firstProposalStem already gives writers.
func filterByProposalRef(tasks []proposal.TaskRecord, ref string) []proposal.TaskRecord {
	stem := strings.TrimSuffix(ref, ".md")
	want := "spec_proposal:" + stem
	out := make([]proposal.TaskRecord, 0, len(tasks))
	for _, t := range tasks {
		for _, lbl := range t.Labels {
			if lbl == want || lbl == want+".md" {
				out = append(out, t)
				break
			}
		}
	}
	return out
}
