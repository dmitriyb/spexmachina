package main

import (
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/cli"
)

func main() {
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newHashCmd(),
		newDiffCmd(),
		newValidateCmd(),
		newImpactCmd(),
		newApplyCmd(),
		newMapCmd(),
		newRegisterCmd(),
		newLogCmd(),
		newTemplateCmd(),
		newVersionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
