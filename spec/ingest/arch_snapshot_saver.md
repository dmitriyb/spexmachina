# SnapshotSaver

[[cf47671793fa|Writes `spec/.snapshot.json` with the current merkle tree **iff** the receipts' top-level status is `complete`]]. Partial runs leave the snapshot file untouched so the next emit recomputes against the same baseline.

## Responsibilities

- Accept the completeness gate: `complete` → write; `partial` → skip.
- Compute the current merkle tree by invoking the `merkle` module against the spec directory.
- Ask the `merkle` module for the snapshot's canonical bytes rather than encoding them here.
- Atomically write via temp file + rename.

## Interface

The saver is configured with two paths: the spec directory it hashes, and the snapshot file it writes. Both carry defaults — the spec root when the directory is unset, and `.snapshot.json` inside that directory when the path is unset. The shipped command sets only the directory, to whatever `--spec-dir` resolved to, so the snapshot lands beside the spec it describes.

One call hands it the run's top-level status. It answers whether it wrote, or it fails. It is never told *what* to write: the tree is computed from the spec directory as it stands at the moment of the call.

## Gate Logic

Any status other than `complete` skips the write and reports that nothing was written. Nothing is read and no tree is built on that path — the previous `spec/.snapshot.json` is left byte-for-byte as it was. Only on `complete` is the current merkle tree computed and written.

That yes-or-no answer is what the run's summary reports as `snapshot_saved`, so a caller reading stdout can tell a saved snapshot from a skipped one without stat-ing the file.

## Why the Gate

- A partial run means some ops succeeded and some didn't. The mapping store reflects the partial state (records for ok creates, no records for error creates).
- If we wrote the snapshot on partial, the next `spex emit` would diff against the new (partial) baseline and miss the ops that still need to run.
- Leaving the snapshot untouched means the next emit diffs the spec against the ORIGINAL baseline. The resulting impact report re-includes the failed ops. Emit re-reserves labels for those ops (counter didn't advance past them since ingest didn't commit their records). Adapter re-runs. Ingest reconciles. If the second run is complete, snapshot gets saved.

This is the "unfinished operations resurface through the idempotency path" mechanism described in the proposal.

## Atomic Write

The snapshot is encoded in full by the `merkle` module, written to a temp file beside the destination, flushed, and only then renamed into place. Rename is atomic on POSIX filesystems for a same-device move, so a reader of `spec/.snapshot.json` sees either the previous snapshot or the new one and never a splice of the two. [[ffab5d1337ac|The destination is never opened for truncation and rewritten in place]].

Flushing before the rename is what carries that guarantee across a crash: the bytes are on disk before the name points at them, so a crash immediately after the rename cannot leave a snapshot that is empty or half-written.

A write that fails part-way removes its own temp file, so a failed save leaves nothing behind but the untouched original. A crash can still strand one; the next save writes to the same name and renames it away.

## Snapshot Format

Inherited from the `merkle` module, and not just in shape: the bytes are the ones merkle's encoder returns, so there is one implementation of this format and no second walk to keep in step. The file records the root key, the root hash, the time it was written, and a flat map holding one entry per node in the tree — leaves, modules and the project root alike. Most keys are the node's identity hash; the exceptions are the project root, keyed `project`, and the envelope leaves, keyed `meta/project` and `meta/<module identity hash>`. Each entry carries that node's hash and what kind of node it is, plus its owning module and the keys of its children where those apply. See `spec/merkle/module.json` for the authoritative schema.

## Non-Responsibilities

- Does NOT decide which ops to reconcile — that's `Reconciler`'s job.
- Does NOT touch `.bead-map.json` — separate concern.
- Does NOT clean up stale `.tmp` files from prior crashes — startup-time cleanup is a separate operational concern (not tracked here).
