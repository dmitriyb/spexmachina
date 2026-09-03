# IdempotencyLabeler

Assigns each create op the `idempotency.label` [[885096d4941c|the adapter matches against the
tracker before it creates anything]], so a re-run of the same changeset re-attaches to the task
the last run made instead of making a second one. One rule covers every action class: the label
is `spex:<eid>` of the journal event the op's `task_created` will reference. Event ids are
deterministic, which is what lets the builder know them before ingest mints the events: no
cursor, no store write, no state.

The label is the eid's tracker-side carrier and nothing more — insurance an adapter MAY honor,
not a conformance bar. The eid itself is mandatory either way: it is the identity the journal's
`task_created` pairing rests on, which is why the epic's missing-registration error below is
fatal regardless of what any tracker does with labels. An adapter for a tracker without label
support ignores the field entirely and stays conformant, at the stated cost that a run which
died between a create and its receipt can duplicate that create on a blind re-run; receipts,
once written, remain the pairing mechanism either way.

## Responsibilities

- For each create action, return `spex:<eid>` of the op's referent event, per the referent rules
  below. Labeler is **per-action**, not per-batch: the label depends on what the action looks
  like, not on the action's position in the ordered batch, and never on where some other block of
  ops will be numbered.
- Surface assigned labels to ChangesetBuilder so each op's JSON carries its label.

## One rule, three referents

| Action class | Discriminator | Referent event whose eid the label carries |
|--------------|---------------|--------------------------------------------|
| Node-bearing | the action targets a spec node — a fresh node's create and a finished-task node's successor create alike | the change event ingest will mint, eid derived from `(git_head, op_id)` |
| Cleanup      | the action's reason starts with `"Code cleanup:"` | the `removed` event the cleanup answers: the journal's latest change event for the node, read from the fold, when that event is a removal — an earlier batch removed the node and its cleanup never landed — else the `removed` event this op itself will mint, eid derived from its own `(git_head, op_id)` exactly as a node-bearing create's. The same resolution order the reconciler pairs the receipt by, so label and referent stay one fact |
| Epic         | `spec_node_kind` is `proposal_epic` | the proposal's `registered` event, eid (`<git_head>:<slug>`) read from the run's registration — not from the fold, which carries the epic only once its task exists |

The label and the pairing are the same fact stated twice: whatever event the op's `task_created`
will reference, that event's eid is the label. The retired scheme keyed labels on the *node*
(`spex:<spec_node_id>`), which collides across a node's lifetime by construction and forced two
compensations — the adapter's open-only filter and the `cleanup-` prefix — plus a third shape for
epics, whose registration produced no event at all. Keying on the *event* dissolves all three: a
label is unique per change, which is what a task is, so cleanup and ordinary tasks for the same
node carry different labels because they reference different events, and the epic keys the event
its lifecycle opened with.

The cleanup row's second arm is the ordinary one. A removed node's cleanup is issued when the
node's task is finished, and no close op accompanies it — there is nothing live to close — so no
other op in the batch mints the removal; the cleanup op does, and its label is derived from its
own op id like any other create's. The first arm covers the re-run: a removal already journaled
whose cleanup errored, so the same removal is answered again under the label it already has.
Which arm applies is read off the node's *latest* change event, never off any earlier removal in
its lineage: a node removed, re-added and removed again answers the second removal, not the
first.

A retarget op carries the same derivation with a different destination: `spex:<eid>` of this
run's `modified` event, derived from `(git_head, op_id)` exactly as a node-bearing create's, but
placed in the op's `labels` array rather than under `idempotency` — the adapter applies it as an
added label on the existing task, and never probes for it, because an update is naturally
idempotent. The eid the label carries is the eid the op's `task_retargeted` receipt will
reference, so the one-fact rule holds across all four shapes.

## Why label-at-build-time, not adapter-time

The label is the adapter's idempotency key — it checks the tracker for a task carrying this label
before creating. Computing labels deterministically at build time keeps them stable: the same
changeset re-run derives byte-identical labels, so the adapter's exact-match probe re-attaches to
whatever the earlier run already made. A failed plan run that never reaches ingest changes nothing
anywhere; re-running from the same inputs — the same diff, journal and `--git-head` —
produces the same labels.

The label guards re-runs of one changeset; receipts guard recovery across changesets. An eid
embeds the run's `git_head`, so a *fresh* plan run at a moved HEAD mints fresh labels — which is
correct, because that run describes a new batch — and the discipline for an aborted adapter run is
already re-run-the-same-changeset, never re-plan-and-ingest-the-gap: the adapter that dies before
writing receipts leaves nothing for ingest, and its receipts (`op_id → task_id`), once written,
are what pair every landed task. Journal-side, deterministic eids make ingest re-runs append
nothing; adapter-side, receipts are the recovery mechanism.

Partial runs compose the same way. Plan three fresh creates; the adapter lands the first two and
fails the third; ingest appends the two pairings and — because the run was partial — writes no
snapshot. The next diff recomputes against the same baseline, and the resulting changeset carries
only the op whose pairing never landed, so nothing already journaled is re-created whatever label
the new run mints for it.

## Interface

Labeler is asked for one label at a time and answers with one label, or with an error — for an
action so malformed it has no referent to derive (no spec_node_id to key an event from), and for
an epic whose proposal has no registration in the journal, which means the proposal was never
registered: the fix is `spex register`, not a guessed label. That second error and Resolver's
missing-parent error are one verdict read twice — both decided on the run's registration, so a
changeset can never carry an epic op whose label is a guess and whose parent is a synthesis. The builder passes either error
straight up, so `spex plan` fails without writing a changeset rather than emitting an op whose
idempotency key is a guess.

An earlier design reserved a flat block of N integer labels up front. It could not express
per-action rules; the design after it read a cursor from the mapping store and kept invariant
machinery to stop the cursor and the records desynchronizing; the design after that keyed on the
node and needed a prefix and a slug shape to dodge the collisions node-keying creates; the design
after that derived a cleanup's label from a close op elsewhere in the batch, and had to predict
where that block of ops would be numbered. Deriving the label from the referent event, and
minting the referent from the op that carries the label, retired all of it — there is nothing
left to reserve, read, prefix, predict or desynchronize.

## Conflict Avoidance

- An `idempotency.label` is only assigned to a *new* create op. Close ops and
  retarget ops do not get one (they reference existing tasks by `task_id`); a retarget's event
  label rides in `labels` and guards nothing, because updates need no guard.
- An eid has exactly one spelling, so a label comparison is a string comparison — the adapter's
  probe matches exact strings with no parsing.
- Two create ops can never share a label, because no two ops reference the same event: eids embed
  the op id (or, for the epic, the registered event's slug), and event ids are unique in the
  journal by construction.
- Legacy `spex:<int>`, `spex:<spec_node_id>` and `spex:cleanup-<hash>` labels on existing tasks
  are inert: no rule emits those shapes, and exact matching means old labels can never capture a
  new create.

## Test surface

IdempotencyLabeler has no public API surface independent of `ChangesetBuilder` — only Builder
consumes it. Cross-component integration coverage (Labeler paired with Sorter and Builder,
exercising the three label shapes through `Builder.Build()`'s public API) lives in
`test_changeset_builder`'s `describes` array. Per-method unit tests for the per-action LabelFor
branches live in `plan/labeler_test.go` and ship with this component's implementation task.
