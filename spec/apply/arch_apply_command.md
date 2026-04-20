# ApplyCommand

CLI entry point for `spex apply`. Reads an impact report and executes bead actions with deterministic ordering.

## Responsibilities

- Parse CLI flags: impact report (stdin or file), bead CLI binary, proposal reference
- Wire BeadCloser to label obsolete beads (mark intent, keep open)
- Wire BeadCreator to create the proposal epic first, then feature/task beads in topological order, all parented under the proposal epic
- Wire BeadCloser to close obsolete beads (after replacements exist)
- Wire SnapshotSaver to save new merkle snapshot

## Interface

```
spex apply [--report file] [--bead-cli br] [--proposal ref] [--dry-run]
```

## Execution Order

1. **Label obsoletes** — mark beads being replaced or removed with `spex:obsolete` + `commit:<HEAD>` labels, but keep them open. For removed nodes, delete the mapping record (looked up by the action's `SpecNodeID` identity hash). For modified nodes, leave the record for BeadCreator to update in place.
2. **Create proposal epic** — first create action when the run has any `create` entries. The epic is named after the `--proposal` flag value, typed `epic`, no parent. Its bead ID is captured and reused as `--parent` for every subsequent create. A bead-map record is written with `node_type=proposal`, `bead_type=epic`, `spec_node_id=<proposal-ref>`.
3. **Creates in topological order** — features (components) and task-type creates (data_flow, multi-component test_section) are placed in a single pool and topologically sorted by their `DepBeadIDs` so that dependency beads are created before their dependents. Every bead in this pool is parented under the proposal-epic bead ID from step 2. Old beads are still open at this point, so `--deps blocks:<old-bead-id>` references valid open beads.
4. **Close obsoletes** — close all beads that were labeled in step 1. This is safe because replacements already exist.
5. **Save snapshot** — record the new baseline state

This label→epic→create→close ordering ensures: (1) old beads are still open when new beads reference them via `--deps blocks`, (2) the proposal epic exists before any feature/task is created with `--parent`, (3) dependency beads within the create pool are created before dependents, and (4) `br` auto-flush correctly persists all bead states to the JSONL file that `bv` reads.

Historical note: proposal-label tagging is gone. The ProposalTagger component and its requirement have been removed. Bead grouping by proposal wave is now structural (every created bead is a child of that run's proposal epic) instead of metadata-based.

## No spec_node_id derivation

Each `Action` carries the affected node's identity hash in `SpecNodeID`. ApplyCommand passes this value straight through to BeadCreator (which writes it to the new mapping record) and BeadCloser (which uses it to find and delete the existing record). There is no helper that reconstructs `spec_node_id` from module name + integer ID; the earlier `deriveSpecNodeID` function is deleted. The merkle diff, the impact report, and the mapping store all use the same identity hash, so the value flows through unchanged from end to end.

## Topological Sort Within Type Levels

When multiple beads of the same type are created in a single apply run and some depend on others (via `DepBeadIDs`), the creation order within that type level is determined by topological sort:

1. Build a dependency graph from `DepBeadIDs` — only edges between actions in the current create batch are relevant
2. Topological sort the graph (Kahn's algorithm or DFS-based)
3. If no dependency edges exist between batch members, the original order is preserved (stable sort)
4. Circular dependencies are detected and reported as an error — the apply aborts
