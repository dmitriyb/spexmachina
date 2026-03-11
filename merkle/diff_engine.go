package merkle

import "sort"

// ChangeType classifies a leaf-level difference between two merkle trees.
type ChangeType int

const (
	Added    ChangeType = iota + 1
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
	Path    string     // spec ID key, e.g. "module/1/component/2"
	Type    ChangeType // Added, Removed, or Modified
	OldHash string     // empty for Added
	NewHash string     // empty for Removed
}

// Diff compares two merkle trees (current vs snapshot) and returns leaf-level
// changes sorted by path. If snapshot is nil (first run), all current leaves
// are reported as "added".
func Diff(current, snapshot *Node) []Change {
	currentLeaves := make(map[string]string)
	flattenLeaves(currentLeaves, current)

	snapshotLeaves := make(map[string]string)
	if snapshot != nil {
		flattenLeaves(snapshotLeaves, snapshot)
	}

	var changes []Change

	// Added and modified: paths in current
	for path, curHash := range currentLeaves {
		oldHash, exists := snapshotLeaves[path]
		if !exists {
			changes = append(changes, Change{
				Path:    path,
				Type:    Added,
				NewHash: curHash,
			})
		} else if curHash != oldHash {
			changes = append(changes, Change{
				Path:    path,
				Type:    Modified,
				OldHash: oldHash,
				NewHash: curHash,
			})
		}
	}

	// Removed: paths in snapshot but not in current
	for path, oldHash := range snapshotLeaves {
		if _, exists := currentLeaves[path]; !exists {
			changes = append(changes, Change{
				Path:    path,
				Type:    Removed,
				OldHash: oldHash,
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	return changes
}

// flattenLeaves walks the tree and collects only leaf nodes into a key → hash map.
// Each leaf's Key is used directly (no path building needed with spec-ID keys).
func flattenLeaves(leaves map[string]string, n *Node) {
	if n.Type == "leaf" {
		leaves[n.Key] = n.Hash
		return
	}

	for _, child := range n.Children {
		flattenLeaves(leaves, child)
	}
}
