# DiffCommand

CLI entry point for `spex diff`. Compares the current merkle tree against a stored snapshot and classifies impact.

## Responsibilities

- Parse CLI flags: spec directory, snapshot path (optional, defaults to stored)
- Wire SnapshotStore to load the previous snapshot
- Wire DiffEngine to compare current vs stored trees
- Wire ImpactClassifier to classify changes
- Output the diff report

## Interface

```
spex diff [dir] [--snapshot path] [--json]
```
