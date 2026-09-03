# SnapshotSaver

[[cf47671793fa|Writes the snapshot, at its resolved location, with the current merkle tree **iff** the receipts' top-level status is `complete`]]. Partial runs leave the snapshot file untouched so the next plan run recomputes against the same baseline.

## Responsibilities

- Accept the completeness gate: `complete` → write; `partial` → skip.
- Compute the current merkle tree by invoking the `merkle` module against the spec directory.
- Ask the `merkle` module for the snapshot's canonical bytes rather than encoding them here.
- Atomically write via temp file + rename.

## Interface

The saver is configured with two paths: the spec directory it hashes, and the snapshot file it writes. The shipped command sets the snapshot path to the location inside `.spex/` that the lifecycle pre-flight resolved, so this writer computes no location of its own.

One call hands it the run's top-level status — read off the receipts v2 document, whose version envelope IngestCommand's pre-flight has already checked, so the saver sees a status and never a version. It answers whether it wrote, or it fails. It is never told *what* to write: the tree is computed from the spec directory as it stands at the moment of the call.

## Gate Logic

Any status other than `complete` skips the write and reports that nothing was written. Nothing is read and no tree is built on that path — the previous snapshot is left byte-for-byte as it was. Only on `complete` does this component compute and write the current merkle tree.

That gate governs this write path, not the file. `spex ingest --mode refresh` reaches the same snapshot file through [[f9033352c13f|RefreshHandler]], which never calls this component's status gate — though it does share the temp-file-plus-rename writer described under "Atomic Write" — and has a gate of its own: an empty changeset, an empty receipts file, and a non-empty journal (the bootstrap guard — a cycle must have completed), after which it writes only if something drifted. A refresh over an already-current spec succeeds, reports `snapshot_saved` false, and leaves the file byte-identical. A reader tracing who moves the snapshot must count both paths.

The two paths also differ in where their provenance comes from. On this component's path the receipts answer to a changeset that carries its own `git_head`, so the saver needs no commit input of its own; the refresh path's empty changeset carries none, which is why the command surface grew a refresh-only `--git-head` for the operator to supply — stamped on the refresh receipt, never consumed here.

That yes-or-no answer is what the run's summary reports as `snapshot_saved`, so a caller reading stdout can tell a saved snapshot from a skipped one without stat-ing the file.

## Why the Gate

- A partial run means some ops succeeded and some didn't. The journal reflects the partial state (pairings for ok creates, nothing for error creates).
- If we wrote the snapshot on partial, the next `spex plan` run would diff against the new (partial) baseline and miss the ops that still need to run.
- Leaving the snapshot untouched means the next run diffs the spec against the ORIGINAL baseline. The resulting changeset re-includes only the ops whose pairings never landed — the journal already pairs everything that succeeded, so nothing landed is re-created; a landed task that has meanwhile finished is still already tracked, because that cell is decided on the journal's hash and not on the task-state artifact. Adapter re-runs. Ingest reconciles — appending nothing for lines already present. If the second run is complete, snapshot gets saved.

This is the "unfinished operations resurface through the idempotency path" mechanism described in the proposal.

## Atomic Write

The snapshot is encoded in full by the `merkle` module, written to a temp file beside the destination, flushed, and only then renamed into place. Rename is atomic on POSIX filesystems for a same-device move, so a reader of the snapshot file sees either the previous snapshot or the new one and never a splice of the two. [[ffab5d1337ac|The destination is never opened for truncation and rewritten in place]].

Flushing before the rename is what carries that guarantee across a crash: the bytes are on disk before the name points at them, so a crash immediately after the rename cannot leave a snapshot that is empty or half-written.

A write that fails part-way removes its own temp file, so a failed save leaves nothing behind but the untouched original. A crash can still strand one; the next save writes to the same name and renames it away.

## Snapshot Format

Inherited from the `merkle` module, and not just in shape: the bytes are the ones merkle's encoder returns, so there is one implementation of this format and no second walk to keep in step. The file records the root key, the root hash, the time it was written, and a flat map holding one entry per node in the tree — leaves, modules and the project root alike. Most keys are the node's identity hash; the exceptions are the project root, keyed `project`, and the envelope leaves, keyed `meta/project` and `meta/<module identity hash>`. Each entry carries that node's hash and what kind of node it is, plus its owning module and the keys of its children where those apply. See `spec/merkle/module.json` for the authoritative schema.

## Non-Responsibilities

- Does NOT decide which ops to reconcile — that's `Reconciler`'s job.
- Does NOT touch the journal — separate concern, appended by Reconciler and RefreshHandler.
- Does NOT clean up stale `.tmp` files from prior crashes — startup-time cleanup is a separate operational concern (not tracked here).
