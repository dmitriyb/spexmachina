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

	if report.Summary.CreateCount == 0 && report.Summary.CloseCount == 0 && report.Summary.ReviewCount == 0 {
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
	createActions := convertCreateActions(report.Creates, modules, hashes)
	createdIDs, err := apply.CreateBeads(ctx, cli, createActions)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// 2. Updates (reviews)
	updateActions := convertReviewActions(report.Reviews, hashes)
	if err := apply.UpdateBeads(ctx, cli, updateActions, logger); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// 3. Closes
	closeActions := convertCloseActions(report.Closes)
	if err := apply.CloseBeads(ctx, cli, closeActions, logger); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	// 4. Tag all affected beads with proposal.
	proposalFlag, _ := cmd.Flags().GetString("proposal")
	if proposalFlag != "" {
		allIDs := collectAffectedIDs(createdIDs, report.Reviews, report.Closes)
		if err := apply.TagWithProposal(ctx, cli, allIDs, proposalFlag, logger); err != nil {
			fmt.Fprintf(os.Stderr, "spex apply: tag warnings: %v\n", err)
		}
	}

	// 5. Save snapshot.
	if err := apply.SaveSnapshot(ctx, specDir, time.Now()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	fmt.Fprintf(os.Stderr, "spex apply: done (created=%d updated=%d closed=%d)\n",
		len(createdIDs), len(report.Reviews), len(report.Closes))
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
	fmt.Printf("dry-run: %d creates, %d reviews, %d closes\n",
		report.Summary.CreateCount, report.Summary.ReviewCount, report.Summary.CloseCount)
	for _, a := range report.Creates {
		fmt.Printf("  create: %s/%s\n", a.Module, a.Node)
	}
	for _, a := range report.Reviews {
		fmt.Printf("  review: %s (bead %s)\n", a.Node, a.BeadID)
	}
	for _, a := range report.Closes {
		fmt.Printf("  close:  %s (bead %s)\n", a.Node, a.BeadID)
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

// convertReviewActions converts impact review actions to apply actions.
func convertReviewActions(reviews []impact.Action, hashes map[string]string) []apply.Action {
	actions := make([]apply.Action, 0, len(reviews))
	for _, r := range reviews {
		actions = append(actions, apply.Action{
			BeadID:   r.BeadID,
			Module:   r.Module,
			Node:     r.Node,
			SpecHash: lookupHash(hashes, r.Node),
		})
	}
	return actions
}

// convertCloseActions converts impact close actions to apply actions.
func convertCloseActions(closes []impact.Action) []apply.Action {
	actions := make([]apply.Action, 0, len(closes))
	for _, c := range closes {
		actions = append(actions, apply.Action{
			BeadID: c.BeadID,
			Module: c.Module,
			Node:   c.Node,
		})
	}
	return actions
}

// collectAffectedIDs gathers all bead IDs affected by the apply operation.
func collectAffectedIDs(createdIDs []string, reviews, closes []impact.Action) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, id := range createdIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, r := range reviews {
		if !seen[r.BeadID] {
			seen[r.BeadID] = true
			ids = append(ids, r.BeadID)
		}
	}
	for _, c := range closes {
		if !seen[c.BeadID] {
			seen[c.BeadID] = true
			ids = append(ids, c.BeadID)
		}
	}
	return ids
}
