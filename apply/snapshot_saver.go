package apply

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// SaveSnapshot computes a fresh merkle tree from the spec directory and writes
// it to spec/.snapshot.json. The createdAt parameter controls the snapshot
// timestamp for deterministic output. This should only be called after all
// bead actions have completed successfully.
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
