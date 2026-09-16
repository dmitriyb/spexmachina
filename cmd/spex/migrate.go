package main

import (
	"github.com/dmitriyb/spexmachina/author"
	"github.com/spf13/cobra"
)

// newMigrateCmd is `spex migrate`, Migrator's surface: it needs no
// initialised project and takes no flags beyond the root's --spec-dir
// (spec/author/arch_migrator.md; spec/author/flow_authoring.md, "0.
// Migrate, adopters only").
func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Rewrite a spec from an earlier format version to the current one",
		Args:  cobra.NoArgs,
		RunE:  runMigrateE,
	}
}

func runMigrateE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.Migrate(specDir)
	return finishAuthorResult("migrate", report, refusals, err)
}
