# Report Format

## JSON Structure

```json
{
  "creates": [
    {
      "type": "create",
      "module": "validator",
      "node": "ContentResolver",
      "impact": "arch_impl",
      "reason": "New spec node: validator/ContentResolver"
    }
  ],
  "closes": [
    {
      "type": "close",
      "bead_id": "spexmachina-abc",
      "module": "validator",
      "node": "LegacyChecker",
      "reason": "Spec node removed: validator/LegacyChecker"
    }
  ],
  "reviews": [
    {
      "type": "review",
      "bead_id": "spexmachina-def",
      "module": "merkle",
      "node": "Hasher",
      "impact": "impl_only",
      "reason": "Spec node modified (impl_only): merkle/Hasher"
    }
  ],
  "summary": {
    "create_count": 1,
    "close_count": 1,
    "review_count": 1
  }
}
```

## Serialization

Use `json.NewEncoder(w).Encode(&report)` with 2-space indentation for human readability. The report is written to stdout for piping.

## Empty Report

When no changes are detected, the report has empty arrays and zero counts. This is a valid report — `spex apply` handles it as a no-op.
