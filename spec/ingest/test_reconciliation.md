# Reconciliation tests

Integration tests for `Reconciler.Apply` against fixture changesets and receipts.

## Setup

- In-code Go fixtures, no on-disk testdata: helpers in `ingest/reconciler_test.go` build each
  changeset/receipts pair and seed a temporary journal.
- Tests write an initial `spec/.history.jsonl`, run reconciliation, and assert the appended lines
  match expected — by parsing, never by byte comparison, since field order inside a line is the
  encoder's business.

## Scenarios

### Ok create → event and receipt appended

- Changeset: one create op, spec_node_id `abc123def456`, idempotency label `spex:abc123def456`,
  git_head `cafe1234`.
- Receipts: corresponding op receipt with `status: "ok"`, `bead_id: "br-new"`, `was_existing: false`.
- Expected: the journal gains an `added` change event for node `abc123def456` (eid derived from
  `(cafe1234, <op_id>)`) and a `task_created` receipt with `for` naming that eid and
  `task_id: "br-new"`.

### Ok close on removed → removed event and task_closed appended

- Initial journal: `added` + `task_created` (task `br-old`) for node `beadbead0001`.
- Changeset: one close op targeting bead `br-old`, `reason` starting with "Spec node removed".
- Receipts: op receipt `status: "ok"`.
- Expected: a `removed` change event for `beadbead0001` carrying its name, node_type and module — the
  biography that outlives the node — plus a `task_closed` receipt for `br-old`. The earlier
  pairing lines remain untouched: nothing is ever deleted.

### Modified node: create+close → lineage extended, not rebound

- Initial journal: `added` + `task_created` (task `br-old`) for node `beadbead0002`.
- Changeset: create op for `beadbead0002` with label `spex:beadbead0002` and a `blocks` dep on
  `br-old`, then close op for `br-old` — emit's real ordering (creates before closes).
- Receipts: both ok; create's `bead_id: "br-new"`, `was_existing: false`.
- Expected: a `modified` change event for `beadbead0002`, a `task_closed` for `br-old`, a `task_created`
  for `br-new` — all built while processing the create, off its `blocks` dep, without waiting for
  the close. The fold now answers `beadbead0002 → br-new`; the `br-old` pairing remains as lineage.

### Was_existing=true → idempotent no-op

- Initial journal already holds the change event and `task_created` for node `A`, task `br-7`.
- Changeset: the same create op re-emitted (same git_head, same op_id).
- Receipts: create op `status: "ok"`, `bead_id: "br-7"`, `was_existing: true`.
- Expected: nothing appended — the derived eid already exists, and the receipt pairs with it.
  No error.

### Error status → op skipped, nothing appended

- Changeset: one create op.
- Receipts: that op with `status: "error"`, `bead_id: ""`.
- Expected: no event, no receipt; no error from the reconciler (the adapter's failure is the
  user's problem, ingest records truth — and the truth is that nothing happened).

### Skipped status → op no-op

- Changeset: create op.
- Receipts: op with `status: "skipped"`.
- Expected: nothing appended.

### Mixed ops: one batch, ordered append

- Changeset: [create for node X replacing A (modified lineage), create for new node Y, close br-A
  (modified), close br-B (removed)] — emit's real ordering (creates before closes).
- Receipts: all ok.
- Expected: the journal gains, in op order, the modified event for X with its two receipts, the
  added event for Y with its task_created, and the removed event for B's node with its
  task_closed. The fold answers X → new task, no live entry for B's node, Y → its task.

### Proposal-epic create → receipt keyed by slug, no spec-graph lookup

- Changeset: one create op with `spec_node_kind: "proposal_epic"`,
  `spec_node_id: "2026-04-29-decouple-contract-gaps"` (the proposal stem),
  `idempotency.label: "spex:2026-04-29-decouple-contract-gaps"`.
- Receipts: `status: "ok"`, `bead_id: "br-epic"`, `was_existing: false`.
- Spec graph: empty (the proposal stem is NOT a spec-graph node by design).
- Expected: a `task_created` receipt carrying `proposal: "2026-04-29-decouple-contract-gaps"` and
  no `for` — no change event is invented for it. The reconciler MUST NOT resolve the stem against
  the spec graph; that lookup would fail because the stem is not in the identity-hash keyspace.

### Cleanup create → receipt pairs with the prior removed event

- Initial journal: a `removed` event for node `abc123def456` (eid `E1`), from a prior run.
- Changeset: one create op with `spec_node_kind: "cleanup"`, `spec_node_id: "abc123def456"`,
  `idempotency.label: "spex:cleanup-abc123def456"`.
- Receipts: `status: "ok"`, `bead_id: "br-cleanup"`, `was_existing: false`.
- Expected: a `task_created` receipt with `for: "E1"` and `task_id: "br-cleanup"` — the cleanup
  task is born pointing at the removal it answers. No new change event.

### Cleanup create → receipt pairs with a same-batch removal

- Initial journal: `added` + `task_created` (task `br-gone`) for the still-live node being
  cleaned up.
- Changeset: [cleanup create for that node's hash, then close `br-gone` (removed)] — emit's real
  ordering, the cleanup create before the close that performs its removal.
- Receipts: both ok.
- Expected: same shape as the prior-removal scenario, but the referent is resolved from the
  batch's own removal close rather than the journal — neither the fold nor lines already appended
  show the node as removed yet when the cleanup create is processed, since its removal comes
  after it in this same batch.

### Modified close with no paired create → refused before append

- Changeset: one close op, reason starting "Spec node modified", targeting a bead that no create
  in the batch claims via a `blocks` dep.
- Receipts: close op `status: "ok"`.
- Expected: structured error naming the op and the unclaimed bead; the journal file is
  byte-identical to its pre-run state. A batch is not reported as successful with a retired
  task's closure silently missing from the journal.

### Receipt referencing nothing → refused before append

- Changeset/receipts constructed so an ok create's op matches no change event and no prior removed
  event, and carries no proposal stem.
- Expected: structured error naming the op; the journal file is byte-identical to its pre-run
  state — a refused batch appends nothing.

## Idempotent Append

Re-running `Reconciler.Apply` with the same changeset+receipts pair over the already-appended
journal appends zero lines and reports success. Event ids derive from `(git_head, op_id)`; receipt
pairing keys the same way. This replaces the retired counter-advance semantics — there is no
counter, and nothing is spent by a re-run.

## Fixtures

In-code Go fixtures, no on-disk testdata. `ingest/reconciler_test.go` builds each scenario's
`emit.Changeset` and `adapters.Receipts` as Go values, seeds the journal with a test helper, and
supplies spec metadata with a fake spec graph.
