package main

import (
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

	contextCmd := &cobra.Command{
		Use:   "context <record-id>",
		Short: "Resolve full spec context for a mapping record",
		Args:  cobra.ExactArgs(1),
		RunE:  runMapContextE,
	}
	contextCmd.Flags().String("map-file", ".bead-map.json", "path to mapping file")

	mapCmd.AddCommand(getCmd, listCmd, contextCmd)
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

func runMapContextE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	mapFile, _ := cmd.Flags().GetString("map-file")

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("map context: invalid record ID: %s", args[0])
	}

	store := mapping.NewFileStore(mapFile)
	record, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("map context: %w", err)
	}

	result, err := mapping.ResolveContext(specDir, record)
	if err != nil {
		return fmt.Errorf("map context: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("map context: %w", err)
	}
	return nil
}
