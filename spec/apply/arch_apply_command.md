# ApplyCommand

CLI entry point for `spex apply`. Reads an impact report and executes bead actions with deterministic ordering.

## Responsibilities

- Parse CLI flags: impact report (stdin or file), bead CLI binary, proposal reference
- Wire BeadCloser for obsolete actions (run first)
- Wire BeadCreator for create actions (run second, in hierarchy order)
- Wire ProposalTagger to tag all affected beads
- Wire SnapshotSaver to save new merkle snapshot

## Interface

```
spex apply [--report file] [--bead-cli br] [--proposal ref] [--dry-run]
```

## Execution Order

1. **Obsoletes first** — close all beads being replaced or removed
2. **Creates in hierarchy order with topological sort** — epics (modules) first, then features (components), then tasks (test_sections). Within each type level, beads are topologically sorted by their `DepBeadIDs` so that dependency beads are created before their dependents. Parent bead IDs are resolved from the mapping file after each level.
3. **Tag all** — tag every affected bead (created + obsoleted) with the proposal reference
4. **Save snapshot** — record the new baseline state

This ordering ensures: (1) parent beads exist before children are created with `--parent`, (2) old beads are closed before new ones reference them with `--deps blocks`, and (3) dependency beads within the same type level are created before dependents so their IDs are available for `--deps depends`.

## Topological Sort Within Type Levels

When multiple beads of the same type are created in a single apply run and some depend on others (via `DepBeadIDs`), the creation order within that type level is determined by topological sort:

1. Build a dependency graph from `DepBeadIDs` — only edges between actions in the current create batch are relevant
2. Topological sort the graph (Kahn's algorithm or DFS-based)
3. If no dependency edges exist between batch members, the original order is preserved (stable sort)
4. Circular dependencies are detected and reported as an error — the apply aborts
