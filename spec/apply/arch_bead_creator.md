# BeadCreator

Creates new beads via the bead CLI (`br` or `bd`) for spec nodes that don't yet have tracking beads.

## Responsibilities

- Read "create" actions from the impact report
- Construct bead create commands with spec-tracking labels
- Execute bead creation and capture the new bead ID
- Return created bead IDs for subsequent tagging

## Interface

```go
type BeadCLI interface {
    Create(ctx context.Context, opts CreateOpts) (string, error)
    FindExisting(ctx context.Context, labels []string) (string, error)
}

func CreateBeads(ctx context.Context, cli BeadCLI, creates []Action) ([]string, error)
```

## Command Construction

For each create action:
```
<bin> create --title "<module>: <node_name>" \
  --type task \
  --labels spec_module:<module>,spec_component:<component>,spec_hash:<hash> \
  --silent
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`). Spec metadata is encoded as labels with a `spec_` prefix and colon separator.

The title follows the pattern `"<module>: <node_name>"` for consistency and searchability.

## Idempotency

Before creating, check if a bead with matching `spec_module:<module>` + `spec_component:<component>` (or `spec_impl_section:<section>`) labels already exists and is open. If so, skip creation and return the existing bead ID.

## External Binary Compatibility

BeadCreator shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: At construction time, verify the binary exists on PATH. Fail with a clear error if missing: `"apply: bead CLI not found: <bin>"`.
- **Probe**: Run `<bin> create --dry-run --title probe --type task --labels probe --silent` once at construction. If this fails, the CLI flags are incompatible and we report the error with the binary version.
- **Minimum versions**: Tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced — only add one if a breaking change is discovered.
- **No version parsing**: We probe behavior, not version strings. This avoids brittleness from non-semver or pre-release versioning.
