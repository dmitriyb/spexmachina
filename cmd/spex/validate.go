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
		Args:  cobra.NoArgs,
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
	errs = append(errs, validator.CheckLinks(specDir)...)
	errs = append(errs, validator.CheckIDs(specDir)...)
	errs = append(errs, validator.CheckIDDerivation(specDir)...)
	errs = append(errs, validator.CheckDAG(specDir)...)
	errs = append(errs, validator.CheckNameConsistency(specDir)...)
	errs = append(errs, validator.CheckTestCoverage(specDir)...)
	errs = append(errs, validator.CheckRequirementCoverage(specDir)...)
	errs = append(errs, validator.CheckCoupledSections(specDir)...)

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	report, err := validator.Report(errs, os.Stdout, isTTY)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Exit status is read off the report that was just serialized, so it can
	// never disagree with the `valid` field the caller sees on stdout.
	if !report.Valid {
		return fmt.Errorf("validation failed")
	}
	return nil
}
