package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/dmitriyb/spexmachina/validator"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate spec directory structure",
		RunE:  runValidateE,
	}
}

func runValidateE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	var errs []validator.ValidationError
	errs = append(errs, validator.CheckSchema(specDir)...)
	errs = append(errs, validator.CheckContentPaths(specDir)...)
	errs = append(errs, validator.CheckIDs(specDir)...)
	errs = append(errs, validator.CheckDAG(specDir)...)
	errs = append(errs, validator.CheckOrphans(specDir)...)

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if err := validator.Report(errs, os.Stdout, isTTY); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	for _, e := range errs {
		if e.Severity == "error" {
			return fmt.Errorf("validation failed")
		}
	}
	return nil
}
