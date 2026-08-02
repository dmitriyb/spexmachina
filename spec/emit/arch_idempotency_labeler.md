# IdempotencyLabeler

Assigns each create op the `idempotency.label` [[0d468e176aaf|the adapter
matches against the tracker before it creates anything]], so a re-run
re-attaches to the bead the last run made instead of making a second one.
The label format depends on the action class — three branches:
modify-pair, cleanup, fresh.

## Responsibilities

- For each create action, return the appropriate label per the per-action
  rules below. Labeler is **per-action**, not per-batch: the label depends
  on what the action looks like, not on the action's position in the
  ordered batch.
- Maintain an in-memory cursor for fresh creates only. The cursor reads the
  mapping store's persisted counter on first use; emit never writes that
  counter back, because advancing it is ingest's job.
- Surface assigned labels to ChangesetBuilder so each op's JSON carries
  its label.

## Per-action rules

| Action class | Discriminator | Label format | Cursor effect |
|--------------|---------------|--------------|---------------|
| Modify-pair  | the action names an old bead AND its reason does NOT start with `"Code cleanup:"` | `spex:<id of the record already bound to that old bead>` | Cursor does NOT advance |
| Cleanup      | the action's reason starts with `"Code cleanup:"` (the same discriminator the retired apply path used) | `spex:cleanup-<spec_node_id>` | Cursor does NOT advance |
| Fresh        | All other creates (no old bead, no cleanup reason) | `spex:<cursor>` and advance the cursor | Cursor advances by 1 |

### Why modify-pair reuses the existing record-id

Ingest's reconciler detects a modify-pair by reading the record id back
out of `spex:<n>` and looking for a record already sitting at that id. If
emit hands a modify-pair create a fresh cursor value, the reconciler
finds no record there — it inserts a parallel record at the new id, and
the original record, still pointing at the closed bead, is orphaned.
Invariant 3 fails (`modified bead X record still points to old bead_id`).
Reusing the existing record-id is what lets the reconciler take its
modify-pair update branch and rebind `bead_id` to the new value.

### Why cleanup uses a per-spec-node-id label

Pre-decouple `createCleanupBead` had NO idempotency check: re-runs would
create duplicate cleanup beads. Post-decouple, the adapter's idempotency
check before `br create` uses the op's `idempotency.label`. A label of
the form `spex:cleanup-<spec_node_id>` is unique per removed-node identity
hash and gives clean re-run idempotency. It does NOT consume the cursor
because cleanup beads have no mapping record and therefore no record-id
to allocate.

## Why label-at-emit-time, not adapter-time

The label is the adapter's idempotency key — it checks the tracker for a
bead carrying this label before creating. If two emit runs were to assign
labels for the same spec node differently, re-runs could duplicate beads.
Computing labels deterministically at emit time keeps them stable.

The counter is advanced by ingest, not by emit. Emit only **reads** it for
fresh creates, so a failed emit that never reaches ingest leaves the
counter untouched and the next emit re-derives the same labels: modify-pair
labels find the same existing record, cleanup labels are pure functions of
`spec_node_id`, and fresh labels start from the same counter value. Emit is
deterministic over its inputs.

Ingest raises the counter to one past the highest record id it committed,
so a **partial** run advances it too. Emit three fresh creates as
`spex:42`, `spex:43`, `spex:44`; the adapter lands the first two and fails
the third; ingest commits records 42 and 43 and leaves the counter at 44,
and — because the run was partial — writes no snapshot. The next diff
therefore recomputes against the same baseline, the impact report carries
only the op that never landed, and its label is `spex:44` again. Nothing in
the tracker carries that label yet, so the create goes through as a fresh
bead.

## Interface

Labeler is asked for one label at a time and answers with one label, or
with an error: the mapping store could not be read, or a modify-pair names
an old bead that has no record. The builder passes that error straight up,
so `spex emit` fails without writing a changeset rather than emitting an
op whose idempotency key is a guess.

The next label can also be read without being consumed, so a caller can
see where the cursor stands without spending an id.

An earlier design reserved a flat block of N labels up front. It cannot
express these rules: a modify-pair label and a cleanup label are functions
of the specific action, not of a position in a sequence, so only one of
the three branches would ever have fitted in the block.

## Conflict Avoidance

- A label is only assigned to a *new* create op. Close ops, label ops, and
  tag ops do not get `spex:<...>` assigned (they reference existing beads
  by `bead_id` or target spec_node_id).
- Fresh-create cursor starts at the store's persisted `next_id` counter,
  read once when the store is loaded. That counter is monotonic and never
  recomputed from the records present, so deleting the highest record does
  not lower it and there is no re-use of closed-record IDs.
- Fresh and modify-pair labels are `spex:<n>` with `<n>` a positive decimal
  integer written without leading zeros, so one record id has exactly one
  spelling and a label comparison is a string comparison.
- Modify-pair labels reuse the existing record's id by design — the
  Reconciler's modify-pair detection requires this match. The new-bead's
  ingestion *replaces* the bead_id in that record; the record-id stays
  the same.
- Cleanup labels are pure functions of `action.SpecNodeID`; identical
  inputs across runs produce identical labels. The adapter's `br list
  --json --label spex:cleanup-<spec_node_id>` lookup gives idempotent
  re-runs.

## Test surface

IdempotencyLabeler has no public API surface independent of
`ChangesetBuilder` — only Builder consumes it. Cross-component integration
coverage (Labeler paired with Sorter and Builder, exercising modify-pair
record-id reuse, cleanup label format, and fresh-cursor allocation through
`Builder.Build()`'s public API) lives in `test_changeset_builder`'s
`describes` array. Per-method unit tests for the per-action LabelFor
branches live in `emit/labeler_test.go` and ship with this component's
implementation bead.
