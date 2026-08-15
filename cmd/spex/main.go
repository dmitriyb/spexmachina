package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/cli"
)

func main() {
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newHashIDCmd(),
		newDiffCmd(),
		newValidateCmd(),
		newImpactCmd(),
		newMapCmd(),
		newRegisterCmd(),
		newLogCmd(),
		newTemplateCmd(),
		newVersionCmd(),
		newRenderCmd(),
		newEmitCmd(),
		newPlanCmd(),
		newIngestCmd(),
		newUpgradeCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}
