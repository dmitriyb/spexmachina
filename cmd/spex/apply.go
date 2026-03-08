package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmitriyb/spexmachina/apply"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
)

func runApply(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	reportFlag := fs.String("report", "", "path to impact report JSON (default: stdin)")
	beadCLI := fs.String("bead-cli", "br", "bead CLI binary name")
	proposalFlag := fs.String("proposal", "", "proposal reference to tag on affected beads")
	dryRunFlag := fs.Bool("dry-run", false, "print actions without executing")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	specDir := "spec"
	if fs.NArg() > 0 {
		specDir = fs.Arg(0)
	}

	// Read impact report.
	reportData, err := readReport(*reportFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	var report impact.ImpactReport
	if err := json.Unmarshal(reportData, &report); err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: parse report: %v\n", err)
		return 1
	}

	if report.Summary.CreateCount == 0 && report.Summary.CloseCount == 0 && report.Summary.ReviewCount == 0 {
		fmt.Fprintln(os.Stderr, "spex apply: nothing to do")
		return 0
	}

	if *dryRunFlag {
		return printDryRun(report)
	}

	absSpec, err := filepath.Abs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: resolve spec path: %v\n", err)
		return 1
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cli, err := apply.NewBeadCLI(ctx, *beadCLI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	// Build merkle tree for hash lookup and node maps for name resolution.
	tree, err := merkle.BuildTree(absSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}
	hashes := flattenTree(tree)

	modules, err := buildNodeMaps(absSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	// 1. Creates
	createActions := convertCreateActions(report.Creates, modules, hashes)
	createdIDs, err := apply.CreateBeads(ctx, cli, createActions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	// 2. Updates (reviews)
	updateActions := convertReviewActions(report.Reviews, hashes)
	if err := apply.UpdateBeads(ctx, cli, updateActions, logger); err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	// 3. Closes
	closeActions := convertCloseActions(report.Closes)
	if err := apply.CloseBeads(ctx, cli, closeActions, logger); err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	// 4. Tag all affected beads with proposal.
	if *proposalFlag != "" {
		allIDs := collectAffectedIDs(createdIDs, report.Reviews, report.Closes)
		if err := apply.TagWithProposal(ctx, cli, allIDs, *proposalFlag, logger); err != nil {
			fmt.Fprintf(os.Stderr, "spex apply: tag warnings: %v\n", err)
		}
	}

	// 5. Save snapshot.
	if err := apply.SaveSnapshot(ctx, absSpec, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "spex apply: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "spex apply: done (created=%d updated=%d closed=%d)\n",
		len(createdIDs), len(report.Reviews), len(report.Closes))
	return 0
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
func printDryRun(report impact.ImpactReport) int {
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
	return 0
}

// flattenTree walks a merkle tree and returns a map of path → hash for all leaves.
// Paths follow the merkle tree convention: "module/group/file.md".
func flattenTree(n *merkle.Node) map[string]string {
	leaves := make(map[string]string)
	walkTree(leaves, n, "")
	return leaves
}

func walkTree(leaves map[string]string, n *merkle.Node, prefix string) {
	var key string
	if prefix == "" {
		key = n.Name
	} else {
		key = prefix + "/" + n.Name
	}

	if n.Type == "leaf" {
		leaves[key] = n.Hash
		return
	}
	for _, child := range n.Children {
		walkTree(leaves, child, key)
	}
}

// lookupHash finds the hash of a spec node in the merkle tree.
// The impact report Node for creates/reviews is a filename like "arch_comp.md".
// The merkle tree path is "project/module/group/file.md".
func lookupHash(hashes map[string]string, module, node string) string {
	group := nodeGroup(node)
	// Try all paths — the project name prefix varies.
	for path, hash := range hashes {
		if strings.HasSuffix(path, "/"+module+"/"+group+"/"+node) {
			return hash
		}
	}
	return ""
}

// nodeGroup returns the merkle tree group for a filename based on its prefix.
func nodeGroup(filename string) string {
	switch {
	case strings.HasPrefix(filename, "arch_"):
		return "arch"
	case strings.HasPrefix(filename, "impl_"):
		return "impl"
	case strings.HasPrefix(filename, "flow_"):
		return "flow"
	default:
		return ""
	}
}

// nodeType returns the apply node type for a filename based on its prefix.
func nodeType(filename string) string {
	switch {
	case strings.HasPrefix(filename, "arch_"):
		return "component"
	case strings.HasPrefix(filename, "impl_"):
		return "impl_section"
	default:
		return ""
	}
}

// resolveNodeName resolves a filename to a spec node name using module NodeMaps.
// Falls back to the filename if no mapping exists.
func resolveNodeName(modules map[string]impact.NodeMap, module, filename string) string {
	if nm, ok := modules[module]; ok {
		if name, ok := nm[filename]; ok {
			return name
		}
	}
	return filename
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
			SpecHash: lookupHash(hashes, c.Module, c.Node),
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
			SpecHash: lookupHash(hashes, r.Module, r.Node),
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
