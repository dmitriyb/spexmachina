# BeadCreator

Creates new beads via the bead CLI (`br` or `bd`) with deterministic type assignment, parent hierarchy, lineage tracking, and priority propagation. After creation, creates or updates the mapping record in `.bead-map.json` and sets the bead label to the record ID.

## Responsibilities

- Read "create" actions from the impact report
- Determine bead type from spec node type (module→epic, component→feature, test_section→task)
- Set `--parent` to establish hierarchy (component under module epic, test under component feature)
- Set `--deps blocks:<old-bead-id>` for lineage when replacing an obsoleted bead
- Set `--deps depends:<dep-bead-id>` for each spec-graph dependency from `DepBeadIDs`
- Set `--priority` derived from project requirement chain
- Execute bead creation and capture the new bead ID
- Create or update the mapping record in `.bead-map.json`
- Set the bead label to `spex:<record-id>`
- Return created bead IDs for subsequent tagging

## Type Assignment Table

| Spec Node Type | Bead Type | Rationale |
|---------------|-----------|-----------|
| module | `epic` | Grouping container for a module's work |
| component | `feature` | Each component is a distinct capability to build |
| test_section | `task` | Distinct verification effort |

The type is a pure function of the spec node type — no history queries needed.

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
  --type <type_from_table> \
  --parent <parent-bead-id> \
  --deps blocks:<old-bead-id> \
  --deps depends:<dep-bead-id-1> \
  --deps depends:<dep-bead-id-2> \
  --priority <priority> \
  --silent
```

The `--deps blocks:` flag is for lineage (replacing an obsoleted bead). The `--deps depends:` flags are for spec-graph dependencies resolved from `uses` and `requires_module` edges. Multiple `--deps` flags are supported by `br create`. The `depends` relationship type means "don't start this until that is done."

After creation, the mapping record is created/updated and the bead label is set:
```
<bin> update <bead_id> --add-label spex:<record-id>
```

Where `<bin>` is the configured bead CLI binary (`br` or `bd`).

## Priority Derivation

```
component.implements[] → module requirement.preq_id → project requirement.priority
bead_priority = min(all project requirement priorities in that set)
```

The lowest priority number (highest urgency) wins. Passed via `--priority` on bead creation.

## Mapping Record Maintenance

For new nodes: call `store.Create()` with all fields including `bead_type`.

For modified nodes (replacing an obsoleted bead): call `store.Update()` on the existing record, setting new `bead_id`, `spec_hash`.

## Cleanup Bead Handling

When the create action is a cleanup (reason starts with "Code cleanup:"), the bead is created differently:

- Title: `"Code cleanup: <ComponentName>"`
- No mapping record is created (the component no longer exists in the spec)
- Label: `spex:cleanup` (not `spex:<record-id>`)
- `--deps blocks:<old-bead-id>` for lineage tracking to the obsoleted bead
- Type: `task` (cleanup is a discrete work item)

## Idempotency

Before creating, check if a bead with a matching `spex:` label already exists and is open. If so, verify the mapping record exists and return the existing bead ID.

## External Binary Compatibility

BeadCreator shells out to `br` or `bd` — both are external binaries outside our control. Strategy:

- **Detection**: At construction time, verify the binary exists on PATH. Fail with a clear error if missing: `"apply: bead CLI not found: <bin>"`.
- **Probe**: Run `<bin> create --dry-run --title probe --type task --silent` once at construction. If this fails, the CLI flags are incompatible and we report the error with the binary version.
- **Minimum versions**: Tested with `br >= 0.1.20`, `bd >= 0.56.1`. No upper bound enforced — only add one if a breaking change is discovered.
- **No version parsing**: We probe behavior, not version strings. This avoids brittleness from non-semver or pre-release versioning.
