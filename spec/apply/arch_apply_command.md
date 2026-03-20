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
2. **Creates in hierarchy order** — epics (modules) first, then features (components), then tasks (test_sections). Parent bead IDs are resolved from the mapping file after each level.
3. **Tag all** — tag every affected bead (created + obsoleted) with the proposal reference
4. **Save snapshot** — record the new baseline state

This ordering ensures parent beads exist before children are created with `--parent`, and old beads are closed before new ones reference them with `--deps blocks`.
