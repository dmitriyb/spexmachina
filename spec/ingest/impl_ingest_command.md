# Ingest command implementation

Implementation notes for `cmd/spex/ingest.go`.

## Skeleton

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/dmitriyb/spexmachina/emit"
    "github.com/dmitriyb/spexmachina/ingest"
    "github.com/dmitriyb/spexmachina/mapping"
    "github.com/dmitriyb/spexmachina/receipts"
    "github.com/dmitriyb/spexmachina/spec"
    "github.com/spf13/cobra"
)

func newIngestCmd() *cobra.Command {
    var changesetPath, receiptsPath string
    cmd := &cobra.Command{
        Use:   "ingest",
        Short: "Reconcile mapping records and save snapshot from a receipts.json produced by an adapter",
        RunE: func(cmd *cobra.Command, args []string) error {
            cs, err := loadChangeset(changesetPath)
            if err != nil { return err }
            rc, err := loadReceipts(receiptsPath)
            if err != nil { return err }

            if err := preflightPair(cs, rc); err != nil { return err }

            store, err := mapping.Open("./.bead-map.json")
            if err != nil { return err }
            defer store.Close()

            graph, err := spec.Load("./spec")
            if err != nil { return err }

            reconciler := &ingest.Reconciler{MappingStore: store, SpecGraph: graph}
            summary, err := reconciler.Apply(cs, rc)
            if err != nil {
                return exitInvariant(err)
            }

            saver := &ingest.Saver{SpecDir: "./spec"}
            wrote, err := saver.Save(rc.Status)
            if err != nil {
                return fmt.Errorf("ingest: snapshot: %w", err)
            }

            final := Summary{
                Ok: summary.OkCreates + summary.OkCloses,
                Skipped: summary.Skipped,
                Errors: summary.Errors,
                RecordsAdded: summary.RecordsAdded,
                RecordsUpdated: summary.RecordsUpdated,
                RecordsDeleted: summary.RecordsDeleted,
                SnapshotSaved: wrote,
                Status: rc.Status,
            }
            return json.NewEncoder(cmd.OutOrStdout()).Encode(final)
        },
    }
    cmd.Flags().StringVar(&changesetPath, "changeset", "", "path to changeset.json")
    cmd.Flags().StringVar(&receiptsPath,  "receipts",  "", "path to receipts.json")
    _ = cmd.MarkFlagRequired("changeset")
    _ = cmd.MarkFlagRequired("receipts")
    return cmd
}
```

## Preflight

```go
func preflightPair(cs emit.Changeset, rc receipts.Receipts) error {
    if cs.Version != 1 { return fmt.Errorf("ingest: changeset version must be 1, got %d", cs.Version) }
    if rc.Version != 1 { return fmt.Errorf("ingest: receipts version must be 1, got %d", rc.Version) }

    opIDs := map[string]bool{}
    for _, op := range cs.Ops { opIDs[op.OpID] = true }

    for _, rop := range rc.Ops {
        if !opIDs[rop.OpID] {
            return fmt.Errorf("ingest: receipt op_id %s not in changeset", rop.OpID)
        }
    }
    // Every op must have a receipt.
    rcIDs := map[string]bool{}
    for _, rop := range rc.Ops { rcIDs[rop.OpID] = true }
    for _, op := range cs.Ops {
        if !rcIDs[op.OpID] {
            return fmt.Errorf("ingest: no receipt for op %s", op.OpID)
        }
    }
    return nil
}
```

## Exit Code Mapping

```go
type invariantErr struct{ err error }
func (e *invariantErr) Error() string { return e.err.Error() }

func exitInvariant(err error) error {
    // Wrap in a sentinel that the root command translates to exit code 2.
    return &invariantErr{err: err}
}
```

In the root command's ExecuteC, check for `*invariantErr` and set `os.Exit(2)`; other RunE errors → exit 1.

## Summary Struct

```go
type Summary struct {
    Ok             int    `json:"ok"`
    Skipped        int    `json:"skipped"`
    Errors         int    `json:"errors"`
    RecordsAdded   int    `json:"records_added"`
    RecordsUpdated int    `json:"records_updated"`
    RecordsDeleted int    `json:"records_deleted"`
    SnapshotSaved  bool   `json:"snapshot_saved"`
    Status         string `json:"status"` // complete|partial
}
```

## Registration

`cmd/spex/main.go`:

```go
root.AddCommand(newIngestCmd())
```

## Non-Responsibilities

- Ingest does not rebuild the snapshot from scratch if reconciliation fails; it just returns.
- Ingest does not attempt to recover from an inconsistent .bead-map.json on load — it delegates to `mapping.Open`'s schema validation.
