# Snapshot Format

## File Structure

The snapshot is a JSON file at `spec/.snapshot.json`, keyed by identity hash. The two synthetic envelope leaves use `meta/project` and `meta/<module-identity-hash>` keys:

```json
{
  "root_hash": "abc123...",
  "created_at": "2026-02-24T12:00:00Z",
  "nodes": {
    "meta/project": {
      "hash": "abc...",
      "type": "leaf",
      "node_type": "meta"
    },
    "0011223344aa": {
      "hash": "def...",
      "type": "module",
      "node_type": "module"
    },
    "meta/0011223344aa": {
      "hash": "ghi...",
      "type": "leaf",
      "node_type": "meta",
      "module": "0011223344aa"
    },
    "55667788cc99": {
      "hash": "jkl...",
      "type": "leaf",
      "node_type": "component",
      "module": "0011223344aa"
    },
    "ddeeff001122": {
      "hash": "mno...",
      "type": "leaf",
      "node_type": "impl_section",
      "module": "0011223344aa"
    }
  }
}
```

The hex strings above are placeholders — every key is the actual identity hash of the spec node it represents.

## Design Decisions

### Flat node map keyed by identity hash

Nodes are stored in a flat map keyed by identity hash rather than file path or composite integer key. This makes lookup O(1) and diff comparison straightforward — iterate keys in both maps. Because identity hashes are derived from the node's name and module, not from filesystem layout, renaming a module directory or content file does not change the snapshot structure. Renaming a node (changing its `name` field) does change its identity hash and is recorded as a delete + add — see `impl_diff_algorithm.md` for details.

The snapshot keys are byte-identical to the merkle tree keys *and* to the bead-map `spec_node_id` values. There is one key format across the entire pipeline.

### Timestamps

`created_at` records when the snapshot was taken. This is metadata for human consumption — it is not used in diff computation. The hash values are the sole basis for change detection.

### No content storage

The snapshot stores hashes, not file contents. Content is always read from the working tree. This keeps snapshots small and diff-friendly in git.

### Node metadata

Each node entry carries `node_type` and `module` fields. `node_type` is required because identity hashes do not embed type information — there is no way to look at `55667788cc99` and tell whether it points at a component or an impl_section. `module` carries the parent module's identity hash so the ImpactClassifier can group changes by module without re-walking the tree. Both fields are denormalized into the snapshot specifically so downstream consumers do not have to parse keys.
