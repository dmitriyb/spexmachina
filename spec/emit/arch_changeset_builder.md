# ChangesetBuilder

Composes `changeset.json` v1 from the impact report, the spec graph, the mapping store, and a caller-supplied git HEAD SHA. Delegates dep resolution to `Resolver`, ordering to `TopologicalSorter`, and idempotency label assignment to `IdempotencyLabeler`.

## Responsibilities

- Load impact report actions (create, obsolete/close) and classify them by spec node type tier (proposal epic / feature+data_flow task / multi-component test task).
- Resolve each create action's parent and deps via `Resolver` into the three ref shapes.
- Order the create ops via `TopologicalSorter` so in-batch deps come before dependents.
- Assign idempotency labels via `IdempotencyLabeler`.
- Emit close ops with the obsolete labels (`spex:obsolete`, `commit:<git_head>`).
- Write the final v1 changeset with canonical field order and stable key ordering inside every nested object.

## Interface

```go
type Builder struct {
    SpecGraph   *spec.Graph
    MappingStore mapping.Store
    GitHead     string
    Proposal    string
}

func (b *Builder) Build(report impact.Report) (changeset.Changeset, error)
```

`changeset.Changeset` is a plain value type mirroring the JSON schema:

```go
type Changeset struct {
    Version int        `json:"version"`     // always 1
    GitHead string     `json:"git_head"`
    Proposal string    `json:"proposal"`
    Ops     []Op       `json:"ops"`
}
```

## Op Kinds

Four op kinds flow through the builder:

| Op type | Emitted for | Adapter action |
|---------|-------------|----------------|
| `create` | new or replacement spec node | create a bead in the tracker |
| `close`  | obsoleted bead (old modified or removed) | close the bead with labels |
| `label`  | standalone label additions | add labels without create/close |
| `tag`    | proposal tagging on existing beads | add `spec_proposal:<ref>` label |

The current proposal produces mostly create and close ops plus the initial proposal-epic create. Label and tag are reserved for future flows (e.g., cross-proposal tagging).

## Op Shape

```json
{
  "op_id": "op-0001",
  "type": "create",
  "spec_node_kind": "component",
  "spec_node_id": "7f06f7d80e94",
  "idempotency": { "label": "spex:142" },
  "parent": { "ref": "op", "op_id": "op-0000" },
  "deps": [
    { "ref": "bead", "bead_id": "spexmachina-ab1" },
    { "ref": "op", "op_id": "op-0003" }
  ],
  "priority": 1,
  "title": "emit: ChangesetBuilder",
  "body": "…"
}
```

Close ops omit `deps`/`parent` and add `target` + `labels`:

```json
{
  "op_id": "op-0042",
  "type": "close",
  "target": { "ref": "bead", "bead_id": "spexmachina-tjs" },
  "labels": ["spex:obsolete", "commit:deadbeefcafe1234"],
  "reason": "Spec node modified: apply/ApplyCommand"
}
```

## Canonical Output

- Field order inside every op is fixed (op_id, type, spec_node_kind, spec_node_id, idempotency, parent, deps, priority, title, body / target, labels, reason).
- `ops` array preserves the order produced by TopologicalSorter — never re-sorted at write time.
- JSON is indented 2 spaces, LF-only newlines, no trailing whitespace.

## Error Handling

- Any error from Resolver or TopologicalSorter aborts the build; no partial changeset is ever produced.
- Missing `git_head` is checked by EmitCommand before Builder runs.
- An impact report with top-level errors is rejected upstream (EmitCommand); Builder assumes a clean report.
