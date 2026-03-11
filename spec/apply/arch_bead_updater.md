# BeadUpdater

Updates metadata on beads whose spec nodes were modified. Updates the corresponding mapping record in `.bead-map.json` with the new spec hash.

## Responsibilities

- Read "review" actions from the impact report
- Update `spec_hash` in the bead's `spex:` label metadata
- Update the corresponding mapping record's `spec_hash` in `.bead-map.json`

## Interface

Reuses the `BeadCLI` interface from BeadCreator, extended with an `Update` method:

```go
type BeadCLI interface {
    Create(ctx context.Context, opts CreateOpts) (string, error)
    FindExisting(ctx context.Context, labels []string) (string, error)
    Close(ctx context.Context, id string, reason string) error
    Update(ctx context.Context, id string, metadata map[string]string) error
}

func UpdateBeads(ctx context.Context, cli BeadCLI, store map.Store, reviews []Action, logger *slog.Logger) error
```

## Command Construction

For each review action:
```
<bin> update <bead_id> --add-label spec_hash:<new_hash>
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`).

## Mapping Record Update

After updating the bead, BeadUpdater calls `store.Update(recordID, {"spec_hash": newHash})` to update the mapping record. The record ID is obtained from the bead's `spex:<record-id>` label.

## What Gets Updated

Only `spec_hash` is updated automatically — both on the bead label and in the mapping record. The bead's title, description, and other metadata remain unchanged — the review action signals that a human should review the bead in context of the spec change, not that the bead should be automatically modified.

## Error Handling

Update errors are logged as warnings but do not abort the batch. This covers:

- Bead ID no longer exists (manually closed between diff and apply)
- Bead already has the target hash (idempotent — `update` overwrites metadata)

If the bead update succeeds but the mapping record update fails, the error is logged as a warning. The stale record will be detected by `spex check` on the next preflight.

Only binary-not-found errors (from `NewBeadCLI` construction) are fatal.

`UpdateBeads` returns a summary error aggregating all warnings, or nil if all succeeded.

## External Binary Compatibility

BeadUpdater shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: Reuses the `BeadCLI` from BeadCreator, which validates the binary at construction time via PATH lookup.
- **Probe**: At `NewBeadCLI` construction, run `<bin> update --help` to verify the `update` subcommand exists. If this fails, report the error with the binary version.
- **Exit code only**: Success is determined solely by exit code 0 vs non-zero. No JSON or stderr parsing — keeps the implementation portable across `br` and `bd` versions.
- **Minimum versions**: Same as BeadCreator — tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced.
- **No version parsing**: We probe behavior, not version strings.
