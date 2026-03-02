# BeadCloser

Closes obsolete beads whose spec nodes have been removed.

## Responsibilities

- Read "close" actions from the impact report
- Construct `bd close` commands with descriptive reasons
- Execute bead closure

## Interface

```go
func CloseBeads(ctx context.Context, closes []Action) error
```

## bd Command Construction

For each close action:
```
bd close <bead_id> --reason "Spec node removed: <module>/<node_name>"
```

## Idempotency

If a bead is already closed, `bd close` should be a no-op. Check bead status before closing to avoid errors.

## Error Handling

If a bead ID from the impact report no longer exists in bd, log a warning but do not fail. The bead may have been manually closed between diff and apply.
