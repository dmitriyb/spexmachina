# ImpactCommand

CLI entry point for `spex impact`. Reads a merkle diff and maps changed spec nodes to affected beads.

## Responsibilities

- Parse CLI flags: diff input (stdin or file), bead CLI binary name
- Wire BeadReader to load existing bead metadata
- Wire NodeMatcher to correlate changed nodes with beads
- Wire ActionClassifier to determine actions (create/close/review)
- Wire ReportGenerator to output the impact report as JSON

## Interface

```
spex impact [--diff file] [--bead-cli br] [--json]
```
