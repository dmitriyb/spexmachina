package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"

	"github.com/dmitriyb/spexmachina/validator"
)

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex validate: %v\n", err)
		return 1
	}

	specDir := "spec"
	if fs.NArg() > 0 {
		specDir = fs.Arg(0)
	}

	specDir, err := filepath.Abs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex validate: resolve path: %v\n", err)
		return 1
	}

	var errs []validator.ValidationError
	errs = append(errs, validator.CheckSchema(specDir)...)
	errs = append(errs, validator.CheckContentPaths(specDir)...)
	errs = append(errs, validator.CheckIDs(specDir)...)
	errs = append(errs, validator.CheckDAG(specDir)...)
	errs = append(errs, validator.CheckOrphans(specDir)...)

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if err := validator.Report(errs, os.Stdout, isTTY); err != nil {
		fmt.Fprintf(os.Stderr, "spex validate: %v\n", err)
		return 1
	}

	for _, e := range errs {
		if e.Severity == "error" {
			return 1
		}
	}
	return 0
}
