# BeadUpdater

Updates metadata on beads whose spec nodes were modified.

## Responsibilities

- Read "review" actions from the impact report
- Update `spec_hash` metadata to reflect the new spec content hash
- Optionally add a comment noting the spec change

## Interface

```go
func UpdateBeads(ctx context.Context, reviews []Action) error
```

## bd Command Construction

For each review action:
```
bd update <bead_id> --metadata spec_hash=<new_hash>
```

## What Gets Updated

Only `spec_hash` is updated automatically. The bead's title, description, and other metadata remain unchanged — the review action signals that a human should review the bead in context of the spec change, not that the bead should be automatically modified.
