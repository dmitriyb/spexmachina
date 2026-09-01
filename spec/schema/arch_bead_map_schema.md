# BeadMapSchema

The journal-line JSON Schema definition. It validates each line of the task journal — the
append-only log linking spec nodes to tasks, living in the `.spex/` state directory the lifecycle
pre-flight resolves; the line format is the contract and is indifferent to the file's location —
and those line shapes are the whole of
[[f7ef8bef0ba1|what the journal may hold]]: change events, the registered event, receipt
events, and nothing else.

## Scope

Defines the JSON Schema for one journal line, covering:

- **Change events**: `event` drawn from `added` / `removed` / `modified`, plus `eid`, `node`,
  `name`, `node_type`, `module`, `before`, `after`, `git_head`, `proposal`, and an optional
  `path` — the node's content-leaf path, present when the node has one. `before` and `after`
  admit `null` — an add has no before, a removal no after.
- **Registered event**: `event: "registered"`, plus `eid` (of the form `<git_head>:<slug>`),
  `proposal` (the slug), and `git_head`. No `node` field — registration opens a proposal's
  lifecycle before any spec change exists, and the epic's `task_created` references this event.
- **Task receipts**: `event` drawn from `task_created` / `task_closed` / `task_retargeted`, plus
  `task_id` and a `for` (an `eid`). `task_retargeted` records a task whose target moved to a new
  state without lifecycle: its `for` references the retarget's own `modified` event, and the fold
  treats it as task-bearing exactly as it treats `task_created`. A `task_created` carrying a
  `proposal` slug instead of `for` remains admitted as a legacy shape: pre-migration epic lines
  validate as inert history, but no new append uses it.
- **Refresh receipts**: `event: "refresh"`, `git_head` (admitting recorded absence), `absorbed`
  (an array of `eid`s). One shape serves both producers: whole-run refresh batches and the
  per-node absorptions a normal run's changeset carries in its `absorbed` array close their
  `modified` events with the same receipt.
- **Format constraints**: `node` is constrained to the 12-character identity-hash pattern; a
  *legacy* epic receipt's `proposal` carries the slug-shaped reference instead — current epic
  receipts reference the registered event through `for` like every other receipt. `node_type` is the closed
  enum of node kinds a change event may describe — the task-owning kinds `component`,
  `data_flow`, `test_section`, plus `requirement` and `api`, which only refresh-born events
  carry — and each line is a self-contained object: there is no envelope, no counter, and no
  integer id anywhere in the format. Every line shape also admits an optional integer `v` — the
  journal-line [[b1baa51bd7a9|format version]], metadata outside any hashed payload: absent
  means version 1, so no existing line changes meaning; a writer stamps the current version, 1;
  and the schema pins no upper bound, because the journal is append-only and permanent, so
  readers accept every version from 1 forward, forever.

## Design Notes

### Single key format across the pipeline

A change event's `node` is that node's identity hash. The merkle tree keys its leaves by the same
identity hash, so plan's node matching joins a changed merkle node against the journal fold with no
key translation anywhere. Tracker labels carry the *event* id instead (`spex:<eid>`) — an
adapter-facing idempotency key spex reads nothing from, so no label ever needs parsing back into a
node key. Earlier formats used `<module>/<node_type>/<integer_id>` keys
that required rekeying between merkle and the retired bead-map; that translation layer died when
identity hashes were introduced, and the journal keeps the single-format property.

### `node_type` is the closed set of things an event may describe

The enum is the schema's own statement of what a change event may describe: the task-owning kinds
(component, data_flow, test_section) plus the refresh-absorbable ones (requirement, api) —
a proposal is never a change event's subject; it appears as the slug on the `registered` event
that opens its lifecycle. Task receipts pair with events of task-owning kinds and, for epics,
with the registered event; requirement and api events exist so refresh absorption is on the
record, never to mint tasks. A
kind of spec content that exists only as part of a component's contract has no entry, because it
never owns a task of its own — it reaches the tracker inside the task of the component it belongs
to.

Stating that in the schema rather than leaving it implied is what makes the boundary fail closed:
ingest validates every line against this schema before the append commits, so a batch that somehow
constructed an event for an untaskable kind is rejected at the write boundary, leaving the journal
untouched. The corollary is that retiring a kind of spec content that was never in the enum is not
a journal migration: no line named one, so no line has to be rewritten.

### Per-line validation, not per-file

Each line validates independently. That is what lets the map query surface name the exact line
that violates the schema, and what lets a gating reader treat one bad line as degradation rather
than a poisoned file — semantics the retired whole-file envelope could not express.

### No cross-line constraints in schema

The schema does not enforce that a receipt's `for` references an existing `eid`, that event ids
are unique, or that a node's events are ordered sensibly. JSON Schema cannot express cross-line
constraints over a JSONL stream. These are enforced programmatically by ingest's invariants at
append time and surfaced by the fold at read time.
