# Reconciler

Orchestrates a normal-mode ingest run. Consumes `changeset.json` + `receipts.json` and turns the
pair into one atomic journal append: [[539030e8c5a4|every ok receipt's lines land, and a receipt
reporting an error appends nothing]]. The construction, checking and encoding it used to carry
inline are carved out beneath it — it builds [[3c0569749972|EventBuilder]] once per run and
dispatches each op to it, runs [[5fd9613616e1|InvariantChecker]] over journal-plus-batch, and
encodes the surviving lines through [[6ce1df0a456b|JournalEncoder]] — and it remains the one
place the run's sequence lives.

## Responsibilities

- Pair each op in the changeset with its corresponding receipt by op_id.
- Open the journal and fold it.
- Assemble the per-run state — the eid predicate over journal and in-flight batch, the fold with
  each node's latest change event, the registered-by-stem map — and hand it to
  [[3c0569749972|EventBuilder]] once, at construction, instead of threading it through every
  call. No batch-wide pre-pass precedes the loop: no op's lines depend on another op in the
  batch, so there is nothing to resolve up front.
- Loop the ops in changeset order, dispatching each (and each absorbed entry) to the builder;
  collect what it constructs.
- After the whole batch is constructed, run [[5fd9613616e1|InvariantChecker]] against
  journal-plus-batch; refuse the run on the first failure.
- Encode each surviving line through [[6ce1df0a456b|JournalEncoder]], whose schema gate refuses
  the batch before the write on an invalid line.
- Commit the append through MappingStore's writer-owner primitive: whole-file write-and-rename,
  same guarantee as the snapshot.

## Interface

Reconciler is set up from two things: the journal it will append to, and a view of the current
spec graph it can ask for a node's name, kind and module — the identity a change event must carry,
because the event is the only record of it once the node is gone. It is then handed the changeset
and the receipts together, in one call, and answers with a tally or with an error — never with
both. The changeset is v4 and the receipts v2; a receipt's tracker id is read from its `task_id`
field, and the version envelopes are checked by IngestCommand before the pair reaches here.

That tally is what the run's summary on stdout is built from: ok creates, ok closes and ok
retargets (added together into the summary's `ok` count), skipped ops, errored ops, and the two
append counts — events and receipts.

## Adapter-Side Recovery

`was_existing = true` means the adapter found a task already carrying the op's exact label, in
any status — a receipt only an adapter that implements the optional label probe can produce; an
adapter without label support never reports it, and the reconciler asks no questions either way:
receipts are the adapter-side half of re-run idempotency, the probe merely narrows how often a
crashed run's task gets a duplicate.
Usually the journal already holds the pairing and the derived eid matches — the batch constructs
the same lines, finds them present, and appends nothing. The interesting case is
`was_existing = true` with no pairing in the journal: the signature of a previous run where the
adapter created the task and then died before its receipt reached disk. The batch's lines are
genuinely new, so they land now, pairing the event with the task the dead run made — and the next
plan run's fold sees the node as taken. See `test_partial_run_recovery.md`, "Partial with
Adapter-Side Duplicates".

## Transaction Semantics

- The batch is constructed in memory against a parsed copy of the journal.
- Only after every invariant holds and every line validates does the commit happen, via the
  atomic write-and-rename.
- A failure at any step — construction, checking, encoding — leaves the on-disk journal
  untouched.

The commit does not wait for a complete run. Reconciler never reads the receipts' top-level
status: [[16dbbee94e88|a run the adapter stopped part-way still appends every ok op's lines and
constructs nothing for the skipped and errored ones]], so what lands is internally consistent at
the point the adapter stopped rather than a rollback of the whole batch. Gating on completeness is
the snapshot's job, not the journal's — the journal may run ahead of the snapshot until the
completing re-run, and only in that direction.

## One write path, no tracker

Reconciler (with RefreshHandler, its refresh-mode sibling) appends through the map module's
MappingStore — the journal's writer-owner, whose one primitive also serves the proposal
Registrar's `registered` event. Plan folds the journal — parent resolution, epic recognition — and
never writes it. That asymmetry is deliberate: a failed plan run that never reaches ingest must
leave the journal byte-identical, so the next run re-derives the same view and is retryable.

Reconciler also never contacts a tracker. It learns what happened solely from the
`(changeset, receipts)` pair — which is why a receipt's `task_id` and `was_existing` are
load-bearing rather than advisory, and why an op with an `error` receipt constructs nothing
rather than triggering a status query. `spex` closes the loop on file evidence alone; the adapter
is the only participant that ever ran a tracker command. The same holds for what the pair does
not say: a task the pipeline never closed is never journaled as closed, however finished it is in
the tracker, because completion is not something this run did.

The practical consequence is that ingest is replayable. Reconciling the same changeset and
receipts twice produces the same journal — the second pass derives the same eids, finds every
line present, and appends nothing — because every line is keyed off the op and the changeset's
git_head rather than off tracker state that may have moved underneath.

## Ordering

Within a run, ops are processed in the order they appear in changeset.json — same order the
adapter executed them, same order their lines land in the journal. Plan orders every changeset
create-before-close: all create ops first, in the sorter's topological order, then the retargets,
then one close op per close action (`arch_changeset_builder.md`, "Canonical Output"). No op in
the batch names another op's node — a successor create carries no reference to a predecessor, and
a cleanup is never accompanied by a close for the same node — so no construction path depends on
seeing any other op first. Ordering is still what makes the fold's "latest wins" rule meaningful
once lines land.

## Error Surface

All errors are wrapped with `"ingest: reconcile: ..."` context, whichever carved-out component
raised them. Invariant failures name the specific invariant and the offending spec_node_id,
op_id or eid.
