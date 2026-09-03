# EventBuilder

Constructs journal lines per action class. [[2b5158af774b|Reconciler]] builds it once per run and
calls it once per op (and once per absorbed entry); the builder answers with the lines the op
implies — [[539030e8c5a4|an ok create yields a change event and the task_created receipt pairing
it, an ok close yields the removed or modified event and its task_closed, and a receipt
reporting an error yields nothing]]. Cleanup creates pair with the removal event they answer —
already journaled, or minted by the cleanup itself; epic creates pair with the proposal's
`registered` event; an ok retarget yields [[7191a50f7447|the `modified` change event plus its
`task_retargeted` receipt]]; the changeset's `absorbed` array yields [[7900dcd38c4a|one `modified`
event per entry, closed by one `refresh` receipt naming them]] — every receipt references an
event.

## Per-Run State

The builder owns the state every construction path shares, handed to it once at setup instead of
threaded through each call:

- the **eid predicate** — answers whether an eid is already present, in the journal *or* in the
  batch constructed so far. It is mutated as the batch grows, so a mid-batch collision is caught
  exactly as a journal-side duplicate is;
- the **journal fold** — live pairings, epic slugs, legacy lines behind the read-only branch,
  and each node's latest change event, which is what tells a create whether its node is new,
  modified or coming back from a removal;
- the **registered-by-stem map** — the journal's `registered` events keyed by proposal stem, for
  epic creates.

No op's lines depend on another op in the batch. The retired modify pair made a close's lines
depend on its paired create, and a cleanup's referent depend on a close elsewhere in the batch;
both dependencies are gone with the pair, so the builder needs no batch-wide pre-pass and no
"already handled" bookkeeping — each op is constructed from itself, the journal and the spec
graph.

Any line whose derived eid the predicate already answers for is dropped, which is what makes the
batch idempotent: [[fd6f08ef34fa|re-processing the same changeset+receipts pair constructs the
same lines, finds every one present, and yields nothing]].

## Per-Op Construction Table

| Op type | spec_node_kind | Receipt status | Journal lines constructed |
|---------|----------------|----------------|---------------------------|
| create  | `proposal_epic` | ok            | one `task_created` whose `for` is the proposal's `registered` event — no change event (see Proposal-Epic Ops) |
| create  | `cleanup`       | ok            | one `task_created` whose `for` is the `removed` event for the op's spec_node_id — the journal's, or one this op mints (see Cleanup-Create Ops) |
| create  | other          | ok             | the change event — `added` or `modified`, derived from the journal (see Node-Bearing Creates) — plus a `task_created` with the receipt's task id |
| create  | any            | error/skipped  | nothing |
| close   | —              | ok (reason="Spec node removed")  | the `removed` change event plus a `task_closed` |
| close   | —              | ok (reason starts "Spec node modified") | the `modified` change event plus a `task_closed`, built from the close alone — no `task_created` (see Fold-Back Closes) |
| close   | —              | error          | nothing |
| retarget | —             | ok             | the `modified` change event plus a `task_retargeted` with the receipt's task id (see Retarget Ops) |
| retarget | —             | error/skipped  | nothing |

Node identity — name, kind, module — comes from the view of the current spec graph the builder is
set up with, because a change event is the only record of that identity once the node is gone;
for a node already gone (a cleanup's, or a removal close's), it comes from the journal's fold
entry instead.

## Node-Bearing Creates

A create op says which node it is for and nothing about the node's past: the same op is emitted
for a brand-new node and for a node whose earlier task finished. The change type is therefore
derived from the journal, not read off the op. The builder looks up the node's latest change
event in the fold:

- no change event at all, or a latest event whose `after` is null (the node was removed and is
  now coming back) → an `added` event, `before` null;
- a latest event carrying an `after` hash → a `modified` event, with that hash as `before`.

The event's eid derives from `(git_head, op_id)`, its node identity and current hash from the op
and the spec graph, its `git_head` and `proposal` from the changeset's top-level fields. One
`task_created` pairs it to the receipt's task id.

A node's earlier pairing is left exactly as it is. No `task_closed` is constructed for the
finished task, because nothing closed it — its implementer finished it, and the journal records
what the pipeline did, never a task's completion. The old pairing stays in the journal as
lineage, and the fold answers with the successor because it is later in the file. A node may
therefore carry several `task_created` lines across its history with no `task_closed` between
them; that is the normal shape of a completed task's successor, and no invariant reads it as a
gap.

## Fold-Back Closes

A close op whose reason starts "Spec node modified" is the shape plan emits for a coupled
`test_section` edit — the section's `describes` dropped to one component, so its coverage folds
into that component going forward and the section's own open task is cancelled. The node itself
still exists — only its hash changed — so the close builds the `modified` event and its
`task_closed` on its own: identity and prior hash come from the journal's live fold entry for the
closing task (exactly as a removed node's identity is resolved), and current name, module, path
and hash come from the spec graph. No `task_created` is built — there is no successor task.

A close naming a task the journal has never heard of (no fold entry at all) has no identity to
build from and is refused as a malformed changeset. "Spec node removed" closes construct the
`removed` event directly, from the fold entry's identity and prior hash and an `after` of null.

## Retarget Ops

A retarget op is what plan emits for a modified node whose open, unclaimed task moves to the new
state instead of being followed by a successor. Its ok receipt constructs a pair: the `modified`
change event — eid derived from `(git_head, op_id)` exactly as a create op's event is, node
identity and hashes from the op, name/kind/module from the spec graph — plus a `task_retargeted`
receipt whose `for` is that event's eid and whose `task_id` is the existing task the op targeted.
No `task_closed` and no `task_created` accompany the pair: nothing died and nothing was born. The
old pairing stays in the journal as history, and the fold answers with the retargeted event
because `task_retargeted` is task-bearing and later in the file. Both lines dedup by derived event
id, so re-processing the batch appends nothing.

## Absorbed Entries

The changeset's top-level `absorbed` array is processed alongside the ops, from the changeset
alone — no receipt corresponds to an absorbed entry, because the adapter ignores the array and no
tracker work happened. For each entry, the builder constructs one `modified` change event: node
identity and before/after hashes off the entry itself, name/kind/module from the spec graph, the
event's `git_head` and `proposal` from the changeset's own top-level fields — exactly as op-born
events take them — and its eid derived from `(node, before, after)` exactly as refresh-born events
are, because an absorbed entry has no create op to key from. The batch's absorbed events are
closed by one `refresh` receipt naming exactly their eids, the same shape a whole-run refresh
appends, so a reader of the journal cannot tell per-node absorption from refresh-mode absorption
except by what else the run appended. No task receipt is constructed — a `refresh` receipt is not
task-bearing, so the node's pairing keeps its sourcing event and the task owes nothing.

Absorbed entries are not receipt-gated: they describe spec state, not tracker work, so they append
on partial runs too. Re-absorption is harmless by construction — after a partial run the snapshot
is unsaved, the next diff re-reports the node, the operator marks it again, and the re-derived
eids find their lines already present and append nothing. An empty `absorbed` array constructs
nothing, not an empty receipt.

## Proposal-Epic Ops

Create ops with `spec_node_kind == "proposal_epic"` represent a proposal as a tracker task. Their
`spec_node_id` is the **proposal stem** (e.g. `2026-04-29-decouple-contract-gaps`), NOT a
12-character identity hash, because proposals live as markdown files under `spec/proposals/` and
have no spec-graph node.

The builder MUST treat them as a distinct case:

- **Skip** the spec-graph metadata lookup. The stem is not an identity hash; the lookup would
  always fail with `"spec graph: no node <stem>"` and abort the run with exit 2.
- **Construct** only a receipt: `{"event": "task_created", "for": "<registered eid>",
  "task_id": <receipt's>}`. The referent is the proposal's `registered` event, answered by the
  registered-by-stem map — registration appended it when the lifecycle opened (a one-shot
  backfill line covers proposals registered before the event type existed). No change event
  exists or is invented — an epic is not born from a diff entry; it is born from a registration,
  and the registered event is that fact on the record. The epic's receipt dedups by its
  registered-event referent like every other line.
- An epic op for a proposal with no `registered` event in the journal is an invariant failure —
  plan refuses to build such an op, so its arrival marks a malformed changeset.

Legacy epic receipts carrying `proposal: <stem>` and no `for` are read as inert history behind
the fold's read-only legacy branch; none is ever constructed anew. The fold lists epic tasks
keyed by their slug — carried by the registered event, or by the legacy receipt itself — which is
how a re-run recognises an epic that already exists.

## Cleanup-Create Ops

Create ops with `spec_node_kind == "cleanup"` represent a code-cleanup task for a spec node that
was removed while its own task was already finished. Their `spec_node_id` carries the identity
hash of the now-removed node, and their label is `spex:<eid>` of the removal event they answer —
label and `task_created` referent are the same event.

The builder MUST treat them as a distinct case:

- **Discriminate on the op's spec node kind before anything else** — before the spec graph is
  consulted, which no longer holds the node.
- **Resolve the referent from the node's latest change event.** If that event is a `removed`
  one — the removal landed in an earlier batch whose cleanup never did — the `task_created`'s
  `for` is its eid, and no new change event is constructed. Otherwise the cleanup mints the
  removal itself: one `removed` change event with eid derived from the cleanup op's own
  `(git_head, op_id)`, `before` taken from the latest event's `after`, `after` null, and
  name/kind/module from the fold entry — the biography that outlives the node — followed by the
  `task_created` naming it. No close op accompanies a cleanup, because the node's task was
  finished and there was nothing live to close; the cleanup is the one op in the batch that
  knows the node is gone, so it is the one that says so.
- A cleanup op whose hash the journal has never seen at all is an invariant failure, not a
  fallback — a cleanup for a node with no history is a malformed changeset.

The finished task that the removed node had gets no `task_closed`, for the same reason a
successor create constructs none: the journal does not record completion.

## Error Surface

Construction errors are wrapped with `"ingest: reconcile: ..."` context and name the offending
spec_node_id, op_id or eid. A construction failure refuses the run before anything is checked or
written — the builder never emits a half-built pair.
