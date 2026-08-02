# BeadMapSchema

The journal-line JSON Schema definition. It validates each line of `spec/.history.jsonl` — the
task journal linking spec nodes to tasks — and those line shapes are the whole of
[[f7ef8bef0ba1|what the journal may hold]]: change events, receipt events, and nothing else.

## Scope

Defines the JSON Schema for one journal line, covering:

- **Change events**: `event` drawn from `added` / `removed` / `modified`, plus `eid`, `node`,
  `name`, `node_type`, `module`, `before`, `after`, `git_head`, `proposal`, and an optional
  `path` — the node's content-leaf path, present when the node has one. `before` and `after`
  admit `null` — an add has no before, a removal no after.
- **Task receipts**: `event` drawn from `task_created` / `task_closed`, plus `task_id` and exactly
  one of `for` (an `eid`) or `proposal` (a slug — the epic case).
- **Refresh receipts**: `event: "refresh"`, `git_head` (admitting recorded absence), `absorbed`
  (an array of `eid`s).
- **Format constraints**: `node` is constrained to the 12-character identity-hash pattern; an
  epic receipt's `proposal` carries the slug-shaped reference instead. `node_type` is the closed
  enum of node kinds a change event may describe — the task-owning kinds `component`,
  `data_flow`, `test_section`, plus `requirement` and `api`, which only refresh-born events
  carry — and each line is a self-contained object: there is no envelope, no counter, and no integer id anywhere
  in the format.

## Design Notes

### Single key format across the pipeline

A change event's `node` is that node's identity hash. The merkle tree keys its leaves by the same
identity hash, and the tracker labels carry it too (`spex:<spec_node_id>`). This means impact
analysis joins a changed merkle node against the journal fold — and against parsed bead labels —
with no key translation anywhere. Earlier formats used `<module>/<node_type>/<integer_id>` keys
that required rekeying between merkle and the retired bead-map; that translation layer died when
identity hashes were introduced, and the journal keeps the single-format property.

### `node_type` is the closed set of things an event may describe

The enum is the schema's own statement of what a change event may describe: the task-owning kinds
(component, data_flow, test_section) plus the refresh-absorbable ones (requirement, api) —
proposals appear only in receipts, as slugs. Task receipts pair only with events of task-owning
kinds; requirement and api events exist so refresh absorption is on the record, never to mint
tasks. A
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
