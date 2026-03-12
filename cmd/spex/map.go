package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dmitriyb/spexmachina/mapping"
)

func runMap(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: spex map <get|list> [args]")
		return 1
	}

	switch args[0] {
	case "get":
		return runMapGet(args[1:])
	case "list":
		return runMapList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "spex map: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runMapGet(args []string) int {
	fs := flag.NewFlagSet("map get", flag.ContinueOnError)
	mapFile := fs.String("map-file", ".bead-map.json", "path to mapping file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex map get: %v\n", err)
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: spex map get <record-id>")
		return 1
	}

	id, err := strconv.Atoi(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex map get: invalid record ID: %s\n", fs.Arg(0))
		return 1
	}

	store := mapping.NewFileStore(*mapFile)
	record, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex map get: %v\n", err)
		return 1
	}

	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		fmt.Fprintf(os.Stderr, "spex map get: %v\n", err)
		return 1
	}
	return 0
}

func runMapList(args []string) int {
	fs := flag.NewFlagSet("map list", flag.ContinueOnError)
	mapFile := fs.String("map-file", ".bead-map.json", "path to mapping file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex map list: %v\n", err)
		return 1
	}

	store := mapping.NewFileStore(*mapFile)
	records, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex map list: %v\n", err)
		return 1
	}

	if err := json.NewEncoder(os.Stdout).Encode(records); err != nil {
		fmt.Fprintf(os.Stderr, "spex map list: %v\n", err)
		return 1
	}
	return 0
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	mapFile := fs.String("map-file", ".bead-map.json", "path to mapping file")
	specDirFlag := fs.String("spec-dir", "spec", "path to spec directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex check: %v\n", err)
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: spex check <bead-id>")
		return 1
	}

	absSpec, err := filepath.Abs(*specDirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex check: resolve spec path: %v\n", err)
		return 1
	}

	store := mapping.NewFileStore(*mapFile)
	spec, err := mapping.NewSpecGraph(absSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex check: %v\n", err)
		return 1
	}

	ctx := context.Background()
	result, err := mapping.Check(ctx, store, spec, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex check: %v\n", err)
		return 1
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "spex check: %v\n", err)
		return 1
	}

	if result.Status != "ready" {
		return 1
	}
	return 0
}
