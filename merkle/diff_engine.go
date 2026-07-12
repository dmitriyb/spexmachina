package merkle

import "sort"

// ChangeType classifies a leaf-level difference between two merkle trees.
type ChangeType int

const (
	Added ChangeType = iota + 1
	Removed
	Modified
)

func (ct ChangeType) String() string {
	switch ct {
	case Added:
		return "added"
	case Removed:
		return "removed"
	case Modified:
		return "modified"
	default:
		return "unknown"
	}
}

// Change represents a single leaf-level difference between two merkle trees.
type Change struct {
	Key      string     // identity hash key
	Type     ChangeType // Added, Removed, or Modified
	NodeType string     // "component", "impl_section", "data_flow", "test_section", "meta", "requirement"
	Module   string     // identity hash of parent module ("" for project-level nodes)
	OldHash  string     // empty for Added
	NewHash  string     // empty for Removed
}

// leafInfo holds leaf-level metadata extracted during tree flattening.
type leafInfo struct {
	Hash     string
	NodeType string
	Module   string
}

// Diff compares two merkle trees (current vs snapshot) and returns leaf-level
// changes sorted by key. If snapshot is nil (first run), all current leaves
// are reported as "added".
func Diff(current, snapshot *Node) []Change {
	currentLeaves := make(map[string]leafInfo)
	flattenLeaves(currentLeaves, current)

	snapshotLeaves := make(map[string]leafInfo)
	if snapshot != nil {
		flattenLeaves(snapshotLeaves, snapshot)
	}

	var changes []Change

	// Added and modified: keys in current
	for key, cur := range currentLeaves {
		old, exists := snapshotLeaves[key]
		if !exists {
			changes = append(changes, Change{
				Key:      key,
				Type:     Added,
				NodeType: cur.NodeType,
				Module:   cur.Module,
				NewHash:  cur.Hash,
			})
		} else if cur.Hash != old.Hash {
			changes = append(changes, Change{
				Key:      key,
				Type:     Modified,
				NodeType: cur.NodeType,
				Module:   cur.Module,
				OldHash:  old.Hash,
				NewHash:  cur.Hash,
			})
		}
	}

	// Removed: keys in snapshot but not in current
	for key, old := range snapshotLeaves {
		if _, exists := currentLeaves[key]; !exists {
			changes = append(changes, Change{
				Key:      key,
				Type:     Removed,
				NodeType: old.NodeType,
				Module:   old.Module,
				OldHash:  old.Hash,
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Key < changes[j].Key
	})

	return changes
}

// flattenLeaves walks the tree and collects only leaf nodes into a key → metadata map.
// Each leaf's Key is used directly (no path building needed with spec-ID keys).
func flattenLeaves(leaves map[string]leafInfo, n *Node) {
	if n.Type == "leaf" {
		leaves[n.Key] = leafInfo{
			Hash:     n.Hash,
			NodeType: n.NodeType,
			Module:   n.Module,
		}
		return
	}

	for _, child := range n.Children {
		flattenLeaves(leaves, child)
	}
}
