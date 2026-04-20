# BeadCreator

Creates new beads via the bead CLI (`br` or `bd`) with deterministic type assignment, parent hierarchy, lineage tracking, and priority propagation. After creation, creates or updates the mapping record in `.bead-map.json` and sets the bead label to the record ID.

## Responsibilities

- Read "create" actions from the impact report
- Create the proposal epic first on each apply run, reuse its bead ID as `--parent` for every subsequent create
- Determine bead type from spec node type (proposal→epic, component→feature, data_flow→task, test_section→task)
- Set `--parent <proposal-epic-bead-id>` for hierarchy
- Set `--deps blocks:<old-bead-id>` for lineage when replacing an obsoleted bead
- Set `--deps depends:<dep-bead-id>` for each spec-graph dependency from `DepBeadIDs`
- Set `--priority` derived from project requirement chain
- Execute bead creation and capture the new bead ID
- Create or update the mapping record in `.bead-map.json`
- Set the bead label to `spex:<record-id>`
- Return created bead IDs for the epic close-eligibility query

## Type Assignment Table

| Spec Node Type | Bead Type | Rationale |
|---------------|-----------|-----------|
| proposal | `epic` | One epic per apply run; groups every bead created in that run |
| component | `feature` | Each component is a distinct capability to build |
| data_flow | `task` | Cross-component contract; must land before participating component beads start |
| test_section (len(describes) >= 2) | `task` | Cross-component integration test that cannot be bundled into any single component bead |

The type is a pure function of the spec node type — no history queries needed. Note that test_sections with `len(describes) == 1` do not produce beads at all; they are bundled into the feature bead of the single component they describe, and the implement skill reads the test_section content as part of that component's TDD workflow.

## Proposal Epic Creation

At the start of each apply run with at least one create action:

1. BeadCreator issues a single `<bin> create --type epic --title "<proposal>" --priority <inherited>` call.
2. The returned bead ID is stored on the BeadCreator instance and reused as the `--parent` value for every subsequent create in the same run.
3. A bead-map record is written with `node_type = "proposal"`, `bead_type = "epic"`, `spec_node_id = <proposal-reference>`, `spec_hash = ""`, `module = ""`.
4. The epic bead is never obsoleted. When all of its children close, `br epic close-eligible <epic-id>` closes it.

Module identity plays no role in epic assignment. Modules are already addressable via labels and spec-graph queries; conflating them with epics (the previous design) added noise without adding value.

## Coupling Rule: Single-Component Test Sections

When an action references a `test_section` node, BeadCreator asserts before creation:

```
test_section = module_spec.find_test_section(action.SpecNodeID)
if len(test_section.describes) < 2:
    return error "single-component test_section reached BeadCreator — ActionClassifier should have filtered it"
```

This is a defense-in-depth check. The ActionClassifier upstream is the authoritative gate; BeadCreator asserts the invariant so any future bug is caught at the creation boundary rather than producing a stray bead.

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
