# SnapshotStore

[[e99a846810df|Writes the complete hash tree out to the project's snapshot file, committed to git]], and reads that file back into a tree. The snapshot is JSON and is keyed by identity hash.

## Responsibilities

- Serialize an ID-keyed merkle tree to the snapshot file's canonical bytes
- Deserialize a stored snapshot back into a tree
- Read and write the snapshot at the resolved location handed to it — the component computes no location of its own

## Interface

Three operations: one over bytes, two over a file.

**Encode** takes a finished tree and the timestamp to record in it, and returns the snapshot's bytes — a single JSON document, two-space indented, with a trailing newline, so a snapshot change reviews as a readable git diff instead of one rewritten line. The timestamp is a parameter rather than a reading of the clock, so the same tree and the same timestamp always produce the same bytes. This is the only place a tree is flattened into the file's shape. Anything that persists a snapshot goes through it, whatever it then does with the bytes: a second implementation of that walk is how two writers of one format drift apart while both keep passing their own tests.

**Save** is Encode plus the plainest write there is — it creates the destination's parent directory if that is missing and replaces the file. It offers no atomicity: a reader arriving mid-write can see a truncated file. That is why the one process that persists a snapshot in production does not use it; see "Read vs. write call sites".

**Load** takes a path and returns the tree the file holds. Every failure is an error, absence included — there is no fallback of any kind: an absent file (a typed error, distinguishable from a parse failure, whose meaning is stated under "Read vs. write call sites" below), an unreadable file, malformed JSON, a snapshot that records no root key, a root key with no entry in the node map, and a child key that a parent lists but no entry defines. That last one matters — a snapshot missing an entry its parent still names fails loudly instead of quietly loading as a smaller tree.

## Snapshot Location

The snapshot lives at the location the lifecycle pre-flight ([[a9aa93774cc2|ProjectResolver]]) answers: inside the `.spex/` state directory, the only layout there is. It is committed to git. Only one snapshot exists at a time — it represents the last known state.

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

One snapshot is sufficient. Git history provides access to any previous snapshot via `git show <commit>:<snapshot path>`. No need for a snapshot archive within the working tree.

### JSON format

JSON is human-readable and diff-friendly in git. When a spec changes, the snapshot diff shows exactly which hashes changed, making it easy to review in PRs.

### ID-keyed, not path-keyed

Keying by spec ID instead of file path makes the snapshot rename-stable. Renaming a module directory or content file does not invalidate the snapshot — the IDs remain the same.

## Read vs. write call sites

Reads and writes reach this component from different commands:

- `Load` is called from `spex diff` to read the previous snapshot for
  comparison, and from `spex ingest --mode refresh`, which loads it to
  diff against a freshly built tree and decide whether the drift is
  content-only. When the file does not exist, `Load` returns a typed
  absence error — never a baseline. The empty tree (root with no
  children, root hash = SHA-256 of the empty string) that this branch
  once returned is now produced in exactly one place: as the seed
  snapshot `spex init` writes when a project is born. Callers reach
  `Load` with a location the lifecycle pre-flight resolved, so an absent
  file here means the project is uninitialised or broken — which the
  pre-flight, not this component, reports to the user. Bootstrap is
  diffing against the snapshot init seeded, not a fallback anywhere in
  the read path.
- `Encode` is called from `spex ingest`, by both of its writers: the
  SnapshotSaver path on a complete-status normal run, and the refresh
  pathway, which shares that path's temp-file-and-rename helper rather
  than repeating it. The invariant that snapshot and the task journal move
  together is enforced by ingest, which atomically commits both files
  (mode: normal on complete receipts; mode: refresh always), and `Save`'s
  replace-in-place write cannot hold that invariant — which is why the
  composition sits there rather than here.
  There is no other writer — no standalone `spex hash` command, no other
  subcommand that persists the tree.
- `Save` therefore has no production caller. It stays for callers that
  want a snapshot on disk without ingest's invariant, which today means
  the test suites of this module, of `spex diff` and of
  `spex plan` seeding a baseline to diff against, plus
  `ingest/snapshot_format_test.go`, which pins that Save and the ingest
  writer emit the same bytes.

Keeping reads in `spex diff` and writes in `spex ingest` is what holds the
snapshot+journal atomicity invariant in place.
