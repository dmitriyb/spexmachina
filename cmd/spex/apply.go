package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/apply"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Execute bead actions from impact report",
		RunE:  runApplyE,
	}
	cmd.Flags().String("report", "", "path to impact report JSON (default: stdin)")
	cmd.Flags().String("bead-cli", "br", "bead CLI binary name")
	cmd.Flags().String("proposal", "", "proposal reference to tag on affected beads")
	cmd.Flags().Bool("dry-run", false, "print actions without executing")
	_ = cmd.MarkFlagRequired("proposal")
	return cmd
}

func runApplyE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	reportFlag, _ := cmd.Flags().GetString("report")
	reportData, err := readReport(reportFlag)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal(reportData, &report); err != nil {
		return fmt.Errorf("apply: parse report: %w", err)
	}

	proposalFlag, _ := cmd.Flags().GetString("proposal")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")

	// Resolve mapping store.
	mapPath := filepath.Join(filepath.Dir(specDir), ".bead-map.json")
	store := mapping.NewFileStore(mapPath)

	// Build NodeMaps for name and content file resolution by identity hash.
	modules, contents, err := buildNodeMaps(specDir)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// SpecGraph gives test_section describes arrays for the BeadCreator
	// defense-in-depth gate. A nil graph would cause DescribesCount to fall
	// through to 0, tripping the gate on valid multi-component test_sections
	// with a misleading "ActionClassifier should have filtered it" error — so
	// propagate the error instead of swallowing it.
	specGraph, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		return fmt.Errorf("apply: load spec graph: %w", err)
	}

	// Convert impact actions to apply actions.
	creates := convertCreateActions(report.Creates, modules, contents, store, specGraph)
	obsoletes := convertObsoleteActions(report.Obsoletes)

	opts := apply.ApplyOpts{
		Creates:     creates,
		Obsoletes:   obsoletes,
		ProposalRef: proposalFlag,
		DryRun:      dryRunFlag,
		SpecDir:     specDir,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}

	if dryRunFlag {
		return apply.RunApply(context.Background(), nil, nil, opts)
	}

	ctx := context.Background()
	beadCLIFlag, _ := cmd.Flags().GetString("bead-cli")
	cli, err := apply.NewBeadCLI(ctx, beadCLIFlag)
	if err != nil {
		return err
	}

	opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return apply.RunApply(ctx, cli, store, opts)
}

// readReport reads the impact report from a file or stdin.
func readReport(path string) ([]byte, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read report: %w", err)
		}
		return data, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return data, nil
}

// resolveNodeName resolves an impact action's node reference to a spec node
// name via module NodeMaps (keyed by identity hash). Returns the raw input
// when no mapping exists.
func resolveNodeName(modules map[string]impact.NodeMap, module, node string) string {
	if nm, ok := modules[module]; ok {
		if name, ok := nm[node]; ok {
			return name
		}
	}
	return node
}

// convertCreateActions converts impact create actions to apply actions.
// SpecNodeID flows through unchanged from the impact report — both the merkle
// diff and the mapping store are keyed by the same identity hash.
func convertCreateActions(creates []impact.Action, modules map[string]impact.NodeMap, contents map[string]ContentMap, store mapping.Store, specGraph mapping.SpecGraph) []apply.Action {
	actions := make([]apply.Action, 0, len(creates))
	for _, c := range creates {
		name := resolveNodeName(modules, c.Module, c.Node)

		// Resolve content file: prefer the existing mapping record (for modified
		// nodes), falling back to the spec graph (for new nodes).
		contentFile := ""
		if recs, err := store.GetBySpecNode(c.SpecNodeID); err == nil && len(recs) > 0 {
			contentFile = recs[0].ContentFile
		}
		if contentFile == "" {
			contentFile = resolveContentFile(contents, c.Module, c.SpecNodeID)
		}

		actions = append(actions, apply.Action{
			Module:      c.Module,
			Node:        name,
			NodeType:    c.NodeType,
			SpecHash:    c.SpecHash,
			SpecNodeID:  c.SpecNodeID,
			ContentFile: contentFile,
			OldBeadID:   c.OldBeadID,
			// TODO(bead:spexmachina-0lk.3): apply is being retired. DepSpecNodeIDs
			// now carries identity hashes, not bead IDs — emit's Resolver owns the
			// bead-ID resolution. apply.Action.DepBeadIDs is left empty here until
			// the apply → emit cutover completes.
			DepBeadIDs:     nil,
			DescribesCount: describesCount(specGraph, c.Module, c.NodeType, c.SpecNodeID),
			Priority:       -1,
			Reason:         c.Reason,
		})
	}
	return actions
}

// describesCount returns the length of a test_section's describes array.
// Returns 0 for non-test_section nodes or when the spec graph cannot resolve
// the module/section (e.g. graph nil or lookup failure).
func describesCount(specGraph mapping.SpecGraph, module, nodeType, specNodeID string) int {
	if nodeType != "test_section" || specGraph == nil {
		return 0
	}
	modInfo, err := specGraph.ModuleByName(module)
	if err != nil {
		return 0
	}
	for _, ts := range modInfo.TestSections {
		if ts.ID == specNodeID {
			return len(ts.Describes)
		}
	}
	return 0
}

// resolveContentFile looks up the content file path for a node via the
// ContentMap keyed by identity hash.
func resolveContentFile(contents map[string]ContentMap, module, specNodeID string) string {
	if cm, ok := contents[module]; ok {
		if cf, ok := cm[specNodeID]; ok {
			return cf
		}
	}
	return ""
}

// convertObsoleteActions converts impact obsolete actions to apply actions.
// SpecNodeID is carried through so BeadCloser can locate the mapping record
// by identity hash when deleting records for removed nodes.
func convertObsoleteActions(obsoletes []impact.Action) []apply.Action {
	actions := make([]apply.Action, 0, len(obsoletes))
	for _, o := range obsoletes {
		ct := o.ChangeType
		if ct == "" {
			ct = "modified"
		}
		actions = append(actions, apply.Action{
			BeadID:     o.BeadID,
			Module:     o.Module,
			Node:       o.Node,
			SpecNodeID: o.SpecNodeID,
			ChangeType: ct,
		})
	}
	return actions
}
