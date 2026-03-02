# BeadCreator

Creates new beads via the bd CLI for spec nodes that don't yet have tracking beads.

## Responsibilities

- Read "create" actions from the impact report
- Construct `bd create` commands with appropriate metadata
- Execute bead creation and capture the new bead ID
- Return created bead IDs for subsequent tagging

## Interface

```go
func CreateBeads(ctx context.Context, creates []Action) ([]string, error)
```

## bd Command Construction

For each create action:
```
bd create --title "<module>: <node_name>" \
  --type task \
  --metadata spec_module=<module> \
  --metadata spec_component=<component> \
  --metadata spec_hash=<hash>
```

The title follows the pattern `"<module>: <node_name>"` for consistency and searchability.

## Idempotency

Before creating, check if a bead with matching `spec_module` + `spec_component` (or `spec_impl_section`) already exists and is open. If so, skip creation and return the existing bead ID.
