# ChangesetBuilder

Composes `changeset.json` v1 from the impact report, the spec graph, the mapping store, and a caller-supplied git HEAD SHA. Delegates dep resolution to `Resolver`, ordering to `TopologicalSorter`, and idempotency label assignment to `IdempotencyLabeler`.

## Responsibilities

- Load impact report actions (create, obsolete/close) and classify them by spec node type tier (proposal epic / feature+data_flow task / multi-component test task).
- **Detect cleanup actions** by `Action.Reason` prefix `"Code cleanup:"`. Cleanup actions get a distinct op shape — see "Cleanup op shape" below.
- Resolve each create action's parent and deps via `Resolver` into the three ref shapes.
- Order the create ops via `TopologicalSorter` so in-batch deps come before dependents.
- **Assign idempotency labels via `IdempotencyLabeler.LabelFor(action)` — per-action lookup, NOT a flat `Reserve(N)` slice**. The Labeler branches on action class (modify-pair vs cleanup vs fresh) and returns the appropriate label format; see `arch_idempotency_labeler.md` for the rules.
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

### `spec_node_kind` vocabulary

Create ops carry `spec_node_kind` matching the underlying spec node category:

| Kind | When emitted | Bead-type via adapter mapping |
|------|--------------|-------------------------------|
| `proposal_epic` | First op of a fresh proposal — represents the proposal as an epic bead | `epic` |
| `component`     | New or replacement component arch leaf | `feature` |
| `data_flow`     | New or replacement data_flow leaf | `task` |
| `test_section`  | New or replacement test_section leaf with `describes >= 2` | `task` |
| `cleanup`       | Code-cleanup bead for a removed spec node — see "Cleanup op shape" below | `task` |

The `cleanup` value is what tells the adapter and Reconciler that this op produces a bead with no mapping record (per CLAUDE.md "no map record" rule).

## Cleanup op shape

Cleanup actions (those whose `Action.Reason` starts with `"Code cleanup:"`) are emitted with a distinct op shape rather than the conventional component/data_flow form. The discriminator is the same `isCleanup(a)` test pre-decouple `apply/bead_creator.go` used; emit lifts it from the implementation layer up into the changeset contract:

| Field             | Cleanup op value                                                                            |
|-------------------|---------------------------------------------------------------------------------------------|
| `type`            | `"create"`                                                                                  |
| `spec_node_kind`  | `"cleanup"` (NOT the underlying spec node's kind — adapter and Reconciler key off this)     |
| `spec_node_id`    | `action.SpecNodeID` (identity hash of the now-removed spec node — for traceability)         |
| `idempotency.label` | `"spex:cleanup-<action.SpecNodeID>"` per Labeler's cleanup branch                         |
| `parent`          | proposal-epic ref (same as other creates)                                                   |
| `deps`            | `[{"ref":"bead","bead_id":"<action.OldBeadID>","type":"blocks"}]` lineage                   |
| `priority`        | `3` (`emit.FallbackPriority`)                                                               |
| `title`           | `action.Reason` verbatim (e.g. `"Code cleanup: m/X"`) — NOT `"<module>: <node>"`             |
| `labels`          | `["spex:cleanup"]` — emit populates `Op.Labels` on cleanup creates so the adapter can apply the discriminator label via `--add-label` on `br create`. The `Labels` field already exists on `Op` (used on close ops); cleanup is the first create-class user. |
| `body`            | empty                                                                                       |

Modify-pair create ops (those with `OldBeadID != ""` but NOT cleanup) keep the conventional shape; only `idempotency.label` changes, reused from the existing record's id rather than the cursor.

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
