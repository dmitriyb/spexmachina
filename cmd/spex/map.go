package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/spf13/cobra"
)

func newMapCmd() *cobra.Command {
	mapCmd := &cobra.Command{
		Use:   "map",
		Short: "Manage bead mapping records",
	}

	getCmd := &cobra.Command{
		Use:   "get <record-id>",
		Short: "Get a mapping record by ID",
		Args:  cobra.ExactArgs(1),
		RunE:  runMapGetE,
	}
	getCmd.Flags().String("map-file", ".bead-map.json", "path to mapping file")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all mapping records",
		RunE:  runMapListE,
	}
	listCmd.Flags().String("map-file", ".bead-map.json", "path to mapping file")

	mapCmd.AddCommand(getCmd, listCmd)
	return mapCmd
}

func runMapGetE(cmd *cobra.Command, args []string) error {
	mapFile, _ := cmd.Flags().GetString("map-file")

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("map get: invalid record ID: %s", args[0])
	}

	store := mapping.NewFileStore(mapFile)
	record, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("map get: %w", err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		return fmt.Errorf("map get: %w", err)
	}
	return nil
}

func runMapListE(cmd *cobra.Command, args []string) error {
	mapFile, _ := cmd.Flags().GetString("map-file")

	store := mapping.NewFileStore(mapFile)
	records, err := store.List()
	if err != nil {
		return fmt.Errorf("map list: %w", err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(records); err != nil {
		return fmt.Errorf("map list: %w", err)
	}
	return nil
}

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <bead-id>",
		Short: "Validate mapping status for a bead",
		Args:  cobra.ExactArgs(1),
		RunE:  runCheckE,
	}
	cmd.Flags().String("map-file", ".bead-map.json", "path to mapping file")
	return cmd
}

func runCheckE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	mapFile, _ := cmd.Flags().GetString("map-file")

	store := mapping.NewFileStore(mapFile)
	spec, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}

	ctx := context.Background()
	result, err := mapping.Check(ctx, store, spec, args[0])
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return fmt.Errorf("check: %w", err)
	}

	if result.Status != "ready" {
		return fmt.Errorf("check: status is %s, not ready", result.Status)
	}
	return nil
}
