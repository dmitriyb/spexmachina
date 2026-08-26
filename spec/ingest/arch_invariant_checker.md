# InvariantChecker

Asserts [[ee28b5d190ae|the journal consistency invariants]] over the existing journal plus the
constructed batch — after the whole batch exists, before anything reaches disk. [[2b5158af774b|Reconciler]]
runs it as the last step before the commit; a failure refuses the run with the on-disk journal
untouched.

## What Is Checked, and in What Order

The checks run in numeric order, so the first message a caller sees names the most upstream
cause:

1. Every ok create pairs exactly one `task_created` with exactly one referent event — a change
   event in journal or batch, the removal event for cleanups, or the registered event for epics.
   Every ok retarget pairs exactly one `task_retargeted` with its own `modified` event, and the
   batch's absorbed events are closed by exactly one `refresh` receipt naming them. The retired
   "or a proposal slug" arm survives only in legacy lines already on disk.
2. No receipt references an eid that neither the journal nor the batch contains — `for` fields
   and the entries of a `refresh` receipt's `absorbed` list alike.
3. The batch minus already-present lines is what lands — [[fd6f08ef34fa|re-running the same pair
   appends nothing]], because eids derive from `(git_head, op_id)` for op-born events and
   `(node, before, after)` for absorb-born ones. Enforced by construction: the builder's eid
   predicate drops duplicate lines, so this checker has no per-line predicate for it.
4. Snapshot saved iff receipts are complete. Not checked here — this is SnapshotSaver's gate;
   the checker's scope is the journal batch.
5. Every line to be appended validates against the journal-line schema. Owned by JournalEncoder,
   which refuses the line at the write boundary; the checker does not duplicate the schema gate.

Invariants 3, 4 and 5 have no check of their own here because their enforcement lives elsewhere —
3 holds by construction in EventBuilder's eid predicate, 4 is SnapshotSaver's completeness gate,
5 is JournalEncoder's schema gate — and the numbering still names them so it stays aligned with
the five-invariant contract the requirement states and `test_consistency_invariants.md` titles
its sections by.

## Numbering Resolves In-File

Each check carries a doc comment naming the invariant it enforces and the spec section it traces
to, so a reader inside the file can tell what invariant 2 *is* — and why 3, 4 and 5 have no
check here — without leaving the file. The numbering is contractual, not incidental: it traces to the numbered
invariants in the requirement and to the test section titled for them, so no renumbering or
renaming accompanies the checks.

## Failure Shape

A failure names the invariant and the offending op or eid — `"ingest: receipt references unknown
event <eid>"` and kin, wrapped with `"ingest: reconcile: ..."` context — and no candidate batch
is written. One failure is enough to refuse the whole run: there is no partial enforcement, and
no line of a refused batch reaches disk.
