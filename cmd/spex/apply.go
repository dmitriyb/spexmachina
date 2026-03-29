package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/dmitriyb/spexmachina/apply"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
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

	if report.Summary.CreateCount == 0 && report.Summary.ObsoleteCount == 0 {
		fmt.Fprintln(os.Stderr, "spex apply: nothing to do")
		return nil
	}

	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	if dryRunFlag {
		printDryRun(report)
		return nil
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	beadCLI, _ := cmd.Flags().GetString("bead-cli")
	cli, err := apply.NewBeadCLI(ctx, beadCLI)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// Build merkle tree for hash lookup and node maps for name resolution.
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	hashes := flattenTree(tree)

	modules, err := buildNodeMaps(specDir)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// 1. Creates
	// TODO(bead:spexmachina-riw): fix after spexmachina-igq changed CreateBeads signature to accept mapping.Store
	createActions := convertCreateActions(report.Creates, modules, hashes)
	_ = createActions
	var createdIDs []string
	// createdIDs, err := apply.CreateBeads(ctx, cli, store, createActions)
	// if err != nil {
	// 	return fmt.Errorf("apply: %w", err)
	// }

	// 2. Obsoletes
	obsoleteActions := convertObsoleteActions(report.Obsoletes)
	if err := apply.CloseBeads(ctx, cli, obsoleteActions, logger); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// 3. Tag all affected beads with proposal.
	proposalFlag, _ := cmd.Flags().GetString("proposal")
	if proposalFlag != "" {
		allIDs := collectAffectedIDs(createdIDs, report.Obsoletes)
		if err := apply.TagWithProposal(ctx, cli, allIDs, proposalFlag, logger); err != nil {
			fmt.Fprintf(os.Stderr, "spex apply: tag warnings: %v\n", err)
		}
	}

	// 4. Save snapshot.
	if err := apply.SaveSnapshot(ctx, specDir, time.Now()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	fmt.Fprintf(os.Stderr, "spex apply: done (created=%d obsoleted=%d)\n",
		len(createdIDs), len(report.Obsoletes))
	return nil
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

// printDryRun prints the impact report summary without executing any actions.
func printDryRun(report impact.ImpactReport) {
	fmt.Printf("dry-run: %d creates, %d obsoletes\n",
		report.Summary.CreateCount, report.Summary.ObsoleteCount)
	for _, a := range report.Creates {
		fmt.Printf("  create:   %s/%s\n", a.Module, a.Node)
	}
	for _, a := range report.Obsoletes {
		fmt.Printf("  obsolete: %s (bead %s)\n", a.Node, a.BeadID)
	}
}

// flattenTree walks a merkle tree and returns a map of key → hash for all leaves.
// Keys are spec-ID format, e.g. "module/1/component/2".
func flattenTree(n *merkle.Node) map[string]string {
	leaves := make(map[string]string)
	walkTree(leaves, n)
	return leaves
}

func walkTree(leaves map[string]string, n *merkle.Node) {
	if n.Type == "leaf" {
		leaves[n.Key] = n.Hash
		return
	}
	for _, child := range n.Children {
		walkTree(leaves, child)
	}
}

// lookupHash finds the hash of a spec node in the merkle tree by its key.
func lookupHash(hashes map[string]string, key string) string {
	return hashes[key]
}

// nodeType returns the apply node type for a spec-ID key.
func nodeType(key string) string {
	// Parse key: module/<id>/<node_type>/<node_id>
	parts := splitKey(key)
	if len(parts) >= 3 {
		return parts[2] // "component", "impl_section", "data_flow"
	}
	return ""
}

// nodeGroup is no longer needed with spec-ID keys but kept for backward
// compatibility with the impact report format.
func nodeGroup(filename string) string {
	return ""
}

// resolveNodeName resolves an impact action's node reference to a spec node name
// using module NodeMaps. Falls back to the raw node value if no mapping exists.
// Handles spec-ID paths like "module/1/component/2" by converting to the
// type-qualified key "component/2" for NodeMap lookup.
func resolveNodeName(modules map[string]impact.NodeMap, module, node string) string {
	if nm, ok := modules[module]; ok {
		// Try direct lookup first.
		if name, ok := nm[node]; ok {
			return name
		}
		// Parse spec-ID: module/<id>/<type>/<nodeID> → type/nodeID
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
func convertCreateActions(creates []impact.Action, modules map[string]impact.NodeMap, hashes map[string]string) []apply.Action {
	actions := make([]apply.Action, 0, len(creates))
	for _, c := range creates {
		name := resolveNodeName(modules, c.Module, c.Node)
		actions = append(actions, apply.Action{
			Module:   c.Module,
			Node:     name,
			NodeType: nodeType(c.Node),
			SpecHash: lookupHash(hashes, c.Node),
		})
	}
	return actions
}

// convertObsoleteActions converts impact obsolete actions to apply actions.
func convertObsoleteActions(obsoletes []impact.Action) []apply.Action {
	actions := make([]apply.Action, 0, len(obsoletes))
	for _, o := range obsoletes {
		actions = append(actions, apply.Action{
			BeadID: o.BeadID,
			Module: o.Module,
			Node:   o.Node,
		})
	}
	return actions
}

// collectAffectedIDs gathers all bead IDs affected by the apply operation.
func collectAffectedIDs(createdIDs []string, obsoletes []impact.Action) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, id := range createdIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, o := range obsoletes {
		if !seen[o.BeadID] {
			seen[o.BeadID] = true
			ids = append(ids, o.BeadID)
		}
	}
	return ids
}
