package merkle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is the on-disk JSON representation of a merkle tree.
// Nodes are stored in a flat map keyed by spec ID for O(1) lookup and easy diffing.
type Snapshot struct {
	RootHash  string                   `json:"root_hash"`
	RootKey   string                   `json:"root_key"`
	CreatedAt time.Time                `json:"created_at"`
	Nodes     map[string]*SnapshotNode `json:"nodes"`
}

// SnapshotNode is a single entry in the flat snapshot map.
type SnapshotNode struct {
	Hash     string   `json:"hash"`
	Type     string   `json:"type"`
	NodeType string   `json:"node_type,omitempty"`
	Module   string   `json:"module,omitempty"`
	Children []string `json:"children,omitempty"`
}

// EncodeSnapshot flattens the tree into the canonical snapshot document
// and returns its bytes: two-space indented JSON with a trailing newline.
// createdAt is a parameter rather than a clock reading, so the same tree
// and timestamp always produce the same bytes.
//
// Every writer of spec/.snapshot.json goes through here, whatever it then
// does with the bytes. A second implementation of this walk is how two
// writers of one format drift apart while both keep passing their own
// tests — see arch_snapshot_store.md, "Read vs. write call sites".
func EncodeSnapshot(tree *Node, createdAt time.Time) ([]byte, error) {
	snap := &Snapshot{
		RootHash:  tree.Hash,
		RootKey:   tree.Key,
		CreatedAt: createdAt,
		Nodes:     make(map[string]*SnapshotNode),
	}
	flattenTreeToSnapshot(snap.Nodes, tree)

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("merkle: encode snapshot: %w", err)
	}
	return append(data, '\n'), nil
}

// Save writes the merkle tree to a snapshot file as JSON, replacing the
// destination in place. It carries no atomicity guarantee, so it has no
// production caller: ingest's SnapshotSaver pairs EncodeSnapshot with its
// own temp-file-and-rename write instead.
func Save(tree *Node, path string, createdAt time.Time) error {
	data, err := EncodeSnapshot(tree, createdAt)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("merkle: save snapshot %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("merkle: save snapshot %s: %w", path, err)
	}
	return nil
}

// Load reads a snapshot file and reconstructs the merkle tree.
//
// When the snapshot file does not exist, Load returns the empty-tree
// baseline (see EmptyTree) with a nil error. This is the contract from
// spec/merkle/arch_snapshot_store.md: spex diff treats a missing
// snapshot as the empty baseline so the first run on a fresh project
// reports every leaf as "added" without a separate bootstrap step.
// All other read failures (permission denied, I/O errors, malformed
// JSON, missing root entry) still surface as errors.
func Load(path string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return EmptyTree(), nil
		}
		return nil, fmt.Errorf("merkle: load snapshot %s: %w", path, err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("merkle: parse snapshot %s: %w", path, err)
	}

	root, err := rebuildTree(&snap)
	if err != nil {
		return nil, fmt.Errorf("merkle: load snapshot %s: %w", path, err)
	}
	return root, nil
}

// flattenTreeToSnapshot walks the tree recursively and populates the flat node map.
// Each node's Key is used directly as the map key.
func flattenTreeToSnapshot(nodes map[string]*SnapshotNode, n *Node) {
	sn := &SnapshotNode{
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
		flattenTreeToSnapshot(nodes, child)
	}
}

// rebuildTree reconstructs the Node tree from a flat snapshot.
func rebuildTree(snap *Snapshot) (*Node, error) {
	if snap.RootKey == "" {
		return nil, fmt.Errorf("missing root_key in snapshot")
	}
	if _, ok := snap.Nodes[snap.RootKey]; !ok {
		return nil, fmt.Errorf("root_key %q not found in nodes", snap.RootKey)
	}

	return rebuildNode(snap.Nodes, snap.RootKey)
}

func rebuildNode(nodes map[string]*SnapshotNode, key string) (*Node, error) {
	sn, ok := nodes[key]
	if !ok {
		return nil, fmt.Errorf("missing node %q", key)
	}

	n := &Node{
		Key:      key,
		Hash:     sn.Hash,
		Type:     sn.Type,
		NodeType: sn.NodeType,
		Module:   sn.Module,
	}

	for _, childKey := range sn.Children {
		child, err := rebuildNode(nodes, childKey)
		if err != nil {
			return nil, err
		}
		n.Children = append(n.Children, child)
	}

	return n, nil
}
