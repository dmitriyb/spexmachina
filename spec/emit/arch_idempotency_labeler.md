# IdempotencyLabeler

Assigns each create op the `idempotency.label` [[0d468e176aaf|the adapter matches against the
tracker before it creates anything]], so a re-run re-attaches to the task the last run made
instead of making a second one. The label format depends on the action class — three branches:
node-bearing (fresh and modify-pair alike), cleanup, epic. Every branch is a pure function of the
action: no cursor, no store read, no state.

## Responsibilities

- For each create action, return the appropriate label per the per-action rules below. Labeler is
  **per-action**, not per-batch: the label depends on what the action looks like, not on the
  action's position in the ordered batch.
- Surface assigned labels to ChangesetBuilder so each op's JSON carries its label.

## Per-action rules

| Action class | Discriminator | Label format |
|--------------|---------------|--------------|
| Node-bearing | the action targets a spec node — fresh creates and modify-pair creates alike | `spex:<spec_node_id>` — the node's own identity hash |
| Cleanup      | the action's reason starts with `"Code cleanup:"` | `spex:cleanup-<spec_node_id>` |
| Epic         | `spec_node_kind` is `proposal_epic` | `spex:<proposal-slug>` |

### Why fresh and modify-pair share one branch

The retired integer-label scheme needed three behaviors: fresh creates consumed a counter,
modify-pairs looked the old task's record up to reuse its integer, and the two could desynchronize
(the reconciler's invariant 3 existed to catch exactly that). Under `spex:<spec_node_id>` the
distinction dissolves: the node's identity hash does not change across a modify pair, so the
replacement create carries the same label as the original *by construction*, with no lookup and no
store dependency. The adapter's label probe still resolves to the open task when one exists, which
is all modify-pair detection ever needed from the label.

### Why cleanup uses a distinct prefix

A cleanup task and the node's ordinary task must not collide on one label: the ordinary task
tracks building the node, the cleanup task tracks dismantling what the removed node left behind,
and both can exist in the tracker's history for the same hash. The `cleanup-` prefix keeps the two
keyspaces disjoint while still carrying the removed node's identity — which the journal's
`removed` event makes resolvable from day one.

## Why label-at-emit-time, not adapter-time

The label is the adapter's idempotency key — it checks the tracker for a task carrying this label
before creating. If two emit runs were to assign labels for the same spec node differently,
re-runs could duplicate tasks. Computing labels deterministically at emit time keeps them stable —
and because every branch is a pure function of the action, the determinism no longer depends on
any persisted state. A failed emit that never reaches ingest changes nothing anywhere; the next
emit derives byte-identical labels from the same input.

Partial runs compose the same way. Emit three fresh creates; the adapter lands the first two and
fails the third; ingest appends the two pairings and — because the run was partial — writes no
snapshot. The next diff recomputes against the same baseline, the impact report carries only the
op that never landed, and its label is the same `spex:<spec_node_id>` again. The tracker holds no
open task with that label, so the create goes through.

## Interface

Labeler is asked for one label at a time and answers with one label, or with an error for an
action so malformed it has no spec_node_id to read. The builder passes that error straight up, so
`spex emit` fails without writing a changeset rather than emitting an op whose idempotency key is
a guess.

An earlier design reserved a flat block of N integer labels up front. It could not express
per-action rules; the design after it read a cursor from the mapping store and kept invariant
machinery to stop the cursor and the records desynchronizing. Deriving the label from the op
retired both — there is nothing left to reserve, read, or desynchronize.

## Conflict Avoidance

- A label is only assigned to a *new* create op. Close ops, label ops, and tag ops do not get
  `spex:<...>` assigned (they reference existing tasks by `bead_id` or target spec_node_id).
- Node-bearing labels are `spex:<12-hex>`; the hash has exactly one spelling, so a label
  comparison is a string comparison.
- The three branches produce disjoint label shapes (`spex:<hash>`, `spex:cleanup-<hash>`,
  `spex:<slug>` — a slug is never 12 bare hex characters), so no two action classes can collide
  on one tracker label.
- Legacy `spex:<int>` labels on closed tasks are inert: no branch emits an integer form, and the
  adapter's probe matches exact strings, so old labels can never capture a new create.
- Cleanup labels are pure functions of `action.SpecNodeID`; identical inputs across runs produce
  identical labels. The adapter's label lookup gives idempotent re-runs.

## Test surface

IdempotencyLabeler has no public API surface independent of `ChangesetBuilder` — only Builder
consumes it. Cross-component integration coverage (Labeler paired with Sorter and Builder,
exercising the three label shapes through `Builder.Build()`'s public API) lives in
`test_changeset_builder`'s `describes` array. Per-method unit tests for the per-action LabelFor
branches live in `emit/labeler_test.go` and ship with this component's implementation bead.
