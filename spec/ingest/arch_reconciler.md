# Reconciler

Consumes `changeset.json` + `receipts.json` and turns each op's receipt into journal lines:
[[539030e8c5a4|an ok create appends a change event and the task_created receipt pairing it, an ok
close on a removed node appends the removed event and its task_closed, and a receipt reporting an
error appends nothing]]. Cleanup creates pair with the removal event they answer — prior-batch or
same-batch — instead of a fresh one; epic creates pair with the proposal's `registered` event; an
ok retarget appends [[7191a50f7447|the `modified` change event plus its `task_retargeted` receipt]];
the changeset's `absorbed` array appends [[7900dcd38c4a|one `modified` event per entry, closed by
one `refresh` receipt naming them]] — every receipt references an event. Nothing reaches disk until
[[ee28b5d190ae|the journal invariants this component checks]] hold over the batch.

## Responsibilities

- Pair each op in the changeset with its corresponding receipt by op_id.
- For ok ops, construct the journal lines the op implies — change event, receipts, or both.
- For error/skipped ops, construct nothing.
- Derive each change event's `eid` from `(git_head, op_id)` and drop any line the journal already
  contains — the batch is idempotent by construction, and the epic's receipt dedups by its
  registered-event referent like every other line.
- After constructing the whole batch, assert the invariants against journal-plus-batch.
- Commit the append through MappingStore's writer-owner primitive: whole-file write-and-rename,
  same guarantee as the snapshot.

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
| create  | `proposal_epic` | ok            | one `task_created` whose `for` is the proposal's `registered` event — no change event (see Proposal-Epic Ops) |
| create  | `cleanup`       | ok            | one `task_created` whose `for` is the `removed` event for the op's spec_node_id (see Cleanup-Create Ops) |
| create  | other          | ok             | the change event (`added`, or `modified` when paired with a close) plus a `task_created` with the receipt's task id |
| create  | any            | error/skipped  | nothing |
| close   | —              | ok (reason="Spec node removed")  | the `removed` change event plus a `task_closed` |
| close   | —              | ok (reason starts "Spec node modified"), bead claimed by a create in this batch | a `task_closed` whose `for` is the pair's `modified` event; the paired create op owns that event |
| close   | —              | ok (reason starts "Spec node modified"), bead's create errored/skipped in this batch | nothing — partial run, see "The Modified-Node Pair" |
| close   | —              | ok (reason starts "Spec node modified"), bead claimed by no create in this batch | the `modified` change event plus a `task_closed`, built from the close alone — no `task_created` (see "The Modified-Node Pair") |
| close   | —              | error          | nothing |
| retarget | —             | ok             | the `modified` change event plus a `task_retargeted` with the receipt's bead id (see Retarget Ops) |
| retarget | —             | error/skipped  | nothing |
| label   | —              | any            | nothing (labels don't reach the journal) |
| tag     | —              | any            | nothing |

## Retarget Ops

A retarget op is what plan emits for a modified node whose open, unclaimed task moves to the new
state instead of being recreated. Its ok receipt constructs a pair: the `modified` change event —
eid derived from `(git_head, op_id)` exactly as a create op's event is, node identity and hashes
from the op, name/kind/module from the spec graph — plus a `task_retargeted` receipt whose `for`
is that event's eid and whose `task_id` is the existing task the op targeted. No `task_closed`
and no `task_created` accompany the pair: nothing died and nothing was born. The old pairing
stays in the journal as history, and the fold answers with the retargeted event because
`task_retargeted` is task-bearing and later in the file. Both lines dedup by derived event id, so
re-processing the batch appends nothing.

## Absorbed Entries

The changeset's top-level `absorbed` array is processed alongside the ops, from the changeset
alone — no receipt corresponds to an absorbed entry, because the adapter ignores the array and no
tracker work happened. For each entry, Reconciler constructs one `modified` change event: node
identity and before/after hashes off the entry itself, name/kind/module from the spec graph, the
event's `git_head` and `proposal` from the changeset's own top-level fields — exactly as op-born
events take them — and its eid derived from `(node, before, after)` exactly as refresh-born events
are, because an absorbed entry has no create op to key from. The batch's absorbed events are closed by one `refresh` receipt naming
exactly their eids, the same shape a whole-run refresh appends, so a reader of the journal cannot
tell per-node absorption from refresh-mode absorption except by what else the run appended. No
task receipt is constructed — a `refresh` receipt is not task-bearing, so the node's pairing keeps
its sourcing event and the task owes nothing.

Absorbed entries are not receipt-gated: they describe spec state, not tracker work, so they append
on partial runs too. Re-absorption is harmless by construction — after a partial run the snapshot
is unsaved, the next diff re-reports the node, the operator marks it again, and the re-derived
eids find their lines already present and append nothing. An empty `absorbed` array constructs
nothing, not an empty receipt.

## The Modified-Node Pair

Plan generates two ops for a modified node whose pairing's task is closed: a create op labeled
`spex:<eid>` of the pair's `modified` event — the very event this component will mint from the
create op's `(git_head, op_id)` — plus a close op for the old task, in that
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
earlier create's `blocks` dep. A "Spec node modified" close whose bead was not claimed by an
already-processed create is parked until the whole batch has been processed, then resolved one of
two ways, discriminated on whether a create op for that bead exists in the changeset at all — not
on receipt order:

- **A create op for the bead exists in the changeset, but its receipt was `error` or `skipped`.**
  This is a partial run (`scripts/apply-br.sh` routinely continues past a failed create to close
  the old task anyway): the pair constructs nothing, same as any other errored/skipped create, and
  the rest of the batch still lands.
- **No create op for the bead exists in the changeset at all.** This is the shape the classifier
  emits for a coupled `test_section` edit: an `obsolete` action with
  no replacement create, because the section's bead is folded into its owning component going
  forward rather than replaced. The node itself still exists — only its hash changed — so the close
  builds the `modified` event and its `task_closed` on its own: identity and prior hash come from
  the journal's live fold entry for the closing bead (exactly as `buildRemoved` resolves a removed
  node's identity), and current name/module/path/hash come from the spec graph. No `task_created` is
  built — there is no successor task to pair.

A close naming a bead the journal has never heard of (no fold entry at all) has no identity to
build from and is refused as a malformed changeset. "Spec node removed" closes construct the
`removed` event directly and never park.

## Adapter-Side Recovery

`was_existing = true` means the adapter found a task already carrying the op's exact label, in
any status.
Usually the journal already holds the pairing and the derived eid matches — the batch constructs
the same lines, finds them present, and appends nothing. The interesting case is
`was_existing = true` with no pairing in the journal: the signature of a previous run where the
adapter created the task and then died before its receipt reached disk. The batch's lines are
genuinely new, so they land now, pairing the event with the task the dead run made — and the next
plan run's fold sees the node as taken. See `test_partial_run_recovery.md`, "Partial with Adapter-Side
Duplicates".

## Proposal-Epic Ops

Create ops with `spec_node_kind == "proposal_epic"` represent a proposal as a tracker task. Their
`spec_node_id` is the **proposal stem** (e.g. `2026-04-29-decouple-contract-gaps`), NOT a
12-character identity hash, because proposals live as markdown files under `spec/proposals/` and
have no spec-graph node.

The Reconciler MUST treat them as a distinct case:

- **Skip** the spec-graph metadata lookup. The stem is not an identity hash; the lookup would
  always fail with `"spec graph: no node <stem>"` and abort the run with exit 2.
- **Construct** only a receipt: `{"event": "task_created", "for": "<registered eid>",
  "task_id": <receipt's>}`. The referent is the proposal's `registered` event, which registration
  appended when the lifecycle opened (a one-shot backfill line covers proposals registered before
  the event type existed). No change event exists or is invented — an epic is not born from a
  diff entry; it is born from a registration, and the registered event is that fact on the
  record.
- An epic op for a proposal with no `registered` event in the journal is an invariant failure —
  plan refuses to build such an op, so its arrival marks a malformed changeset.

Legacy epic receipts carrying `proposal: <stem>` and no `for` are read as inert history behind
the fold's read-only legacy branch; none is ever constructed anew. The fold lists epic tasks
keyed by their slug — carried by the registered event, or by the legacy receipt itself — which is
how a re-run recognises an epic that already exists.

## Cleanup-Create Ops

Create ops with `spec_node_kind == "cleanup"` represent a code-cleanup task for a spec node that
was removed in a prior or concurrent batch. Their `spec_node_id` carries the identity hash of the
now-removed node, and their label is `spex:<eid>` of the removal event they answer — label and
`task_created` referent are the same event.

The Reconciler MUST treat them as a distinct case:

- **Discriminate on the op's spec node kind before anything else** — before the spec graph is
  consulted, which no longer holds the node.
- **Construct** a `task_created` whose `for` is the journal's latest `removed` event for that
  hash — the cleanup task is born pointing at the removal it answers, which is what makes its
  label resolvable from day one. If the journal holds no removed event for the hash (the removal
  is in this same batch), the referent is the removal's eid, resolved from the whole batch's ok
  removal closes before any op is processed — not from a scan of lines already appended to the
  batch. The changeset lists the cleanup create before the close that performs its removal (see
  "Ordering"), so at the point the create is processed neither the on-disk fold nor the
  batch-so-far shows the node as removed yet; only a batch-wide, order-independent resolution
  gets the referent right.
- A cleanup op whose hash matches no removed event anywhere is an invariant failure, not a
  fallback — a cleanup for a removal that never happened is a malformed changeset.

## Invariant Enforcement

The invariants are asserted once, after the whole batch is constructed and before anything reaches
disk, in numeric order, so the first message a caller sees names the most upstream cause:

1. Every ok create pairs exactly one `task_created` with exactly one referent event — a change
   event in journal or batch, the removal event for cleanups, or the registered event for epics.
   Every ok retarget pairs exactly one `task_retargeted` with its own `modified` event, and the
   batch's absorbed events are closed by exactly one `refresh` receipt naming them.
   The retired "or a proposal slug" arm survives only in legacy lines already on disk.
2. No receipt references an eid that neither the journal nor the batch contains — `for` fields
   and the entries of a `refresh` receipt's `absorbed` list alike.
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

## One write path, no tracker

Reconciler (with RefreshHandler, its refresh-mode sibling) appends through the map module's
MappingStore — the journal's writer-owner, whose one primitive also serves the proposal
Registrar's `registered` event. Plan folds the journal — parent resolution, epic recognition — and never
writes it. That asymmetry is deliberate: a failed plan run that never reaches ingest must leave the
journal byte-identical, so the next run re-derives the same view and is retryable.

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
adapter executed them, same order their lines land in the journal. Plan orders every changeset
create-before-close: all create ops first, in the sorter's topological order, then the retargets,
then one close op per obsolete (`arch_changeset_builder.md`, "Canonical Output"). The modified-node pair and a cleanup create with
its own removal both arrive in that order — the create before the close naming the same bead or
node. Reconciler does not depend on seeing the close first for either: the modified-node pair is
built entirely from the create op's own `blocks` dep and the pre-batch fold (see "The Modified-Node
Pair"), and a cleanup's referent is resolved from the whole batch's ok removal closes before any op
is processed (see "Cleanup-Create Ops"). Ordering is still what makes the fold's "latest wins" rule
meaningful once lines land.

## Error Surface

All errors are wrapped with `"ingest: reconcile: ..."` context. Invariant failures name the
specific invariant and the offending spec_node_id, op_id or eid.
