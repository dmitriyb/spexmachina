package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Saver writes spec/.snapshot.json from the current merkle tree, gated
// on the receipts top-level status. Partial runs leave the snapshot
// untouched so the next emit diffs against the original baseline and
// resurfaces unfinished ops through the idempotency path.
type Saver struct {
	// SpecDir is the spec root passed to merkle.BuildTree. Defaults to
	// "./spec" when empty.
	SpecDir string
	// SnapshotPath is the destination for the snapshot file. Defaults
	// to <SpecDir>/.snapshot.json when empty.
	SnapshotPath string
	// Now is the timestamp source for the snapshot's created_at field.
	// Defaults to time.Now when nil. Tests inject a fixed clock to pin
	// byte-equality assertions.
	Now func() time.Time
}

// Save writes the snapshot iff status == adapters.StatusComplete.
// Returns (true, nil) when the snapshot was written, (false, nil) when
// the gate skipped the write, and (false, err) on any failure to build
// or persist the tree.
func (s *Saver) Save(status string) (bool, error) {
	if status != adapters.StatusComplete {
		return false, nil
	}

	dir := s.SpecDir
	if dir == "" {
		dir = "./spec"
	}
	path := s.SnapshotPath
	if path == "" {
		path = filepath.Join(dir, ".snapshot.json")
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		return false, fmt.Errorf("ingest: snapshot: build tree: %w", err)
	}
	if err := writeAtomic(path, tree, now()); err != nil {
		return false, fmt.Errorf("ingest: snapshot: write %s: %w", path, err)
	}
	return true, nil
}

// writeAtomic serializes the merkle tree into a temp file alongside the
// destination and renames into place. A crash before rename leaves the
// destination untouched; the temp file is best-effort cleaned up on
// every failure path so a poisoned .tmp does not accumulate.
func writeAtomic(path string, tree *merkle.Node, createdAt time.Time) error {
	snap := buildSnapshot(tree, createdAt)
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

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

// buildSnapshot mirrors merkle.Save's serialization shape so the file
// stays compatible with merkle.Load and downstream diff tooling. The
// flat-map representation is the canonical on-disk form per
// snapshot_store.go.
func buildSnapshot(tree *merkle.Node, createdAt time.Time) *merkle.Snapshot {
	snap := &merkle.Snapshot{
		RootHash:  tree.Hash,
		RootKey:   tree.Key,
		CreatedAt: createdAt,
		Nodes:     map[string]*merkle.SnapshotNode{},
	}
	flatten(snap.Nodes, tree)
	return snap
}

func flatten(nodes map[string]*merkle.SnapshotNode, n *merkle.Node) {
	sn := &merkle.SnapshotNode{
		Hash:     n.Hash,
		Type:     n.Type,
		NodeType: n.NodeType,
		Module:   n.Module,
	}
	for _, child := range n.Children {
		sn.Children = append(sn.Children, child.Key)
	}
	nodes[n.Key] = sn
	for _, child := range n.Children {
		flatten(nodes, child)
	}
}
