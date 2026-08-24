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
		newInitCmd(),
		newHashIDCmd(),
		newDiffCmd(),
		newValidateCmd(),
		newPlanCmd(),
		newMapCmd(),
		newRegisterCmd(),
		newLogCmd(),
		newTemplateCmd(),
		newVersionCmd(),
		newRenderCmd(),
		newIngestCmd(),
		newUpgradeCmd(),
		newDoctorCmd(),
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
