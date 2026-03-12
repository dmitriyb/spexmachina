package cli

import "github.com/spf13/cobra"

// NewRootCmd constructs the top-level spex command with global persistent flags.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spex",
		Short: "The spec state machine",
		Long: `spex owns the structural half of spec-driven development.
It defines specs as a typed graph, tracks changes via a merkle tree,
computes impact deterministically, and maps spec nodes to beads tasks.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringP("spec-dir", "s", "spec/", "path to the spec directory")

	return cmd
}
