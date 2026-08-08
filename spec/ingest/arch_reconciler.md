# Reconciler

Consumes `changeset.json` + `receipts.json` and turns each op's receipt into journal lines:
[[539030e8c5a4|an ok create appends a change event and the task_created receipt pairing it, an ok
close on a removed node appends the removed event and its task_closed, and a receipt reporting an
error appends nothing]]. Cleanup creates pair with a prior removed event instead of a fresh one;
epic creates pair with a proposal slug and no event at all. Nothing reaches disk until
[[ee28b5d190ae|the journal invariants this component checks]] hold over the batch.

## Responsibilities

- Pair each op in the changeset with its corresponding receipt by op_id.
- For ok ops, construct the journal lines the op implies — change event, receipts, or both.
- For error/skipped ops, construct nothing.
- Derive each change event's `eid` from `(git_head, op_id)` and drop any line whose eid the
  journal already contains — the batch is idempotent by construction.
- After constructing the whole batch, assert the invariants against journal-plus-batch.
- Atomically commit the append: whole-file write-and-rename, same guarantee as the snapshot.

## Interface

Reconciler is set up from two things: the journal it will append to, and a view of the current
spec graph it can ask for a node's name, kind and module — the identity a change event must carry,
because the event is the only record of it once the node is gone. It is then handed the changeset
and the receipts together, in one call, and answers with a tally or with an error — never with
both.

That tally is what the run's summary on stdout is built from: ok creates and ok closes (added
together into the summary's `ok` count), skipped ops, errored ops, and the two append counts —
events and receipts.

## Per-Op Construction Table

| Op type | spec_node_kind | Receipt status | Journal lines constructed |
|---------|----------------|----------------|---------------------------|
| create  | `proposal_epic` | ok            | one `task_created` with `proposal: <stem>` and no `for` — no change event (see Proposal-Epic Ops) |
| create  | `cleanup`       | ok            | one `task_created` whose `for` is the prior `removed` event for the op's spec_node_id (see Cleanup-Create Ops) |
| create  | other          | ok             | the change event (`added`, or `modified` when paired with a close) plus a `task_created` with the receipt's task id |
| create  | any            | error/skipped  | nothing |
| close   | —              | ok (reason="Spec node removed")  | the `removed` change event plus a `task_closed` |
| close   | —              | ok (reason starts "Spec node modified") | a `task_closed` whose `for` is the pair's `modified` event; the paired create op owns that event |
| close   | —              | error          | nothing |
| label   | —              | any            | nothing (labels don't reach the journal) |
| tag     | —              | any            | nothing |

## The Modified-Node Pair

Emit generates two ops for a modified node: a create op with the `spex:<spec_node_id>` label —
same label because the node's hash has not changed — plus a close op for the old task, in that
order (see "Ordering"). The create op alone carries everything the pair needs: its `blocks` dep
names the old bead directly, and the node's current live fold entry supplies the prior content
hash for `before`. Reconciler builds the whole pair while processing the create — one `modified`
change event, a `task_closed` for the retired task and a `task_created` for its successor, both
receipts carrying the pair's `modified` event as their `for`, so the join needs no scan by task id
— without waiting to see the paired close. Nothing is rebound and nothing is deleted — the old
pairing stays in the journal as lineage, and the fold answers with the successor because it is
later in the file.

When the paired close is then reached, it constructs nothing of its own — the create already built
both receipts — and detects that by the close op's target bead having already been claimed by an
earlier create's `blocks` dep. A "Spec node modified" close whose bead was never claimed by any
create in the batch is not silently dropped: once the whole batch is processed, Reconciler fails
loudly on it as a malformed changeset, rather than reporting success having appended nothing for
it. "Spec node removed" closes construct the `removed` event directly and never park.

## Adapter-Side Recovery

`was_existing = true` means the adapter found an open task already carrying the op's label.
Usually the journal already holds the pairing and the derived eid matches — the batch constructs
the same lines, finds them present, and appends nothing. The interesting case is
`was_existing = true` with no pairing in the journal: the signature of a previous run where the
adapter created the task and then died before its receipt reached disk. The batch's lines are
genuinely new, so they land now, pairing the event with the task the dead run made — and the next
emit's fold sees the node as taken. See `test_partial_run_recovery.md`, "Partial with Adapter-Side
Duplicates".

## Proposal-Epic Ops

Create ops with `spec_node_kind == "proposal_epic"` represent a proposal as a tracker task. Their
`spec_node_id` is the **proposal stem** (e.g. `2026-04-29-decouple-contract-gaps`), NOT a
12-character identity hash, because proposals live as markdown files under `spec/proposals/` and
have no spec-graph node.

The Reconciler MUST treat them as a distinct case:

- **Skip** the spec-graph metadata lookup. The stem is not an identity hash; the lookup would
  always fail with `"spec graph: no node <stem>"` and abort the run with exit 2.
- **Construct** only a receipt: `{"event": "task_created", "proposal": <stem>, "task_id": <receipt's>}`.
  No change event exists or is invented — an epic is not born from a diff entry, and the receipt's
  `proposal` key is the whole of its addressing.

The fold lists epic tasks keyed by their slug, which is how a re-run recognises an epic that
already exists.

## Cleanup-Create Ops

Create ops with `spec_node_kind == "cleanup"` represent a code-cleanup task for a spec node that
was removed in a prior or concurrent batch. Their `spec_node_id` carries the identity hash of the
now-removed node, and their label is `spex:cleanup-<spec_node_id>`.

The Reconciler MUST treat them as a distinct case:

- **Discriminate on the op's spec node kind before anything else** — before the spec graph is
  consulted, which no longer holds the node.
- **Construct** a `task_created` whose `for` is the journal's latest `removed` event for that
  hash — the cleanup task is born pointing at the removal it answers, which is what makes its
  label resolvable from day one. If the journal holds no removed event for the hash (the removal
  is in this same batch), the referent is the removal's eid, resolved from the whole batch's ok
  removal closes before any op is processed — not from a scan of lines already appended to the
  batch. Emit lists the cleanup create before the close that performs its removal (see
  "Ordering"), so at the point the create is processed neither the on-disk fold nor the
  batch-so-far shows the node as removed yet; only a batch-wide, order-independent resolution
  gets the referent right.
- A cleanup op whose hash matches no removed event anywhere is an invariant failure, not a
  fallback — a cleanup for a removal that never happened is a malformed changeset.

## Invariant Enforcement

The invariants are asserted once, after the whole batch is constructed and before anything reaches
disk, in numeric order, so the first message a caller sees names the most upstream cause:

1. Every ok create pairs exactly one `task_created` with exactly one referent — a change event in
   journal or batch, a prior removed event for cleanups, or a proposal slug for epics.
2. No receipt references an eid that neither the journal nor the batch contains.
3. The batch minus already-present lines is what lands — re-running the same pair appends nothing,
   because eids derive from `(git_head, op_id)`.
4. Not checked here: snapshot-saved-iff-complete is SnapshotSaver's gate.
5. Every line to be appended validates against the journal-line schema.

A failure names the invariant and the offending op or eid, and no candidate batch is written.

## Transaction Semantics

- Reconciler constructs the batch in memory against a parsed copy of the journal.
- Only after every invariant holds does it commit via the atomic write-and-rename.
- A failure at any step leaves the on-disk journal untouched.

The commit does not wait for a complete run. Reconciler never reads the receipts' top-level
status: [[16dbbee94e88|a run the adapter stopped part-way still appends every ok op's lines and
constructs nothing for the skipped and errored ones]], so what lands is internally consistent at
the point the adapter stopped rather than a rollback of the whole batch. Gating on completeness is
the snapshot's job, not the journal's — the journal may run ahead of the snapshot until the
completing re-run, and only in that direction.

## Sole writer, no tracker

Reconciler (with RefreshHandler, its refresh-mode sibling) is the only writer of
`spec/.history.jsonl`. Emit folds the journal — parent resolution, epic recognition — and never
writes it. That asymmetry is deliberate: a failed emit that never reaches ingest must leave the
journal byte-identical, so the next emit re-derives the same view and the run is retryable.

Reconciler also never contacts a tracker. It learns what happened solely from the
`(changeset, receipts)` pair — which is why a receipt's `bead_id` and `was_existing` are
load-bearing rather than advisory, and why an op with an `error` receipt constructs nothing
rather than triggering a status query. `spex` closes the loop on file evidence alone; the adapter
is the only participant that ever ran a tracker command.

The practical consequence is that ingest is replayable. [[fd6f08ef34fa|Reconciling the same
changeset and receipts twice produces the same journal — the second pass derives the same eids,
finds every line present, and appends nothing]], because every line is keyed off the op and the
changeset's git_head rather than off tracker state that may have moved underneath.

## Ordering

Within a run, ops are processed in the order they appear in changeset.json — same order the
adapter executed them, same order their lines land in the journal. Emit orders every changeset
create-before-close: all create ops first, in `Sorter`'s topological order, then one close op per
obsolete (`arch_changeset_builder.md`, "Op ids"). The modified-node pair and a cleanup create with
its own removal both arrive in that order — the create before the close naming the same bead or
node. Reconciler does not depend on seeing the close first for either: the modified-node pair is
built entirely from the create op's own `blocks` dep and the pre-batch fold (see "The Modified-Node
Pair"), and a cleanup's referent is resolved from the whole batch's ok removal closes before any op
is processed (see "Cleanup-Create Ops"). Ordering is still what makes the fold's "latest wins" rule
meaningful once lines land.

## Error Surface

All errors are wrapped with `"ingest: reconcile: ..."` context. Invariant failures name the
specific invariant and the offending spec_node_id, op_id or eid.
