# BeadCloser

Closes obsolete beads whose spec nodes have been removed. Removes the corresponding mapping record from `.bead-map.json`.

## Responsibilities

- Read "close" actions from the impact report
- Construct bead close commands with descriptive reasons
- Execute bead closure via `br` or `bd`
- Remove the corresponding mapping record from `.bead-map.json`

## Interface

Reuses the `BeadCLI` interface from BeadCreator, extended with a `Close` method:

```go
type BeadCLI interface {
    Create(ctx context.Context, opts CreateOpts) (string, error)
    FindExisting(ctx context.Context, labels []string) (string, error)
    Close(ctx context.Context, id string, reason string) error
}

func CloseBeads(ctx context.Context, cli BeadCLI, store map.Store, closes []Action, logger *slog.Logger) error
```

## Command Construction

For each close action:
```
<bin> close <bead_id> --reason "Spec node removed: <module>/<node_name>"
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`).

## Mapping Record Removal

After closing the bead, BeadCloser calls `store.Delete(recordID)` to remove the mapping record. The record ID is obtained from the bead's `spex:<record-id>` label or from the mapping store lookup by spec node ID.

## Idempotency & Error Handling

Close errors are logged as warnings but do not abort the batch. Exit code 0 means success; any non-zero exit is treated as a warning. This covers:

- Bead already closed (idempotent)
- Bead ID no longer exists (manually closed between diff and apply)

If the bead close succeeds but the mapping record deletion fails, the error is logged as a warning. The orphaned record will be cleaned up on the next `spex apply`.

Only binary-not-found errors (from `NewBeadCLI` construction) are fatal.

`CloseBeads` returns a summary error aggregating all warnings, or nil if all succeeded.

## External Binary Compatibility

BeadCloser shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: Reuses the `BeadCLI` from BeadCreator, which validates the binary at construction time via PATH lookup.
- **Probe**: At `NewBeadCLI` construction, run `<bin> close --help` to verify the `close` subcommand exists. If this fails, report the error with the binary version.
- **Exit code only**: Success is determined solely by exit code 0 vs non-zero. No JSON or stderr parsing — keeps the implementation portable across `br` and `bd` versions.
- **Minimum versions**: Same as BeadCreator — tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced.
- **No version parsing**: We probe behavior, not version strings.
