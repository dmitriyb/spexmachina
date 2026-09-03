# Reconciliation tests

Integration tests for `Reconciler.Apply` against fixture changesets and receipts. Every scenario
crosses the orchestrator/builder boundary: `Reconciler` pairs receipts to ops, assembles the
per-run state and dispatches; `EventBuilder` constructs the lines each op implies. The scenarios
assert on what lands in the journal, so a construction defect in the builder and a dispatch defect
in the orchestrator both surface here.

## Setup

- In-code Go fixtures, no on-disk testdata: helpers in `ingest/reconciler_test.go` build each
  changeset/receipts pair and seed a temporary journal.
- Tests write an initial journal file, run reconciliation, and assert the appended lines
  match expected — by parsing, never by byte comparison, since field order inside a line is the
  encoder's business.
- Changesets are v4 and receipts v2 throughout: refs are `ref:task`/`ref:op`, receipts key the
  tracker id as `task_id`, and no op carries a lineage dep.

## Scenarios

### Ok create on an unknown node → added event and receipt appended

- Changeset: one create op, spec_node_id `abc123def456`, idempotency label
  `spex:cafe1234:<op_id>` (the eid the event below will carry), git_head `cafe1234`. The journal
  holds no change event for the node.
- Receipts: corresponding op receipt with `status: "ok"`, `task_id: "br-new"`, `was_existing: false`.
- Expected: the journal gains an `added` change event for node `abc123def456` (eid derived from
  `(cafe1234, <op_id>)`, `before` null) and a `task_created` receipt with `for` naming that eid
  and `task_id: "br-new"`.

### Ok create on a known node → modified event, no task_closed

- Initial journal: `added` (after `aaa`) + `task_created` (task `br-old`) for node `feedface0002`.
- Changeset: one create op for `feedface0002` labeled `spex:<git_head>:<its op_id>`, carrying no
  dep on `br-old` and accompanied by no close op — plan's shape for a modified node whose task is
  finished.
- Receipts: create ok, `task_id: "br-new"`, `was_existing: false`.
- Expected: a `modified` change event for `feedface0002` with `before: "aaa"` — the journal's
  latest `after` for the node, because the op itself carries no change type — plus a
  `task_created` for `br-new`. **No** `task_closed` for `br-old`: the journal never records a
  task's completion, and the old pairing stays as lineage. The fold now answers
  `feedface0002 → br-new`.

### Ok create on a re-added node → added event

- Initial journal: `added` + `task_created` (task `br-1`), then `removed` (after null) +
  `task_closed` for node `feedface0005`.
- Changeset: one create op for `feedface0005`, the node re-added under the same name.
- Receipts: ok, `task_id: "br-2"`.
- Expected: an `added` event with `before` null — the journal's latest change event carries no
  `after`, so the node is born again rather than modified — and a `task_created` for `br-2`.

### Ok close on removed → removed event and task_closed appended

- Initial journal: `added` + `task_created` (task `br-old`) for node `feedface0001`.
- Changeset: one close op targeting task `br-old`, `reason` starting with "Spec node removed".
- Receipts: op receipt `status: "ok"`.
- Expected: a `removed` change event for `feedface0001` carrying its name, node_type and module — the
  biography that outlives the node — plus a `task_closed` receipt for `br-old`. The earlier
  pairing lines remain untouched: nothing is ever deleted.

### Was_existing=true → idempotent no-op

- Initial journal already holds the change event and `task_created` for node `A`, task `br-7`.
- Changeset: the same create op re-emitted (same git_head, same op_id).
- Receipts: create op `status: "ok"`, `task_id: "br-7"`, `was_existing: true`.
- Expected: nothing appended — the derived eid already exists, and the receipt pairs with it.
  No error.

### Error status → op skipped, nothing appended

- Changeset: one create op.
- Receipts: that op with `status: "error"`, `task_id: ""`.
- Expected: no event, no receipt; no error from the reconciler (the adapter's failure is the
  user's problem, ingest records truth — and the truth is that nothing happened).

### Skipped status → op no-op

- Changeset: create op.
- Receipts: op with `status: "skipped"`.
- Expected: nothing appended.

### Mixed ops: one batch, ordered append

- Changeset: [create for known node X (its earlier task finished), create for new node Y, close
  br-B (removed)] — plan's real ordering (creates before closes).
- Receipts: all ok.
- Expected: the journal gains, in op order, the modified event for X with its task_created, the
  added event for Y with its task_created, and the removed event for B's node with its
  task_closed. The fold answers X → new task, no live entry for B's node, Y → its task. No line
  in the batch names X's earlier task.

### Proposal-epic create → receipt references the registered event, no spec-graph lookup

- Initial journal: a `registered` event for the proposal
  (`eid: "beef0001:2026-04-29-decouple-contract-gaps"`).
- Changeset: one create op with `spec_node_kind: "proposal_epic"`,
  `spec_node_id: "2026-04-29-decouple-contract-gaps"` (the proposal stem),
  `idempotency.label: "spex:beef0001:2026-04-29-decouple-contract-gaps"` — the registered eid.
- Receipts: `status: "ok"`, `task_id: "br-epic"`, `was_existing: false`.
- Spec graph: empty (the proposal stem is NOT a spec-graph node by design).
- Expected: a `task_created` receipt with `for: "beef0001:2026-04-29-decouple-contract-gaps"` —
  no change event is invented, and no `proposal`-keyed receipt is constructed. The reconciler
  MUST NOT resolve the stem against the spec graph; that lookup would fail because the stem is
  not in the identity-hash keyspace.

### Proposal-epic create without a registered event → invariant failure

- Same changeset as above, but the journal holds no `registered` event for the slug.
- Expected: a structured error naming the slug and the missing referent; nothing appended. Plan
  refuses to build such an op, so its arrival marks a malformed changeset.

### Cleanup create → receipt pairs with the prior removed event

- Initial journal: the node `abc123def456`'s latest change event is a `removed` event (eid
  `E1`), from a prior run whose cleanup errored.
- Changeset: one create op with `spec_node_kind: "cleanup"`, `spec_node_id: "abc123def456"`,
  `idempotency.label: "spex:E1"` — the removal event's own eid.
- Receipts: `status: "ok"`, `task_id: "br-cleanup"`, `was_existing: false`.
- Expected: a `task_created` receipt with `for: "E1"` and `task_id: "br-cleanup"` — the cleanup
  task is born pointing at the removal it answers. No new change event.

### Cleanup create → the cleanup mints the removal itself

- Initial journal: `added` (after `aaa`) + `task_created` (task `br-gone`) for the node being
  cleaned up; no `removed` event exists for it.
- Changeset: [cleanup create for that node's hash, labeled `spex:<git_head>:<its op_id>`] and
  **no** close op — the node's task is finished, so plan issued nothing to close.
- Receipts: ok, `task_id: "br-cleanup"`.
- Expected: a `removed` change event for the node — eid derived from the cleanup op's own
  `(git_head, op_id)`, `before: "aaa"`, `after` null, name/kind/module from the journal's live
  fold entry for the node, since the spec graph no longer holds it — plus a `task_created` with
  `for` naming that eid. The finished task `br-gone` gets no `task_closed`. This is the ordinary
  cleanup case: the removal event is born with the cleanup, by the same derivation every
  node-bearing create uses.

### Cleanup create after a re-add → a fresh removal, not the old one

- Initial journal: `removed` (eid `E1`) for the node, then `added` + `task_created` for the same
  hash — re-added, then finished.
- Changeset: a cleanup create labeled `spex:<git_head>:<its op_id>`.
- Expected: a new `removed` event from the op's own derivation and a `task_created` naming it;
  `E1` is not the referent, because it is not the node's latest state.

### Fold-back close, task live in the journal → modified event from the close alone

- Initial journal: `added` + `task_created` (task `br-old`) for a `test_section` node.
- Changeset: one close op, reason starting "Spec node modified", targeting `br-old` — the shape
  the classifier emits for a coupled `test_section` edit whose task was open: the section folds
  into its owning component and its own task is cancelled.
- Receipts: close op `status: "ok"`.
- Expected: a `modified` change event for the node (identity and prior hash from the journal's live
  fold entry for `br-old`, current name/module/path/hash from the spec graph) plus a `task_closed`
  for `br-old` naming it. No `task_created` — there is no successor task.

### Fold-back close naming a task unknown to the journal → refused before append

- Changeset: one close op, reason starting "Spec node modified", targeting a task that has no
  fold entry at all.
- Receipts: close op `status: "ok"`.
- Expected: structured error naming the op and the unknown task; the journal file is
  byte-identical to its pre-run state. A close has no identity to build from except the journal's,
  so a close the journal cannot place is a malformed changeset.

### Ok retarget → modified event and task_retargeted appended

- Initial journal: `added` + `task_created` (task `br-open`) for node `feedface0003`.
- Changeset: one retarget op targeting `br-open`, `spec_node_id: "feedface0003"`, its new content
  hash, `labels: ["spex:cafe1234:<op_id>"]` — the eid of the `modified` event below.
- Receipts: op receipt `status: "ok"`, `task_id: "br-open"`.
- Expected: the journal gains a `modified` change event for `feedface0003` (eid derived from
  `(cafe1234, <op_id>)`, name/kind/module from the spec graph) and a `task_retargeted` receipt
  with `for` naming that eid and `task_id: "br-open"`. No `task_closed`, no `task_created` — the
  task neither died nor was born. The fold now answers `feedface0003 → br-open` sourced from the
  new event.

### Retarget re-run → idempotent no-op

- Initial journal already holds the retarget's `modified` event and its `task_retargeted` line.
- The same changeset+receipts pair is reconciled again.
- Expected: nothing appended — both lines dedup by derived event id, like every other batch.

### Retarget with error receipt → nothing appended

- Same retarget changeset; receipt `status: "error"`.
- Expected: no event, no receipt, no error from the reconciler.

### Absorbed entry → modified event and refresh receipt appended

- Changeset: empty `ops`, one `absorbed` entry — node `feedface0004`, before `aaa`, after `bbb`,
  reason "typo sweep". Receipts: empty `ops`, status complete.
- Expected: the journal gains one `modified` change event for `feedface0004` — eid derived from
  `(node, before, after)`, hashes off the entry, name/kind/module from the spec graph — and one
  `refresh` receipt whose `absorbed` list names exactly that eid. No task receipt of any kind: the
  node's existing pairing keeps its sourcing event.

### Absorbed entries land on partial runs too

- Changeset: one create op plus one `absorbed` entry; receipts: the create `status: "error"`,
  top-level status partial.
- Expected: nothing for the errored create; the absorbed entry's `modified` event and the
  `refresh` receipt still land — absorption describes spec state, not tracker work, and is not
  receipt-gated.

### Absorbed re-run → idempotent no-op

- The same changeset+receipts pair reconciled again over the journal the prior scenario left.
- Expected: nothing appended — the `(node, before, after)` derivation finds the event present, and
  an empty remainder appends no second `refresh` receipt.

### Receipt referencing nothing → refused before append

- Changeset/receipts constructed so an ok create's op matches no change event and no prior removed
  event, and carries no proposal stem.
- Expected: structured error naming the op; the journal file is byte-identical to its pre-run
  state — a refused batch appends nothing.

### Eid predicate sees the journal and the in-flight batch

- Initial journal: the change event and `task_created` for node `A` (a journal-side duplicate).
- Changeset: [the same create op for `A` re-emitted (same git_head, same op_id), a create op for
  node `B`, then a second create op whose derived eid collides with `B`'s] — receipts all ok.
- Expected: nothing appended for `A` (its eid is already in the journal), `B`'s event and receipt
  land once, and the colliding op appends nothing — its eid is already in the in-flight batch.
  This pins the one thing the decomposition could plausibly get wrong: the predicate is
  `EventBuilder`'s own per-run state, mutated as the batch grows, and it must answer for both
  sources — lines already on disk and lines constructed earlier in this same run — not only the
  journal it was seeded from.

## Idempotent Append

Re-running `Reconciler.Apply` with the same changeset+receipts pair over the already-appended
journal appends zero lines and reports success. Event ids derive from `(git_head, op_id)`; receipt
pairing keys the same way. This replaces the retired counter-advance semantics — there is no
counter, and nothing is spent by a re-run.

## Fixtures

In-code Go fixtures, no on-disk testdata. `ingest/reconciler_test.go` builds each scenario's
`plan.Changeset` and `adapters.Receipts` as Go values, seeds the journal with a test helper, and
supplies spec metadata with a fake spec graph.
