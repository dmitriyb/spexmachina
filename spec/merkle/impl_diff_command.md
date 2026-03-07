# Diff command implementation

## Structure

`cmd/spex/diff.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, resolve spec directory and snapshot path
2. Compute current tree via Hasher + TreeBuilder
3. Load stored snapshot via SnapshotStore
4. Call `DiffEngine.Diff(current, stored)` to get changed nodes
5. Call `ImpactClassifier.Classify(changes)` to determine impact level
6. Output diff report (JSON or human-readable)
