# SnapshotStore

Reads and writes ID-keyed merkle tree snapshots as JSON files committed to git.

## Responsibilities

- Serialize an ID-keyed merkle tree to a JSON snapshot file
- Deserialize a stored snapshot back into a tree
- Manage snapshot file location within the spec directory

## Interface

```go
// Save writes the merkle tree to a snapshot file.
func Save(tree *Node, path string) error

// Load reads a snapshot file and returns the stored tree.
func Load(path string) (*Node, error)
```

## Snapshot Location

Snapshots are stored at `spec/.snapshot.json`. This file is committed to git alongside the spec. Only one snapshot exists at a time — it represents the last known state.

## File Format

The snapshot is a flat JSON map keyed by spec ID. Each entry contains the node's hash, type, and metadata. The format is `id → {hash, type, node_type, module}` rather than a nested tree, making diff comparison a straightforward map key intersection.

## Design Rationale

### Single snapshot file

One snapshot is sufficient. Git history provides access to any previous snapshot via `git show <commit>:spec/.snapshot.json`. No need for a snapshot archive within the working tree.

### JSON format

JSON is human-readable and diff-friendly in git. When a spec changes, the snapshot diff shows exactly which hashes changed, making it easy to review in PRs.

### ID-keyed, not path-keyed

Keying by spec ID instead of file path makes the snapshot rename-stable. Renaming a module directory or content file does not invalidate the snapshot — the IDs remain the same.
