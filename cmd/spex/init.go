package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the .spex/ project state directory",
		Long: `spex init creates .spex/ with exactly two files: a snapshot seeded
with the canonical empty tree — never a snapshot of the spec that already
exists — and an empty journal, with deliberately no init event.

spex init refuses a directory that already has .spex/, whatever its
condition, and leaves it untouched: it is the one command that can
destroy a journal, so it never overwrites.`,
		Args: cobra.NoArgs,
		RunE: runInitE,
	}
}

// runInitE is InitCommand: it consults the lifecycle pre-flight to
// classify the directory rather than re-deriving the rules — the
// resolver's "never initialised" answer is init's precondition, per
// spec/lifecycle/arch_init_command.md "Refusal". Any other outcome
// (already initialised, or broken) means .spex/ exists in some form, and
// init leaves it untouched.
func runInitE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}
	root := resolveProjectRoot(specDir)

	var uninit *lifecycle.UninitializedError
	if _, resolveErr := lifecycle.Resolve(root); !errors.As(resolveErr, &uninit) {
		return fmt.Errorf("init: %s already exists", filepath.Join(root, lifecycle.StateDirName))
	}

	stateDir := filepath.Join(root, lifecycle.StateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("init: create %s: %w", stateDir, err)
	}

	snapshotPath := filepath.Join(stateDir, lifecycle.SnapshotFileName)
	if err := merkle.Save(merkle.EmptyTree(), snapshotPath, time.Now().UTC()); err != nil {
		return fmt.Errorf("init: seed snapshot: %w", err)
	}

	journalPath := filepath.Join(stateDir, lifecycle.JournalFileName)
	if err := os.WriteFile(journalPath, nil, 0644); err != nil {
		return fmt.Errorf("init: seed journal: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "initialized: %s\n", stateDir)
	return nil
}
