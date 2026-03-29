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

	// Build NodeMaps for name resolution.
	modules, err := buildNodeMaps(specDir)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// Convert impact actions to apply actions.
	creates := convertCreateActions(report.Creates, modules, store)
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

// resolveNodeName resolves an impact action's node reference to a spec node name
// using module NodeMaps. Falls back to the raw node value if no mapping exists.
func resolveNodeName(modules map[string]impact.NodeMap, module, node string) string {
	if nm, ok := modules[module]; ok {
		if name, ok := nm[node]; ok {
			return name
		}
		parts := splitKey(node)
		if len(parts) >= 4 && parts[0] == "module" {
			nmKey := parts[2] + "/" + parts[3]
			if name, ok := nm[nmKey]; ok {
				return name
			}
		}
	}
	return node
}

// splitKey splits a spec-ID key into its path segments.
func splitKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}

// convertCreateActions converts impact create actions to apply actions.
func convertCreateActions(creates []impact.Action, modules map[string]impact.NodeMap, store mapping.Store) []apply.Action {
	actions := make([]apply.Action, 0, len(creates))
	for _, c := range creates {
		name := resolveNodeName(modules, c.Module, c.Node)

		specNID := ""
		if c.OldBeadID != "" {
			if rec, err := store.GetByBead(c.OldBeadID); err == nil {
				specNID = rec.SpecNodeID
			}
		}
		if specNID == "" {
			specNID = deriveSpecNodeID(c.Module, c.Node, c.NodeType)
		}

		actions = append(actions, apply.Action{
			Module:     c.Module,
			Node:       name,
			NodeType:   c.NodeType,
			SpecHash:   c.SpecHash,
			SpecNodeID: specNID,
			OldBeadID:  c.OldBeadID,
			DepBeadIDs: c.DepBeadIDs,
			Priority:   -1,
			Reason:     c.Reason,
		})
	}
	return actions
}

// deriveSpecNodeID constructs a SpecNodeID from a merkle key path or falls back
// to module/nodeType.
func deriveSpecNodeID(moduleName, node, nodeType string) string {
	parts := splitKey(node)
	if len(parts) >= 4 && parts[0] == "module" {
		return moduleName + "/" + parts[2] + "/" + parts[3]
	}
	if len(parts) >= 2 && parts[0] == "module" {
		return moduleName + "/module"
	}
	if nodeType != "" {
		return moduleName + "/" + nodeType
	}
	return moduleName
}

// convertObsoleteActions converts impact obsolete actions to apply actions.
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
			ChangeType: ct,
		})
	}
	return actions
}
