# Snapshot Tests

Integration and acceptance tests for the SnapshotStore (component 3). Validates that merkle trees can be serialized to JSON snapshot files, deserialized back, and that the round-trip preserves all tree information.

## Setup

Scenarios use `t.TempDir()` to create an isolated working directory and build their tree with `setupSpecDir(t)` followed by `BuildTree`, so the fixture is exactly the flat identity-hash shape TreeBuilder produces (a module interior node whose children are the `meta/` envelope leaf plus one leaf per spec node, each keyed by its 12-char hex identity `id`). Sketched, that shape is:

```go
alphaHash := "085966b6bfa1" // identity hash of module alpha
root := &Node{
    Key: "project", Hash: "aaa...", Type: "project",
    Children: []*Node{
        {Key: "meta/project", Hash: "bbb...", Type: "leaf", NodeType: "meta"},
        {Key: alphaHash, Hash: "ccc...", Type: "module", Module: alphaHash, Children: []*Node{
            {Key: "meta/" + alphaHash, Hash: "ddd...", Type: "leaf", NodeType: "meta", Module: alphaHash},
            {Key: "1f00badc0de1", Hash: "eee...", Type: "leaf", NodeType: "component", Module: alphaHash},
            {Key: "2f00badc0de2", Hash: "fff...", Type: "leaf", NodeType: "test_section", Module: alphaHash},
        }},
    },
}
```

The snapshot file path is `<tmpdir>/.snapshot.json`; the production default is `<specDir>/.snapshot.json`. `Save` takes an explicit timestamp — `Save(tree *Node, path string, createdAt time.Time)` — so the byte-equality scenario passes a fixed `createdAt`; the rest pass `time.Now().UTC()` because they assert on decoded structure rather than bytes.

## Scenarios

### S1: Save writes valid JSON to the specified path

**Given** a merkle tree built from the fixture
**When** `Save(tree, snapshotPath, createdAt)` is called
**Then** the file at `snapshotPath` exists and is valid JSON
**And** the JSON contains a `root_hash` field matching the root node's hash
**And** the JSON contains a `created_at` field carrying the supplied `createdAt` as an RFC 3339 timestamp
**And** the JSON contains a `nodes` map

**Rationale**: Validates the basic contract of `Save` — it must produce a well-formed JSON file conforming to the format defined in `arch_snapshot_store.md`.

### S2: Save uses flat node map keyed by identity hash

**Given** the fixture tree with nodes keyed `project`, `meta/project`, `<alpha module hash>`, `meta/<alpha module hash>`, and one identity-hash key per spec-node leaf
**When** `Save(tree, snapshotPath, createdAt)` is called and the output JSON is parsed
**Then** the `nodes` map contains exactly those keys — 12-char hex identity hashes for spec-node leaves, `meta/`-prefixed keys for envelope leaves, `project` for the root
**And** each node entry includes `hash` and `type` fields
**And** interior nodes include a `children` array listing their child keys

**Rationale**: Per `arch_snapshot_store.md`, the snapshot uses a flat map (not a nested tree) for O(1) lookup during diff, keyed by the same identity-hash keys the tree carries — never by file paths.

### S3: Load round-trips the full tree

**Given** a tree saved via `Save(tree, snapshotPath, createdAt)`
**When** `Load(snapshotPath)` is called
**Then** the loaded tree's root hash equals the original tree's root hash
**And** every node in the loaded tree has the same Key, Hash, Type, and Children as the original
**And** the tree structure (parent-child relationships) is fully reconstructed

**Rationale**: The core acceptance criterion for SnapshotStore — Save followed by Load must produce an equivalent tree. This is critical because DiffEngine compares a loaded snapshot against a freshly built tree.

### S4: Save then Load preserves all leaf hashes

**Given** a tree with 5 leaf nodes, each with distinct hashes
**When** the tree is saved (with a fixed `createdAt`) and loaded back
**Then** all 5 leaf hashes in the loaded tree match the originals exactly (character-for-character hex comparison)

**Rationale**: Hash fidelity is non-negotiable. Even a single corrupted character in a saved hash would cause DiffEngine to report a false change.

### S5: Load handles a snapshot produced from a real spec tree

**Given** a full spec directory fixture (project.json, module.json, content files)
**When** `BuildTree` is called to compute the current tree
**And** `Save` writes the snapshot with a fixed `createdAt`
**And** `Load` reads the snapshot back
**Then** the loaded tree is structurally and hash-identical to the computed tree

**Rationale**: End-to-end integration between TreeBuilder and SnapshotStore. This is the real-world usage path: `spex ingest` builds a tree and saves the snapshot on a complete run, then `spex diff` loads it later as the baseline. (The former standalone `spex hash` writer was removed by the pipeline-cleanup proposal.)

### S6: Multiple saves overwrite the same snapshot file

**Given** a tree is saved to `snapshotPath`
**And** a second, different tree is saved to the same `snapshotPath`
**When** `Load(snapshotPath)` is called
**Then** the loaded tree matches the second (most recent) save
**And** the loaded root hash is the second tree's, not the first's

**Rationale**: Per `arch_snapshot_store.md`, only one snapshot exists at a time. Saves must fully replace the previous snapshot, not append or merge.

### S7: Snapshot JSON is human-readable and git-diff friendly

**Given** a tree saved to `snapshotPath`
**When** the snapshot file is read as raw text
**Then** the JSON is pretty-printed (indented), not minified
**And** node keys appear as readable strings (not encoded)

**Rationale**: Per the design rationale in `arch_snapshot_store.md` and `arch_hasher.md`, snapshot files are committed to git and must produce meaningful diffs in pull requests.

## Edge Cases

### E1: Load on non-existent snapshot file returns the empty-tree baseline

**Given** a path to a snapshot file that does not exist
**When** `Load(path)` is called
**Then** it returns the empty-tree baseline — a root with key `project`, no children, and hash = SHA-256 of the empty input — with a nil error

**Rationale**: The missing-file case is handled inside `Load` itself so `spex diff` can bootstrap on a fresh project without a pre-seeded snapshot: diffing the current tree against the empty baseline reports every leaf as "added". See `flow_hash_computation.md` for the full bootstrap flow.

### E1b: Non-ENOENT read errors still fail

**Given** a snapshot path whose read fails for a reason other than file-not-found (e.g. permission denied)
**When** `Load(path)` is called
**Then** it returns a nil tree and an error wrapping the underlying I/O failure

**Rationale**: Only `errors.Is(err, fs.ErrNotExist)` triggers the EmptyTree baseline. Any other read failure is a real fault that must surface, not silently degrade into a "everything added" diff. Pinned by `TestREQ3_Load_PermissionErrorStillFails`.

### E2: Load on malformed JSON

**Given** a file at `snapshotPath` containing `"this is not json{{{"`
**When** `Load(snapshotPath)` is called
**Then** it returns an error wrapping the JSON parse failure
**And** the error includes the file path for debuggability

### E3: Save with empty tree (root only, no children)

**Given** a tree with only a root node: `{Key: "project", Hash: "xyz", Type: "project", Children: nil}`
**When** `Save(tree, snapshotPath, createdAt)` is called
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
**When** `Save(tree, snapshotPath, createdAt)` is called
**Then** the directory is created and the snapshot file is written successfully

**Rationale**: On first run in a new project, the spec directory may exist but the snapshot subdirectory path may not. Save should not fail due to missing intermediate directories.
