# Adapter flow

```
   changeset.json (stdin or $1)
         │
         ▼
  ┌────────────────────────┐
  │ 1. Pre-flight          │
  │  - parse v1            │
  │  - br --version check  │
  │  - SUB_TABLE init      │
  └─────────┬──────────────┘
            │
            ▼
  ┌────────────────────────┐
  │ 2. Iterate ops in order│
  │  for each op:          │
  │   ┌──────────────────┐ │
  │   │ resolve refs     │ │
  │   │ (parent, deps,   │ │
  │   │  target)         │ │
  │   └────────┬─────────┘ │
  │            ▼           │
  │   ┌──────────────────┐ │
  │   │ idempotency check│ │
  │   │ via br list/show │ │
  │   └────────┬─────────┘ │
  │            ▼           │
  │   ┌──────────────────┐ │
  │   │ br create/update/│ │
  │   │ close            │ │
  │   └────────┬─────────┘ │
  │            ▼           │
  │   ┌──────────────────┐ │
  │   │ SUB_TABLE update │ │
  │   │ append receipt   │ │
  │   └──────────────────┘ │
  └─────────┬──────────────┘
            │
            ▼
  ┌────────────────────────┐
  │ 3. Emit receipts.json  │
  │  - determine top status│
  │  - assemble v1 JSON    │
  │  - atomic write        │
  └─────────┬──────────────┘
            │
            ▼
     receipts.json (stdout or $2)
```

## Subprocess Shape

The adapter forks:

- `br list --json` once per create op (idempotency check). Could be optimized by caching a single initial listing, but the reference impl takes the simple path.
- `br show <id> --json` once per close op (idempotency check).
- `br create` once per non-existing create.
- `br update` once per label-add on close.
- `br close` once per non-existing close.

For a changeset with N creates (M fresh) and K closes (J fresh), the adapter makes approximately `N + K + M + 2J` br invocations. On a 10-op changeset, ~10-20 invocations.

## Data Shapes

### Input: changeset.json

See `spec/emit/arch_changeset_builder.md` for the full shape. Adapter consumes: `version`, `git_head`, `proposal`, `ops[].{op_id, type, spec_node_kind, spec_node_id, idempotency.label, parent, deps, priority, title, body, target, labels, reason}`.

### Output: receipts.json

```json
{
  "version": 1,
  "status": "complete",
  "ops": [
    {"op_id":"op-0001","status":"ok","bead_id":"spexmachina-abc","was_existing":false},
    {"op_id":"op-0002","status":"ok","bead_id":"spexmachina-def","was_existing":false},
    {"op_id":"op-0003","status":"skipped","bead_id":"spexmachina-ghi","was_existing":true,"reason":"idempotent re-match"},
    {"op_id":"op-0004","status":"error","bead_id":"","was_existing":false,"error":"br create exited 1: invalid priority -1"}
  ]
}
```

## Error Paths

- Changeset parse error → exit 1, no receipts file written.
- br pre-flight fails → exit 1, no receipts file written.
- br operation errors mid-run → receipts.json gets top-level `partial` status; adapter continues processing remaining ops.
- Unresolvable ref (op_id not in SUB_TABLE) → that op's receipt is `error`; subsequent ops still processed; top-level → `partial`.

## Success Paths

- All ops ok or intentional-skipped → top-level `complete`; receipts.json written; exit 0.
- Empty changeset (no ops) → top-level `complete`, empty `ops` array, exit 0. (Well-formed edge case.)

## Integration with Ingest

The adapter's output is ingest's input. The contract is tight:

- Every op in the changeset has exactly one receipt entry.
- Op ordering in receipts.ops mirrors changeset.ops.
- Status values align with ingest's expected transition table.

Ingest trusts these properties; violations cause ingest to exit 1 with pre-flight errors.
