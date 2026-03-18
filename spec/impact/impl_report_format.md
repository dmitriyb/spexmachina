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
- `creates` — new beads to be created (for added and modified nodes)
- `obsoletes` — existing beads to be obsoleted (for modified and removed nodes)

## Serialization

Use `json.NewEncoder(w).Encode(&report)` with 2-space indentation for human readability. The report is written to stdout for piping.

## Empty Report

When no changes are detected, the report has empty arrays and zero counts. This is a valid report — `spex apply` handles it as a no-op.
