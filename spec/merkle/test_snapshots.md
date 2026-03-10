# Snapshot Tests

Integration and acceptance tests for the SnapshotStore (component 3). Validates that merkle trees can be serialized to JSON snapshot files, deserialized back, and that the round-trip preserves all tree information.

## Setup

Scenarios use `t.TempDir()` to create an isolated working directory. A helper `buildFixtureTree(t)` constructs a known merkle tree in-memory:

```go
root := &Node{
    Name: "root", Hash: "aaa...", Type: "project",
    Children: []*Node{
        {Name: "project.json", Hash: "bbb...", Type: "leaf"},
        {Name: "alpha", Hash: "ccc...", Type: "module", Children: []*Node{
            {Name: "alpha/module.json", Hash: "ddd...", Type: "leaf"},
            {Name: "alpha/arch", Hash: "eee...", Type: "arch", Children: []*Node{
                {Name: "alpha/arch_widget.md", Hash: "fff...", Type: "leaf"},
            }},
            {Name: "alpha/impl", Hash: "ggg...", Type: "impl", Children: []*Node{
                {Name: "alpha/impl_widget_logic.md", Hash: "hhh...", Type: "leaf"},
            }},
        }},
    },
}
```

The snapshot file path is `<tmpdir>/spec/.snapshot.json` per the spec convention.

## Scenarios

### S1: Save writes valid JSON to the specified path

**Given** a merkle tree built from the fixture
**When** `Save(tree, snapshotPath)` is called
**Then** the file at `snapshotPath` exists and is valid JSON
**And** the JSON contains a `root_hash` field matching the root node's hash
**And** the JSON contains a `created_at` field with a valid RFC 3339 timestamp
**And** the JSON contains a `nodes` map

**Rationale**: Validates the basic contract of `Save` — it must produce a well-formed JSON file conforming to the format defined in `impl_snapshot_format.md`.

### S2: Save uses flat node map keyed by path

**Given** the fixture tree with nodes at paths: `project.json`, `alpha`, `alpha/module.json`, `alpha/arch`, `alpha/arch_widget.md`, `alpha/impl`, `alpha/impl_widget_logic.md`
**When** `Save(tree, snapshotPath)` is called and the output JSON is parsed
**Then** the `nodes` map contains exactly those 7 paths as keys
**And** each node entry includes `hash` and `type` fields
**And** interior nodes include a `children` array listing their child paths

**Rationale**: Per `impl_snapshot_format.md`, the snapshot uses a flat map (not a nested tree) for O(1) lookup during diff. This test ensures the tree-to-flat-map serialization is correct.

### S3: Load round-trips the full tree

**Given** a tree saved via `Save(tree, snapshotPath)`
**When** `Load(snapshotPath)` is called
**Then** the loaded tree's root hash equals the original tree's root hash
**And** every node in the loaded tree has the same Name, Hash, Type, and Children as the original
**And** the tree structure (parent-child relationships) is fully reconstructed

**Rationale**: The core acceptance criterion for SnapshotStore — Save followed by Load must produce an equivalent tree. This is critical because DiffEngine compares a loaded snapshot against a freshly built tree.

### S4: Save then Load preserves all leaf hashes

**Given** a tree with 5 leaf nodes, each with distinct hashes
**When** the tree is saved and loaded back
**Then** all 5 leaf hashes in the loaded tree match the originals exactly (character-for-character hex comparison)

**Rationale**: Hash fidelity is non-negotiable. Even a single corrupted character in a saved hash would cause DiffEngine to report a false change.

### S5: Load handles snapshot from a previous `spex hash` run

**Given** a full spec directory fixture (project.json, module.json, content files)
**When** `BuildTree` is called to compute the current tree
**And** `Save` writes the snapshot
**And** `Load` reads the snapshot back
**Then** the loaded tree is structurally and hash-identical to the computed tree

**Rationale**: End-to-end integration between TreeBuilder and SnapshotStore. This is the real-world usage path: `spex hash` builds a tree and saves it, then `spex diff` loads it later.

### S6: Multiple saves overwrite the same snapshot file

**Given** a tree is saved to `snapshotPath`
**And** a second, different tree is saved to the same `snapshotPath`
**When** `Load(snapshotPath)` is called
**Then** the loaded tree matches the second (most recent) save
**And** no trace of the first tree's hashes remains in the file

**Rationale**: Per `arch_snapshot_store.md`, only one snapshot exists at a time. Saves must fully replace the previous snapshot, not append or merge.

### S7: Snapshot JSON is human-readable and git-diff friendly

**Given** a tree saved to `snapshotPath`
**When** the snapshot file is read as raw text
**Then** the JSON is pretty-printed (indented), not minified
**And** node paths appear as readable strings (not encoded)

**Rationale**: Per the design rationale in `arch_snapshot_store.md` and `arch_hasher.md`, snapshot files are committed to git and must produce meaningful diffs in pull requests.

## Edge Cases

### E1: Load on non-existent snapshot file

**Given** a path to a snapshot file that does not exist
**When** `Load(path)` is called
**Then** it returns an error (not nil tree) indicating the file was not found

**Rationale**: DiffEngine needs to distinguish "no snapshot" (first run, treat everything as added) from "corrupted snapshot" (error). The caller (DiffCommand) handles the missing-file case.

### E2: Load on malformed JSON

**Given** a file at `snapshotPath` containing `"this is not json{{{"`
**When** `Load(snapshotPath)` is called
**Then** it returns an error wrapping the JSON parse failure
**And** the error includes the file path for debuggability

### E3: Save with empty tree (root only, no children)

**Given** a tree with only a root node: `{Name: "root", Hash: "xyz", Type: "project", Children: nil}`
**When** `Save(tree, snapshotPath)` is called
**Then** the snapshot file is written successfully
**And** `Load(snapshotPath)` returns a tree with only the root node
**And** the `nodes` map contains exactly one entry

**Rationale**: Degenerate case — an empty spec project with no modules. The snapshot format must handle it gracefully.

### E4: Load snapshot with unknown node type

**Given** a hand-edited snapshot JSON where one node has `"type": "unknown_type"`
**When** `Load(snapshotPath)` is called
**Then** the load succeeds (SnapshotStore does not validate types — that is the Validator's job)
**And** the node retains the type string `"unknown_type"`

**Rationale**: SnapshotStore is a serialization layer, not a validator. Forward compatibility: future spec versions may add new node types.

### E5: Save creates parent directories if needed

**Given** a snapshot path `<tmpdir>/spec/.snapshot.json` where `<tmpdir>/spec/` does not yet exist
**When** `Save(tree, snapshotPath)` is called
**Then** the directory is created and the snapshot file is written successfully

**Rationale**: On first run in a new project, the spec directory may exist but the snapshot subdirectory path may not. Save should not fail due to missing intermediate directories.
