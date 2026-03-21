# BeadCloser

Obsoletes beads via bead CLI (`br` or `bd`) with `spex:obsolete` label and `commit:<HEAD>` label. Uniform path for both modified and removed spec nodes.

## Responsibilities

- Read "obsolete" actions from the impact report
- **Label phase**: Add `spex:obsolete` and `commit:<HEAD>` labels to beads, but keep them open. For removed nodes, delete the corresponding mapping record from `.bead-map.json`. For modified nodes, leave the mapping record (BeadCreator will update it with the new bead).
- **Close phase**: Close the labeled beads after BeadCreator has created replacements. This two-phase approach ensures `br` auto-flush correctly persists all bead states to the JSONL.

## Interface

Reuses the `BeadCLI` interface from BeadCreator, extended with a `Close` method:

```go
type BeadCLI interface {
    Create(ctx context.Context, opts CreateOpts) (string, error)
    FindExisting(ctx context.Context, labels []string) (string, error)
    Close(ctx context.Context, id string, labels []string) error
}

func CloseBeads(ctx context.Context, cli BeadCLI, store map.Store, obsoletes []Action, logger *slog.Logger) error
```

## Command Construction

**Label phase** (before creates):
```
<bin> update <bead_id> --add-label spex:obsolete --add-label commit:<HEAD>
```

**Close phase** (after creates):
```
<bin> close <bead_id>
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`). The `commit:<HEAD>` label stamps the last commit where the bead's spec was valid. Active beads carry no commit label — the commit label only appears at the moment of obsolescence. The label is applied before close so the bead is marked as obsolete while still open, giving `br` auto-flush a clean state transition.

## Mapping Record Handling

- **Removed nodes**: After closing the bead, call `store.Delete(recordID)` to remove the mapping record.
- **Modified nodes**: Leave the mapping record intact — BeadCreator will update it with the new bead ID and spec hash.

The distinction is determined by the action's `change_type` field: `"removed"` triggers deletion, `"modified"` does not.

## Lineage

History is a dependency chain walk in the bead CLI: bead-v3 → bead-v2 → bead-v1. Each obsoleted bead's `commit:<hash>` label provides a precise pointer for historical lookup: `git show <that-commit>:spec/...` recovers the exact spec state.

## Idempotency & Error Handling

Close errors are logged as warnings but do not abort the batch. This covers:

- Bead already closed (idempotent)
- Bead ID no longer exists (manually closed between diff and apply)

`CloseBeads` returns a summary error aggregating all warnings, or nil if all succeeded.

## External Binary Compatibility

BeadCloser shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: Reuses the `BeadCLI` from BeadCreator, which validates the binary at construction time via PATH lookup.
- **Probe**: At `NewBeadCLI` construction, run `<bin> close --help` to verify the `close` subcommand exists. If this fails, report the error with the binary version.
- **Exit code only**: Success is determined solely by exit code 0 vs non-zero. No JSON or stderr parsing — keeps the implementation portable across `br` and `bd` versions.
- **Minimum versions**: Same as BeadCreator — tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced.
- **No version parsing**: We probe behavior, not version strings.
