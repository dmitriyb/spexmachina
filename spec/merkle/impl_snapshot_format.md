# Snapshot Format

## File Structure

The snapshot is a JSON file at `spec/.snapshot.json`, keyed by spec ID:

```json
{
  "root_hash": "abc123...",
  "created_at": "2026-02-24T12:00:00Z",
  "nodes": {
    "project/meta": {
      "hash": "abc...",
      "type": "leaf",
      "node_type": "meta"
    },
    "module/1": {
      "hash": "def...",
      "type": "module",
      "node_type": "module",
      "module": 1
    },
    "module/1/meta": {
      "hash": "ghi...",
      "type": "leaf",
      "node_type": "meta",
      "module": 1
    },
    "module/1/component/1": {
      "hash": "jkl...",
      "type": "leaf",
      "node_type": "component",
      "module": 1
    },
    "module/1/impl_section/1": {
      "hash": "mno...",
      "type": "leaf",
      "node_type": "impl_section",
      "module": 1
    }
  }
}
```

## Design Decisions

### Flat node map keyed by spec ID

Nodes are stored in a flat map keyed by spec ID rather than file path. This makes lookup O(1) and diff comparison straightforward — iterate keys in both maps. Because keys are spec IDs, renaming a module directory or content file does not change the snapshot structure.

### Timestamps

`created_at` records when the snapshot was taken. This is metadata for human consumption — it is not used in diff computation. The hash values are the sole basis for change detection.

### No content storage

The snapshot stores hashes, not file contents. Content is always read from the working tree. This keeps snapshots small and diff-friendly in git.

### Node metadata

Each node entry carries `node_type` and `module` fields. These are used by the ImpactClassifier to classify changes without path parsing. The metadata is redundant with the key format but avoids key parsing at classification time.
