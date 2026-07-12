# Hash Computation Flow

This data flow describes how `Hasher`, `TreeBuilder`, and `SnapshotStore`
compose to produce and persist the merkle tree. The composition is invoked
*inside* `spex diff` (and inside `spex ingest`'s SnapshotSaver path) — there is
no standalone `spex hash` command. This is intentional: a separate hash step
either produces a snapshot that matches the current spec (so the next `spex
diff` reports no changes and the pipeline stalls) or it desynchronises the
snapshot from `.bead-map.json` (breaking the snapshot+bead-map atomicity
invariant). Building the tree on demand inside `diff` and only persisting it
through `ingest` keeps both invariants intact.

## Data Flow

```
spec directory
     │
     ▼
┌────────────┐
│ Read        │── project.json → module paths
│ project.json│   module.json  → content file paths
└──────┬─────┘
       │ file paths
       ▼
┌────────────┐
│ Hasher      │── SHA-256 each file
│ (leaves)    │
└──────┬─────┘
       │ leaf hashes
       ▼
┌────────────┐
│ TreeBuilder │── group by type, compute interior hashes
│ (interior)  │   bottom-up: leaves → groups → modules → root
└──────┬─────┘
       │ complete tree
       ▼
┌──────────────┐
│SnapshotStore │── load existing snapshot for diff (read);
│              │   write spec/.snapshot.json from ingest (write).
└──────────────┘
```

## Bootstrap (no existing snapshot)

On a fresh project there is no `spec/.snapshot.json`. The pipeline is:

```
spec change → spex validate → spex diff → spex impact → spex emit
                                  │
                                  ▼
                       (treats missing snapshot as the
                        empty tree; reports every leaf as
                        "added")
                                  │
                                  ▼
                       adapter → spex ingest
                                  │
                                  ▼
                       (Reconciler writes .bead-map.json,
                        SnapshotSaver writes the FIRST
                        spec/.snapshot.json atomically with
                        the bead-map records)
```

The first `spex diff` invocation builds the current tree (Hasher → TreeBuilder)
and compares against the empty-tree baseline that `SnapshotStore.Load` returns
when the snapshot file is absent. The "everything added" diff feeds the
standard impact → emit → adapter → ingest cycle. Ingest's `SnapshotSaver` is
the only writer of `spec/.snapshot.json`; the first save creates the file
alongside the first batch of bead-map records, so snapshot and bead-map are
born consistent.

No standalone "compute and persist the tree" step is required or supported.
Running such a step before the first ingest would write a snapshot that
matches the current spec, which would make the next `spex diff` produce zero
changes and the pipeline would never create the first beads.

## Steady-state (snapshot exists)

Each subsequent change cycle uses the same composition:

```
spec edit → spex diff → spex impact → spex emit → adapter → spex ingest
                │                                              │
                ▼                                              ▼
         (TreeBuilder rebuilds from        (SnapshotSaver overwrites
          current files; SnapshotStore      spec/.snapshot.json iff
          loads previous snapshot;          receipts top-level status
          DiffEngine compares)              is "complete" — see
                                             ingest module)
```

`Hasher`, `TreeBuilder`, and `SnapshotStore` are still the three components
that turn spec files into a tree and persist it; they are just composed under
the diff and ingest call sites rather than under a separate `hash` command.

## Input

The spec directory must be valid (pass `spex validate`). Tree building reads:
- `spec/project.json`
- `spec/<module>/module.json` for each module
- All content files referenced by `content` fields

## Output

A merkle tree data structure with hashes at every level. The tree is held in
memory by the caller (`spex diff` for comparisons; `spex ingest` via
`SnapshotSaver` for the persisted snapshot).

## Data Shapes

Shapes flowing between the three participating components. A change to any field
here is a contract change: all three components must be updated in lockstep.

### Hasher → TreeBuilder

- file_hash: string, 64-character lowercase hex SHA-256 digest of file bytes
- identity_hash: string, 12-character lowercase hex (spec node ID the leaf represents)

### TreeBuilder → SnapshotStore

- Node:
  - key: string — identity_hash for spec-node leaves; `meta/<module-identity-hash>`
    for module.json envelopes; `meta/project` for project.json envelope
  - hash: string, 64-character lowercase hex SHA-256 digest
  - type: string enum — `leaf` | `module` | `project`
  - node_type: string enum — `component` | `requirement` | `impl_section` |
    `data_flow` | `test_section` | `meta` (for envelope leaves)
  - module: string — identity_hash of the parent module; empty string for
    project-level nodes
  - children: list of Node — empty for leaves; sorted by `key` ascending for
    interior nodes (sort order is part of the hash contract)

### Tree root

The root Node has key `project`, type `project`, and children = [project
envelope leaf, project requirement leaves, each module's subtree]. The root
hash is the SHA-256 of the sorted concatenation of children hashes.

### SnapshotStore.Load missing-file contract

When `spec/.snapshot.json` does not exist, `SnapshotStore.Load` returns the
empty tree (root node with no children, root hash = SHA-256 of the empty
string). Callers diff against this baseline to produce the "everything added"
report on first-run bootstrap. This contract is what enables bootstrap without
a pre-seeded snapshot.
