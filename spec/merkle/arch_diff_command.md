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

JSON output includes both changes and errors:

```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "requirement 2 description changed but component NodeMatcher content leaf unchanged",
      "path": "module/4/requirement/2",
      "related": ["module/4/component/2"]
    }
  ],
  "summary": { ... }
}
```

Text output prints changes first, then errors (if any) highlighted as warnings.
