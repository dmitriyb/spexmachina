# HashCommand

CLI entry point for `spex hash`. Computes the merkle tree over a spec directory and stores a snapshot.

## Responsibilities

- Parse CLI flags: spec directory path, `--json` for structured output
- Wire Hasher to compute leaf hashes
- Wire TreeBuilder to construct the full merkle tree
- Wire SnapshotStore to save the snapshot to disk
- Output the root hash and tree summary

## Interface

```
spex hash [dir] [--json]
```
