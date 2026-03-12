package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// resolveSpecDir reads the --spec-dir persistent flag from the root command
// and returns its absolute path.
func resolveSpecDir(cmd *cobra.Command) (string, error) {
	specDir, err := cmd.Root().PersistentFlags().GetString("spec-dir")
	if err != nil {
		return "", fmt.Errorf("resolve spec-dir: %w", err)
	}
	abs, err := filepath.Abs(specDir)
	if err != nil {
		return "", fmt.Errorf("resolve spec-dir: %w", err)
	}
	return abs, nil
}
