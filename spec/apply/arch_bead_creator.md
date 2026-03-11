# BeadCreator

Creates new beads via the bead CLI (`br` or `bd`) for spec nodes that don't yet have tracking beads. After creation, creates a mapping record in `.bead-map.json` and sets the bead label to the record ID.

## Responsibilities

- Read "create" actions from the impact report
- Construct bead create commands with the component description as bead description
- Execute bead creation and capture the new bead ID
- Create a mapping record in `.bead-map.json` linking the spec node to the bead
- Set the bead label to `spex:<record-id>`
- Return created bead IDs for subsequent tagging

## Interface

```go
type BeadCLI interface {
    Create(ctx context.Context, opts CreateOpts) (string, error)
    FindExisting(ctx context.Context, labels []string) (string, error)
}

func CreateBeads(ctx context.Context, cli BeadCLI, store map.Store, creates []Action) ([]string, error)
```

## Command Construction

For each create action:
```
<bin> create --title "<module>: <node_name>" \
  --type task \
  --silent
```

After creation, the mapping record is created and the bead label is set:
```
<bin> update <bead_id> --add-label spex:<record-id>
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`). The single `spex:<record-id>` label replaces the previous multi-label approach (`spec_module`, `spec_component`, `spec_hash`).

## Mapping Record Creation

After creating the bead, BeadCreator calls `store.Create()` with:
- `spec_node_id`: from the impact action (e.g., `"module/3/component/2"`)
- `bead_id`: from the bead CLI create output
- `module`: module name from the spec graph
- `component`: component/section name from the spec graph
- `content_file`: resolved content file path
- `spec_hash`: current merkle hash of the spec node

## Idempotency

Before creating, check if a bead with a matching `spex:` label already exists and is open. If so, verify the mapping record exists and return the existing bead ID.

## External Binary Compatibility

BeadCreator shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: At construction time, verify the binary exists on PATH. Fail with a clear error if missing: `"apply: bead CLI not found: <bin>"`.
- **Probe**: Run `<bin> create --dry-run --title probe --type task --labels probe --silent` once at construction. If this fails, the CLI flags are incompatible and we report the error with the binary version.
- **Minimum versions**: Tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced — only add one if a breaking change is discovered.
- **No version parsing**: We probe behavior, not version strings. This avoids brittleness from non-semver or pre-release versioning.
