# ApplyCommand

CLI entry point for `spex apply`. Reads an impact report and executes bead actions with deterministic ordering.

## Responsibilities

- Parse CLI flags: impact report (stdin or file), bead CLI binary, proposal reference
- Wire BeadCloser to label obsolete beads (mark intent, keep open)
- Wire BeadCreator for create actions (in hierarchy order, while old beads still open)
- Wire BeadCloser to close obsolete beads (after replacements exist)
- Wire ProposalTagger to tag all affected beads
- Wire SnapshotSaver to save new merkle snapshot

## Interface

```
spex apply [--report file] [--bead-cli br] [--proposal ref] [--dry-run]
```

## Execution Order

1. **Label obsoletes** — mark beads being replaced or removed with `spex:obsolete` + `commit:<HEAD>` labels, but keep them open. For removed nodes, delete the mapping record (looked up by the action's `SpecNodeID` identity hash). For modified nodes, leave the record for BeadCreator to update in place.
2. **Creates in hierarchy order with topological sort** — epics (modules) first, then features (components), then tasks (test_sections). Within each type level, beads are topologically sorted by their `DepBeadIDs` so that dependency beads are created before their dependents. Parent bead IDs are resolved from the mapping file after each level. Old beads are still open at this point, so `--deps blocks:<old-bead-id>` references valid open beads.
3. **Close obsoletes** — close all beads that were labeled in step 1. This is safe because replacements already exist.
4. **Tag all** — tag every affected bead (created + closed) with the proposal reference
5. **Save snapshot** — record the new baseline state

This label→create→close ordering ensures: (1) old beads are still open when new beads reference them via `--deps blocks`, (2) parent beads exist before children are created with `--parent`, (3) dependency beads within the same type level are created before dependents, and (4) `br` auto-flush correctly persists all bead states to the JSONL file that `bv` reads.

## No spec_node_id derivation

Each `Action` carries the affected node's identity hash in `SpecNodeID`. ApplyCommand passes this value straight through to BeadCreator (which writes it to the new mapping record) and BeadCloser (which uses it to find and delete the existing record). There is no helper that reconstructs `spec_node_id` from module name + integer ID; the earlier `deriveSpecNodeID` function is deleted. The merkle diff, the impact report, and the mapping store all use the same identity hash, so the value flows through unchanged from end to end.

## Topological Sort Within Type Levels

When multiple beads of the same type are created in a single apply run and some depend on others (via `DepBeadIDs`), the creation order within that type level is determined by topological sort:

1. Build a dependency graph from `DepBeadIDs` — only edges between actions in the current create batch are relevant
2. Topological sort the graph (Kahn's algorithm or DFS-based)
3. If no dependency edges exist between batch members, the original order is preserved (stable sort)
4. Circular dependencies are detected and reported as an error — the apply aborts
