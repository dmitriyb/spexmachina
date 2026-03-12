package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

func newHashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hash",
		Short: "Build merkle tree and save snapshot",
		RunE:  runHashE,
	}
	cmd.Flags().Bool("json", false, "output as JSON")
	return cmd
}

func runHashE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return printJSON(tree)
	}
	printSummary(tree)
	return nil
}

// hashOutput is the JSON representation of the hash command result.
type hashOutput struct {
	RootHash string     `json:"root_hash"`
	Nodes    []hashNode `json:"nodes"`
}

type hashNode struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
	Type string `json:"type"`
}

func printJSON(tree *merkle.Node) error {
	out := hashOutput{RootHash: tree.Hash}
	collectNodes(&out.Nodes, tree)

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("hash: marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func collectNodes(nodes *[]hashNode, n *merkle.Node) {
	*nodes = append(*nodes, hashNode{
		Key:  n.Key,
		Hash: n.Hash,
		Type: n.Type,
	})
	for _, child := range n.Children {
		collectNodes(nodes, child)
	}
}

func printSummary(tree *merkle.Node) {
	fmt.Printf("root: %s\n", tree.Hash)

	counts := make(map[string]int)
	countNodes(tree, counts)

	fmt.Printf("nodes: %d total", totalCount(counts))
	for _, typ := range []string{"project", "module", "leaf"} {
		if c, ok := counts[typ]; ok {
			fmt.Printf(", %d %s", c, typ)
		}
	}
	fmt.Println()
	fmt.Printf("snapshot: .snapshot.json\n")
}

func countNodes(n *merkle.Node, counts map[string]int) {
	counts[n.Type]++
	for _, child := range n.Children {
		countNodes(child, counts)
	}
}

func totalCount(counts map[string]int) int {
	total := 0
	for _, c := range counts {
		total += c
	}
	return total
}