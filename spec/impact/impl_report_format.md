# Report Format

## JSON Structure

```json
{
  "creates": [
    {
      "type": "create",
      "module": "validator",
      "node": "ContentResolver",
      "node_type": "component",
      "spec_hash": "abc123",
      "old_bead_id": "spexmachina-77",
      "dep_bead_ids": ["spex-050", "spex-060"],
      "reason": "Spec node modified (new): validator/ContentResolver"
    }
  ],
  "obsoletes": [
    {
      "type": "obsolete",
      "bead_id": "spexmachina-77",
      "module": "validator",
      "node": "ContentResolver",
      "reason": "Spec node modified: validator/ContentResolver"
    }
  ],
  "summary": {
    "create_count": 1,
    "obsolete_count": 1
  }
}
```

Two action groups replace the previous three (creates/closes/reviews):
- `creates` — new beads to be created (for added and modified nodes). Each create action may include `dep_bead_ids` — an array of bead IDs representing spec-graph dependencies resolved from `uses` and `requires_module` edges. Empty or omitted when no dependencies exist.
- `obsoletes` — existing beads to be obsoleted (for modified and removed nodes)

## Serialization

Use `json.NewEncoder(w).Encode(&report)` with 2-space indentation for human readability. The report is written to stdout for piping.

## Empty Report

When no changes are detected, the report has empty arrays and zero counts. This is a valid report — `spex apply` handles it as a no-op.
