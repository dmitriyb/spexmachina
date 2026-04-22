# Snapshot saving

Implementation notes for `SnapshotSaver.Save`.

## Full Implementation

```go
package ingest

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/dmitriyb/spexmachina/merkle"
)

type Saver struct {
    SpecDir      string
    SnapshotPath string
}

func (s *Saver) Save(status string) (bool, error) {
    if status != "complete" {
        return false, nil
    }
    dir := s.SpecDir
    if dir == "" {
        dir = "./spec"
    }
    path := s.SnapshotPath
    if path == "" {
        path = dir + "/.snapshot.json"
    }

    tree, err := merkle.BuildTree(dir)
    if err != nil {
        return false, fmt.Errorf("ingest: snapshot: build: %w", err)
    }

    if err := writeAtomic(path, tree); err != nil {
        return false, fmt.Errorf("ingest: snapshot: write %s: %w", path, err)
    }
    return true, nil
}

func writeAtomic(path string, tree merkle.Tree) error {
    tmp := path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    defer func() {
        // Best-effort cleanup if rename never happens.
        _ = os.Remove(tmp)
    }()

    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    enc.SetEscapeHTML(false)
    if err := enc.Encode(tree); err != nil {
        _ = f.Close()
        return err
    }
    if err := f.Sync(); err != nil {
        _ = f.Close()
        return err
    }
    if err := f.Close(); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

## Defer-Remove Idiom

The `defer os.Remove(tmp)` is a best-effort cleanup for the case where Rename doesn't happen (early return due to an error in encoding/sync). If Rename DID happen, `tmp` no longer exists, so the deferred Remove becomes a no-op (os.Remove returns an error which we ignore).

## Sync-Before-Rename

`f.Sync()` ensures the data is flushed to disk before the rename. On some filesystems (especially network-attached), rename atomically publishes the new file; syncing the bytes first guarantees that a crash after rename leaves the file with the correct content, not zero bytes.

## Canonical Format

The merkle module's `json.Encoder` output format is the canonical snapshot format. No post-processing in SnapshotSaver — it just serializes what merkle.BuildTree returns.

## Integration

IngestCommand wires:

```go
saver := &ingest.Saver{SpecDir: "./spec"}
wrote, err := saver.Save(receipts.Status)
```

`wrote` flows into the final summary as `snapshot_saved`.

## Error Surface

Any failure returns `(false, err)`. Caller decides whether to treat a snapshot-save failure as a hard error (ingest exits 1 with stderr). Mapping store has already been committed at this point — a snapshot-save failure doesn't retroactively invalidate the reconciliation, it just means the snapshot needs regeneration (re-run `spex emit` against the fresh spec; the new snapshot will be computed from scratch on the next complete run).
