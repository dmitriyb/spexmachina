# SnapshotStore

[[e99a846810df|Writes the complete hash tree out to a snapshot file that is committed to git beside the spec]], and reads that file back into a tree. The snapshot is JSON and is keyed by identity hash.

## Responsibilities

- Serialize an ID-keyed merkle tree to a JSON snapshot file
- Deserialize a stored snapshot back into a tree
- Manage snapshot file location within the spec directory

## Interface

Two operations over one file.

**Save** takes a finished tree, the path to write it to, and the timestamp to record in it. It creates the destination's parent directory if that is missing, then writes the whole tree as a single JSON document — two-space indented, with a trailing newline — so a snapshot change reviews as a readable git diff instead of one rewritten line. The timestamp is a parameter rather than a reading of the clock, so the same tree and the same timestamp always produce the same bytes.

**Load** takes a path and returns the tree the file holds. Its behaviour when the file is absent is a contract in its own right and is stated under "Read vs. write call sites" below. Every other failure is an error rather than a fallback: an unreadable file, malformed JSON, a snapshot that records no root key, a root key with no entry in the node map, and a child key that a parent lists but no entry defines. That last one matters — a snapshot missing an entry its parent still names fails loudly instead of quietly loading as a smaller tree.

## Snapshot Location

Snapshots are stored at `spec/.snapshot.json`. This file is committed to git alongside the spec. Only one snapshot exists at a time — it represents the last known state.

## File Format

The snapshot is one JSON document: three header fields and a flat map holding every node of the tree, keyed by the node's key. The map is flat rather than nested, which makes comparing two snapshots a key intersection instead of a walk.

```json
{
  "root_hash": "<64-hex digest of the root node>",
  "root_key": "project",
  "created_at": "2026-07-12T07:53:29.600118502Z",
  "nodes": {
    "project": {
      "hash": "<64-hex>",
      "type": "project",
      "children": ["0011223344aa", "meta/project"]
    },
    "meta/project": {
      "hash": "<64-hex>",
      "type": "leaf",
      "node_type": "meta"
    },
    "0011223344aa": {
      "hash": "<64-hex>",
      "type": "module",
      "module": "0011223344aa",
      "children": ["55667788cc99", "meta/0011223344aa"]
    },
    "meta/0011223344aa": {
      "hash": "<64-hex>",
      "type": "leaf",
      "node_type": "meta",
      "module": "0011223344aa"
    },
    "55667788cc99": {
      "hash": "<64-hex>",
      "type": "leaf",
      "node_type": "component",
      "module": "0011223344aa"
    }
  }
}
```

The 12-character keys above are placeholders — in a real snapshot each is the actual identity hash of the spec node it represents. `root_key` names the entry the tree hangs from. `children` lists child keys and appears on interior entries only. `node_type` and `module` are written only when they carry a value, which is why the root entry shows neither and a module entry shows a module but no node type.

Both are carried on the entry itself rather than derived from it, so a consumer never has to parse a key or re-walk the tree: an identity hash does not say what kind of node it points at, and the owning module has to travel with the entry for a change to be attributed to a module at all.

No file content is stored — only the hashes and the handful of fields above. Content is always re-read from the working tree, which keeps the file small and confines its git diff to the hashes that actually moved.

`created_at` records when the snapshot was written and nothing reads it back. Loading ignores it and change detection compares hashes only, so two snapshots taken hours apart over identical spec content diff to nothing.

## Design Rationale

### Single snapshot file

One snapshot is sufficient. Git history provides access to any previous snapshot via `git show <commit>:spec/.snapshot.json`. No need for a snapshot archive within the working tree.

### JSON format

JSON is human-readable and diff-friendly in git. When a spec changes, the snapshot diff shows exactly which hashes changed, making it easy to review in PRs.

### ID-keyed, not path-keyed

Keying by spec ID instead of file path makes the snapshot rename-stable. Renaming a module directory or content file does not invalidate the snapshot — the IDs remain the same.

## Read vs. write call sites

Reads and writes reach this component from different commands:

- `Load` is called from `spex diff` to read the previous snapshot for
  comparison, and from `spex ingest --mode refresh`, which loads it to
  diff against a freshly built tree and decide whether the drift is
  content-only. When `spec/.snapshot.json` does not exist, `Load` returns
  the empty tree (root with no children, root hash = SHA-256 of the empty
  string) rather than an error. This contract is what enables bootstrap
  without a pre-seeded snapshot — the first diff treats the spec as
  entirely added against the empty baseline.
- `Save` is called only from `spex ingest`'s SnapshotSaver path. The
  invariant that snapshot and `.bead-map.json` move together is enforced
  by ingest, which atomically commits both files (mode: normal on complete
  receipts; mode: refresh always). There is no other writer — no
  standalone `spex hash` command, no other subcommand that persists the
  tree.

Keeping reads in `spex diff` and writes in `spex ingest` is what holds the
snapshot+bead-map atomicity invariant in place.
