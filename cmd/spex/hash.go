package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

func runHash(args []string) int {
	fs := flag.NewFlagSet("hash", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex hash: %v\n", err)
		return 1
	}

	specDir := "spec"
	if fs.NArg() > 0 {
		specDir = fs.Arg(0)
	}

	specDir, err := filepath.Abs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex hash: resolve path: %v\n", err)
		return 1
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex hash: %v\n", err)
		return 1
	}

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "spex hash: %v\n", err)
		return 1
	}

	if *jsonOut {
		return printJSON(tree)
	}
	printSummary(tree)
	return 0
}

// hashOutput is the JSON representation of the hash command result.
type hashOutput struct {
	RootHash string       `json:"root_hash"`
	Nodes    []hashNode   `json:"nodes"`
}

type hashNode struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Type string `json:"type"`
}

func printJSON(tree *merkle.Node) int {
	out := hashOutput{RootHash: tree.Hash}
	collectNodes(&out.Nodes, tree, "")

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex hash: marshal json: %v\n", err)
		return 1
	}
	fmt.Println(string(data))
	return 0
}

func collectNodes(nodes *[]hashNode, n *merkle.Node, prefix string) {
	path := n.Name
	if prefix != "" {
		path = prefix + "/" + n.Name
	}
	*nodes = append(*nodes, hashNode{
		Path: path,
		Hash: n.Hash,
		Type: n.Type,
	})
	for _, child := range n.Children {
		collectNodes(nodes, child, path)
	}
}

func printSummary(tree *merkle.Node) {
	fmt.Printf("root: %s\n", tree.Hash)

	counts := make(map[string]int)
	countNodes(tree, counts)

	fmt.Printf("nodes: %d total", totalCount(counts))
	for _, typ := range []string{"project", "module", "arch", "impl", "flow", "leaf"} {
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
