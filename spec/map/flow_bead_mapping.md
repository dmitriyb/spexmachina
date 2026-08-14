# Bead Creation Mapping Flow

## Data Flow

Journal events are never written by a task-creating command inside `spex`. They are appended
through the store's writer-owner primitive — by `spex ingest` after an external adapter has
executed a changeset against the tracker, and by registration when a proposal's lifecycle opens.
The journal event id travels ahead of the pairing: plan derives each create op's eid from
`(git_head, op_id)` — the same derivation ingest will use to mint the event — and stamps it on
the op as the `spex:<eid>` idempotency label; a retarget op carries the same derivation's eid as
a plain label for the task it moves. The adapter applies the label to the task it creates or
updates, and ingest appends the change event and its receipt to the journal at baselining.

```dot
digraph bead_mapping {
    "spex diff"           [style=dashed];
    "spex plan"           [style=dashed];
    "changeset.json"      [style=dashed];
    "scripts/apply-br.sh" [style=dashed];
    "receipts.json"       [style=dashed];
    "spex ingest"         [style=dashed];
    ".history.jsonl"      [style=dashed];
    "205e67ca4aad"        [label="MappingStore\n205e67ca"];
    "08909d62930b"        [label="MapCommand\n08909d62"];

    "spex register"       [style=dashed];

    "spex register"       -> "205e67ca4aad"        [label="append registered event"];
    "spex diff"           -> "spex plan"           [label="diff document"];
    "205e67ca4aad"        -> "spex plan"           [label="fold + registration lookup, read only"];
    "spex plan"           -> "changeset.json"      [label="idempotency.label = spex:<eid>"];
    "changeset.json"      -> "scripts/apply-br.sh";
    "scripts/apply-br.sh" -> "receipts.json"       [label="br create --labels spex:<eid>"];
    "receipts.json"       -> "spex ingest"         [label="paired to its op by op_id"];
    "spex ingest"         -> "205e67ca4aad"        [label="append change events + receipts"];
    "205e67ca4aad"        -> ".history.jsonl"      [label="atomic append; parse and fold"];
    "08909d62930b"        -> "205e67ca4aad"        [label="spex map get / list"];
}
```

[[205e67ca4aad|MappingStore]] sits on both sides of the journal on purpose: it answers every read
and it owns the one append primitive every writer uses. Plan's parent resolution reads it before
the changeset exists — the fold for pairings, the parsed events for the run's registration; the
map query surface reads it after ingest has appended; `spex
ingest` and registration append through it. Everything between plan
and ingest is the label doing the travelling: `spex` itself never invokes a tracker CLI, so every
tracker mutation happens inside the adapter and every journal mutation happens inside the store.
Skills read the settled result through [[08909d62930b|MapCommand]] (`spex map get` / `list` /
`context`), a query face that never writes.

## Lifecycle

### Create (new spec node)

1. `spex plan` classifies the added node as a **create** action and labels the op `spex:<eid>`,
   deriving the eid from `(git_head, op_id)` exactly as ingest will mint the event — nothing is
   read, reserved or counted.
2. The adapter runs `br create --labels spex:<eid> --type <br-type> …`, where `<br-type>`
   is `spec_node_kind` mapped through the adapter's fixed table (`component` → `feature`,
   everything else task-shaped → `task`, `proposal_epic` → `epic`), and records the new task id in
   its receipt. The label is applied at create time, not by a follow-up update.
3. `spex ingest` appends the change event (`added`, with the op's node identity, leaf hashes and
   the changeset's `git_head`) and a `task_created` receipt pairing the event to the task id.
4. The snapshot is written in the same baselining step, only when the adapter reported
   `status = complete`; the journal append and the snapshot describe the same point-in-time state.

If the adapter reports `was_existing = true` (a task already carried the label, in any status),
the event id
— derived from `(git_head, op_id)` — decides which of two things happens. Usually the journal
already holds the pairing from the run that made it, the derived eid matches, and ingest appends no
duplicate. The recovery case is the opposite: an adapter that created the task last run and died
before writing its receipt leaves no pairing in the journal at all, so this run's event and receipt
are genuinely new — they land now, pairing the event with the task the dead run already made.

### Retarget (modified spec node, task still open)

1. `spex plan` classifies a modified node whose live pairing's task is open and unclaimed as a
   **retarget**: no bead is closed, none created — the task's target moves to the node's new
   state. The op carries `spex:<eid>` of this run's `modified` event in its `labels`, and the
   task's recomputed deps.
2. The adapter runs `br update` against the target task: it adds the new event label and any deps
   the task does not already carry. Updates are naturally idempotent, so no probe precedes them.
3. `spex ingest` appends the `modified` change event and a `task_retargeted` receipt pairing it
   to the existing task id. No `task_closed`, no `task_created`.
4. The fold — latest task-bearing event per node, `task_created` and `task_retargeted` alike —
   now answers with the same task id sourced from the newer event, and the context bracket for
   the task widens: `before_head` stays at the task's original birth, `after_head` moves to the
   latest retargeted event, so the implementer sees the whole accumulated change.

A modified node whose pairing is `in_progress` retargets nothing: `spex plan` refuses the run,
naming every claimed task whose node changed. A claimed task's target never moves under it.

### Update (modified spec node, task closed)

1. `spex plan` classifies a modified node whose pairing's task is closed as **obsolete +
   create**: the shipped task stays closed, a fresh task carries the new work.
2. The create op gets a fresh `spex:<eid>` label — the eid derives from this run's
   `git_head` and the op's id, so every change in the node's lineage carries its own label and
   the closed predecessor's label can never collide with it. Idempotency still needs no lookup
   and no cursor.
3. The adapter closes the old task and creates the new one.
4. `spex ingest` appends the `modified` change event, a `task_closed` receipt for the old task and
   a `task_created` receipt for the new one. Nothing is rebound: the old pairing stays in the
   journal as lineage, and the fold now answers with the new task.

Lineage is what makes this an update rather than an overwrite: every task the node has ever had
remains one journal-history read away — MappingStore's event-history interface, keyed by the same identity hash throughout. On the retarget path no `blocks` edge connects generations in the tracker, because there are no generations — the history is the journal's alone.

### Delete (removed spec node)

1. `spex plan` classifies a removed node as **obsolete** and produces a close op with reason
   `"Spec node removed: …"`.
2. The adapter closes the task.
3. `spex ingest` appends the `removed` change event and the `task_closed` receipt. No record is
   deleted, because none exists: the node's biography — name, type, module, removing proposal —
   is now carried by exactly the event just appended, which is what keeps the node resolvable
   after the spec forgets it.

If the removed node's task was already closed, plan additionally classifies a **cleanup**
create. A cleanup task carries `spex:<eid>` of the removal event it answers — the journal's
latest `removed` event for the node when the removal already landed, else the one its same-batch
close implies, derived from the close op's `(git_head, op_id)` — and its `task_created`
references that same event, so the cleanup task is born with working context instead of a
dangling reference: its task id resolves through the journal to the removed node's biography.

### Absorb (marked cosmetic modification)

A node the run's absorb list marks produces no op and no tracker mutation at all. Its `modified`
event still reaches the journal: ingest mints it from the changeset's `absorbed` array — eid
derived from `(node, before, after)`, exactly as refresh-born events are — and closes it with the
existing `refresh` receipt naming it. The fold is untouched by design: a `refresh` receipt is not
task-bearing, so the node's pairing keeps its sourcing event and the task owes nothing.

### Proposal epics

An epic create is synthesized by plan, not born from a diff entry. Its referent is the
`registered` event registration appended when the proposal's lifecycle opened: plan reads that
event's eid (`<git_head>:<slug>`) from the parsed journal, labels the op `spex:<eid>`, and the
epic's `task_created` references the registered event through `for` like every other receipt.
The read is on the events, not on the fold, because until that receipt lands the registration
pairs with no task and the fold — a list of task-bearing pairings — carries nothing for it. Once
it lands, the fold lists the epic keyed by the slug the registered event carries. Legacy epic receipts that carry
`proposal: <slug>` in place of `for` remain readable behind the fold's read-only legacy branch.

## Invariants

Enforced by ingest before the baselining commit, and re-checked on every subsequent run:

- Every ok create pairs exactly one `task_created` receipt with exactly one referent event — the
  change event for node-bearing creates, the implied `removed` event for cleanup creates, the
  `registered` event for epic creates. Every ok retarget pairs exactly one `task_retargeted`
  receipt with its own `modified` event.
- Every receipt references an existing event — a receipt referencing nothing is refused
  before it is appended. The retired "or names a proposal" branch survives only as inert legacy
  lines behind the fold's read-only legacy branch.
- Re-running ingest on the same changeset+receipts pair appends nothing: event ids derive from
  `(git_head, op_id)` and receipts pair by the same key.
- The snapshot is saved only when receipts are complete — SnapshotSaver's gate, not Reconciler's —
  so the journal and the snapshot always describe the same point-in-time spec state.
- Every appended line validates against the journal-line schema before the write happens.

A failure at any step leaves the on-disk journal untouched — the append is a whole-file
write-and-rename, so a refused batch changes nothing.

## Data Shapes

### plan → changeset op (the label carrier)

- `idempotency.label`: string, on creates — `spex:<eid>` of the journal event the op's
  `task_created` will reference: the change event for fresh and modify-pair creates, the removal
  event the cleanup answers (prior-batch or same-batch) for cleanup creates, the `registered`
  event for epic creates. One rule, no shape branching. On a retarget the same derivation rides
  in `labels` instead, because an update needs no probe. This is the only channel by which an
  event's identity reaches the tracker.
- `spec_node_kind`: string enum, on creates — `proposal_epic` | `component` | `data_flow` |
  `test_section` | `cleanup`. Ingest branches its event construction on this value; retarget ops
  carry the op type itself as the discriminator.
- `spec_node_id`: string — 12-char hex identity hash, or the proposal reference on `proposal_epic`
  ops.

### adapter → receipt entry

- `op_id`: string — pairs the receipt back to its op, and (with `git_head`) derives the event id.
- `status`: string enum — `ok` | `skipped` | `error`. Only `ok` produces journal receipts;
  `skipped` and `error` are counted and append nothing.
- `bead_id`: string — the tracker id created, updated, or the pre-existing one.
- `was_existing`: boolean — true when the idempotency label already matched a task, in any
  status. Not applicable to retargets.

### journal line shapes (spec/.history.jsonl)

- Change event: `event` (`added`|`removed`|`modified`), `eid`, `node`, `name`, `node_type`,
  `module`, `before`, `after`, `git_head`, `proposal`, and an optional `path` — the node's
  content-leaf path, present when the node has one.
- Registered event: `event` (`registered`), `eid` (`<git_head>:<slug>`), `proposal`, `git_head` —
  no `node`, because registration precedes any spec change.
- Task receipt: `event` (`task_created`|`task_closed`|`task_retargeted`), `for` (an `eid`),
  `task_id`. A `proposal` slug in place of `for` is a legacy pre-migration shape, read but never
  appended anew.
- Refresh receipt: `event` (`refresh`), `git_head` (or recorded absence), `absorbed` (a list of
  `eid`s) — closing refresh-born and absorb-born `modified` events alike.

No field renames, additions, or removals of these shapes without updating every consumer: plan's
IdempotencyLabeler and Resolver, the reference adapter `scripts/apply-br.sh`, ingest's Reconciler,
and the `spex map` CLI surface.
