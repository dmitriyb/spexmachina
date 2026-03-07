# ApplyCommand

CLI entry point for `spex apply`. Reads an impact report and executes bead actions.

## Responsibilities

- Parse CLI flags: impact report (stdin or file), bead CLI binary, proposal reference
- Wire BeadCreator for create actions
- Wire BeadCloser for close actions
- Wire BeadUpdater for review/update actions
- Wire ProposalTagger to tag all affected beads
- Wire SnapshotSaver to save new merkle snapshot

## Interface

```
spex apply [--report file] [--bead-cli br] [--proposal ref] [--dry-run]
```
