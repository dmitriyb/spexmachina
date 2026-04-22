package apply

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// SaveSnapshot computes a fresh merkle tree from the spec directory and writes
// it to <specDir>/.snapshot.json. The createdAt parameter controls the
// snapshot timestamp for deterministic output. SaveSnapshot is the immediate
// successor of the close-obsoletes phase in the apply flow: it runs last,
// after label, create, and close phases all succeed. If any earlier phase
// fails the snapshot is not updated, so the next diff re-detects the same
// changes and retries the failed actions.
// ctx is currently unused but reserved for future cancellation support.
func SaveSnapshot(ctx context.Context, specDir string, createdAt time.Time) error {
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return fmt.Errorf("apply: build tree for snapshot: %w", err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, createdAt); err != nil {
		return fmt.Errorf("apply: save snapshot: %w", err)
	}
	return nil
}
