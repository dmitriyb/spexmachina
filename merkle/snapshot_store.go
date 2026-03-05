package merkle

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Snapshot is the on-disk JSON representation of a merkle tree.
// Nodes are stored in a flat map keyed by path for O(1) lookup and easy diffing.
type Snapshot struct {
	RootHash  string                   `json:"root_hash"`
	CreatedAt time.Time                `json:"created_at"`
	Nodes     map[string]*SnapshotNode `json:"nodes"`
}

// SnapshotNode is a single entry in the flat snapshot map.
type SnapshotNode struct {
	Hash     string   `json:"hash"`
	Type     string   `json:"type"`
	Children []string `json:"children,omitempty"`
}

// Save writes the merkle tree to a snapshot file as JSON.
func Save(tree *Node, path string) error {
	snap := &Snapshot{
		RootHash:  tree.Hash,
		CreatedAt: time.Now().UTC(),
		Nodes:     make(map[string]*SnapshotNode),
	}
	flattenTree(snap.Nodes, tree, "")

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("merkle: save snapshot: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("merkle: save snapshot %s: %w", path, err)
	}
	return nil
}

// Load reads a snapshot file and reconstructs the merkle tree.
func Load(path string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
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

// flattenTree walks the tree recursively and populates the flat node map.
// The prefix tracks the path from the root (empty for root-level nodes).
func flattenTree(nodes map[string]*SnapshotNode, n *Node, prefix string) {
	key := nodePath(prefix, n.Name)

	sn := &SnapshotNode{
		Hash: n.Hash,
		Type: n.Type,
	}
	for _, child := range n.Children {
		sn.Children = append(sn.Children, nodePath(key, child.Name))
	}
	nodes[key] = sn

	for _, child := range n.Children {
		flattenTree(nodes, child, key)
	}
}

// nodePath builds a slash-separated path. If prefix is empty, returns name as-is.
func nodePath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// rebuildTree reconstructs the Node tree from a flat snapshot.
func rebuildTree(snap *Snapshot) (*Node, error) {
	// Find root: the node whose hash matches root_hash
	var rootKey string
	for key, sn := range snap.Nodes {
		if sn.Hash == snap.RootHash {
			rootKey = key
			break
		}
	}
	if rootKey == "" {
		return nil, fmt.Errorf("no node matching root_hash %s", snap.RootHash)
	}

	return rebuildNode(snap.Nodes, rootKey)
}

func rebuildNode(nodes map[string]*SnapshotNode, key string) (*Node, error) {
	sn, ok := nodes[key]
	if !ok {
		return nil, fmt.Errorf("missing node %q", key)
	}

	n := &Node{
		Name: nodeName(key),
		Hash: sn.Hash,
		Type: sn.Type,
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

// nodeName extracts the last path component (the node's own name).
func nodeName(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}
