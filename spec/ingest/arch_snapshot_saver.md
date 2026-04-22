# SnapshotSaver

Writes `spec/.snapshot.json` with the current merkle tree **iff** receipts top-level status is `complete`. Partial runs leave the snapshot file untouched so the next emit recomputes against the same baseline.

## Responsibilities

- Accept the completeness gate: `complete` → write; `partial` → skip.
- Compute the current merkle tree by invoking the `merkle` module against the spec directory.
- Serialize the snapshot with canonical JSON encoding.
- Atomically write via temp file + rename.

## Interface

```go
type Saver struct {
    SpecDir     string // default "./spec"
    SnapshotPath string // default "./spec/.snapshot.json"
}

// Save writes the snapshot iff status == "complete".
// Returns (wrote bool, err error).
func (s *Saver) Save(status string) (bool, error)
```

## Gate Logic

```go
func (s *Saver) Save(status string) (bool, error) {
    if status != "complete" {
        return false, nil
    }
    tree, err := merkle.BuildTree(s.SpecDir)
    if err != nil {
        return false, fmt.Errorf("ingest: snapshot: build tree: %w", err)
    }
    if err := writeAtomic(s.SnapshotPath, tree); err != nil {
        return false, err
    }
    return true, nil
}
```

## Why the Gate

- A partial run means some ops succeeded and some didn't. The mapping store reflects the partial state (records for ok creates, no records for error creates).
- If we wrote the snapshot on partial, the next `spex emit` would diff against the new (partial) baseline and miss the ops that still need to run.
- Leaving the snapshot untouched means the next emit diffs the spec against the ORIGINAL baseline. The resulting impact report re-includes the failed ops. Emit re-reserves labels for those ops (counter didn't advance past them since ingest didn't commit their records). Adapter re-runs. Ingest reconciles. If the second run is complete, snapshot gets saved.

This is the "unfinished operations resurface through the idempotency path" mechanism described in the proposal.

## Atomic Write

```go
func writeAtomic(path string, tree merkle.Tree) error {
    tmp := path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return fmt.Errorf("ingest: snapshot: create: %w", err)
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(tree); err != nil {
        _ = f.Close()
        _ = os.Remove(tmp)
        return err
    }
    if err := f.Close(); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

`os.Rename` is atomic on POSIX filesystems (same-device rename). A crash between `Create` and `Rename` leaves the temp file behind but the original snapshot untouched — subsequent runs clean up the .tmp file.

## Snapshot Format

Inherited from the `merkle` module. Each node: `{id, kind, hash}`. Root hash is derivable from the tree structure. See `spec/merkle/module.json` for the authoritative schema.

## Non-Responsibilities

- Does NOT decide which ops to reconcile — that's `Reconciler`'s job.
- Does NOT touch `.bead-map.json` — separate concern.
- Does NOT clean up stale `.tmp` files from prior crashes — startup-time cleanup is a separate operational concern (not tracked here).
