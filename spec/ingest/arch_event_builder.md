# EventBuilder

Constructs journal lines per action class. [[2b5158af774b|Reconciler]] builds it once per run and
calls it once per op (and once per absorbed entry); the builder answers with the lines the op
implies — [[539030e8c5a4|an ok create yields a change event and the task_created receipt pairing
it, an ok close on a removed node yields the removed event and its task_closed, and a receipt
reporting an error yields nothing]]. Cleanup creates pair with the removal event they answer —
prior-batch or same-batch — instead of a fresh one; epic creates pair with the proposal's
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
- the **journal fold** — live pairings, epic slugs, legacy lines behind the read-only branch;
- the **same-batch removals** — the whole batch's ok removal closes, resolved before any op is
  processed, which is what lets a cleanup create find a removal that comes after it in op order;
- the **registered-by-stem map** — the journal's `registered` events keyed by proposal stem, for
  epic creates;
- the **modified-handled set** — which beads' modified pairs an earlier create has already built,
  so the paired close constructs nothing twice.

Any line whose derived eid the predicate already answers for is dropped, which is what makes the
batch idempotent: [[fd6f08ef34fa|re-processing the same changeset+receipts pair constructs the
same lines, finds every one present, and yields nothing]].

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

Node identity — name, kind, module — comes from the view of the current spec graph the builder is
set up with, because a change event is the only record of that identity once the node is gone.

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

## The Modified-Node Pair

Plan generates two ops for a modified node whose pairing's task is closed: a create op labeled
`spex:<eid>` of the pair's `modified` event — the very event this component will mint from the
create op's `(git_head, op_id)` — plus a close op for the old task, in that order (see
`arch_reconciler.md`, "Ordering"). The create op alone carries everything the pair needs: its
`blocks` dep names the old bead directly, and the node's current live fold entry supplies the
prior content hash for `before`. The builder constructs the whole pair while processing the
create — one `modified` change event, a `task_closed` for the retired task and a `task_created`
for its successor, both receipts carrying the pair's `modified` event as their `for`, so the join
needs no scan by task id — without waiting to see the paired close, and records the bead in the
modified-handled set. Nothing is rebound and nothing is deleted — the old pairing stays in the
journal as lineage, and the fold answers with the successor because it is later in the file.

When the paired close is then reached, it constructs nothing of its own — the modified-handled
set says the create already built both receipts. A "Spec node modified" close whose bead was not
claimed by an already-processed create is parked until the whole batch has been processed, then
resolved one of two ways, discriminated on whether a create op for that bead exists in the
changeset at all — not on receipt order:

- **A create op for the bead exists in the changeset, but its receipt was `error` or `skipped`.**
  This is a partial run (`scripts/apply-br.sh` routinely continues past a failed create to close
  the old task anyway): the pair constructs nothing, same as any other errored/skipped create, and
  the rest of the batch still lands.
- **No create op for the bead exists in the changeset at all.** This is the shape the classifier
  emits for a coupled `test_section` edit: an `obsolete` action with no replacement create,
  because the section's bead is folded into its owning component going forward rather than
  replaced. The node itself still exists — only its hash changed — so the close builds the
  `modified` event and its `task_closed` on its own: identity and prior hash come from the
  journal's live fold entry for the closing bead (exactly as a removed node's identity is
  resolved), and current name/module/path/hash come from the spec graph. No `task_created` is
  built — there is no successor task to pair.

A close naming a bead the journal has never heard of (no fold entry at all) has no identity to
build from and is refused as a malformed changeset. "Spec node removed" closes construct the
`removed` event directly and never park.

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
was removed in a prior or concurrent batch. Their `spec_node_id` carries the identity hash of the
now-removed node, and their label is `spex:<eid>` of the removal event they answer — label and
`task_created` referent are the same event.

The builder MUST treat them as a distinct case:

- **Discriminate on the op's spec node kind before anything else** — before the spec graph is
  consulted, which no longer holds the node.
- **Construct** a `task_created` whose `for` is the journal's latest `removed` event for that
  hash — the cleanup task is born pointing at the removal it answers, which is what makes its
  label resolvable from day one. If the journal holds no removed event for the hash (the removal
  is in this same batch), the referent is the removal's eid, answered by the same-batch removals
  in the per-run state — resolved from the whole batch's ok removal closes before any op is
  processed, not from a scan of lines already appended to the batch. The changeset lists the
  cleanup create before the close that performs its removal (see `arch_reconciler.md`,
  "Ordering"), so at the point the create is processed neither the on-disk fold nor the
  batch-so-far shows the node as removed yet; only a batch-wide, order-independent resolution
  gets the referent right.
- A cleanup op whose hash matches no removed event anywhere is an invariant failure, not a
  fallback — a cleanup for a removal that never happened is a malformed changeset.

## Error Surface

Construction errors are wrapped with `"ingest: reconcile: ..."` context and name the offending
spec_node_id, op_id or eid. A construction failure refuses the run before anything is checked or
written — the builder never emits a half-built pair.
