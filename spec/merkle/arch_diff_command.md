# DiffCommand

CLI entry point for `spex diff`. Compares the current merkle tree against a stored snapshot, classifies impact, and runs change completeness validation.

## Responsibilities

- Parse CLI flags: spec directory, snapshot path (optional, defaults to stored)
- Wire SnapshotStore to load the previous snapshot
- Wire DiffEngine to compare current vs stored trees
- Wire ImpactClassifier to classify changes
- Wire CompletenessChecker to validate that requirement changes are accompanied by component content changes
- Output the diff report with changes and errors

## Interface

```
spex diff [dir] [--snapshot path] [--json]
```

## Output

JSON output includes both changes and errors. The `path` and `related` fields carry identity hashes (the same values used as merkle keys and bead-map `spec_node_id`s):

```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "requirement 'Match changed nodes to beads' description changed but component NodeMatcher content leaf unchanged",
      "path": "7c5e2fa1b3d8",
      "related": ["a1b2c3d4e5f6"]
    }
  ],
  "summary": { ... }
}
```

Text output prints changes first, then errors (if any) highlighted as warnings.
