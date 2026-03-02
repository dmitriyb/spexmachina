# Snapshot Format

## File Structure

The snapshot is a JSON file at `spec/.snapshot.json`:

```json
{
  "root_hash": "abc123...",
  "created_at": "2026-02-24T12:00:00Z",
  "nodes": {
    "project.json": {
      "hash": "abc...",
      "type": "leaf"
    },
    "schema": {
      "hash": "def...",
      "type": "module",
      "children": ["schema/module.json", "schema/arch", "schema/impl"]
    },
    "schema/module.json": {
      "hash": "ghi...",
      "type": "leaf"
    },
    "schema/arch": {
      "hash": "jkl...",
      "type": "arch",
      "children": ["schema/arch_project_schema.md", "schema/arch_module_schema.md"]
    }
  }
}
```

## Design Decisions

### Flat node map

Nodes are stored in a flat map keyed by path rather than a nested tree. This makes lookup O(1) and diff comparison straightforward — iterate keys in both maps.

### Timestamps

`created_at` records when the snapshot was taken. This is metadata for human consumption — it is not used in diff computation. The hash values are the sole basis for change detection.

### No content storage

The snapshot stores hashes, not file contents. Content is always read from the working tree. This keeps snapshots small and diff-friendly in git.
