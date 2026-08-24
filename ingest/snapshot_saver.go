package ingest

import (
	"fmt"
	"os"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Saver writes the snapshot from the current merkle tree, gated on the
// receipts top-level status. Partial runs leave the snapshot untouched
// so the next spex plan diffs against the original baseline and
// resurfaces unfinished ops through the idempotency path.
type Saver struct {
	// SpecDir is the spec root passed to merkle.BuildTree.
	SpecDir string
	// SnapshotPath is the destination for the snapshot file. The shipped
	// command sets it to the location inside .spex/ the lifecycle
	// pre-flight resolved, so this writer computes no location of its
	// own.
	SnapshotPath string
	// Now is the timestamp source for the snapshot's created_at field.
	// Defaults to time.Now when nil. Tests inject a fixed clock to pin
	// byte-equality assertions.
	Now func() time.Time
}

// Save writes the snapshot iff status == adapters.StatusComplete.
// Returns (true, nil) when the snapshot was written, (false, nil) when
// the gate skipped the write, and (false, err) on any failure to build
// or persist the tree — including an unset SpecDir or SnapshotPath,
// which are caller errors rather than a fallback.
func (s *Saver) Save(status string) (bool, error) {
	if status != adapters.StatusComplete {
		return false, nil
	}
	if s.SpecDir == "" {
		return false, fmt.Errorf("ingest: snapshot: SpecDir is required")
	}
	if s.SnapshotPath == "" {
		return false, fmt.Errorf("ingest: snapshot: SnapshotPath is required")
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}

	tree, err := merkle.BuildTree(s.SpecDir)
	if err != nil {
		return false, fmt.Errorf("ingest: snapshot: build tree: %w", err)
	}
	if err := writeAtomic(s.SnapshotPath, tree, now()); err != nil {
		return false, fmt.Errorf("ingest: snapshot: write %s: %w", s.SnapshotPath, err)
	}
	return true, nil
}

// writeAtomic serializes the merkle tree into a temp file alongside the
// destination and renames into place. A crash before rename leaves the
// destination untouched; the temp file is best-effort cleaned up on
// every failure path so a poisoned .tmp does not accumulate.
func writeAtomic(path string, tree *merkle.Node, createdAt time.Time) error {
	data, err := merkle.EncodeSnapshot(tree, createdAt)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := f.Write(data); err != nil {
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
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
