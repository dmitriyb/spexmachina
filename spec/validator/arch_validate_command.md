# ValidateCommand

CLI entry point for `spex validate`. Orchestrates all validation checks on a spec directory.

## Responsibilities

- Parse CLI flags: spec directory path (positional or `--dir`)
- Resolve that path to an absolute spec directory and hand it to each checker; every checker loads `project.json` and the `module.json` files it needs for itself
- Run eight checkers in a fixed order: SchemaChecker, ContentResolver, IDValidator, DAGChecker, NameConsistencyChecker, TestCoverageChecker, RequirementCoverageChecker, CoupledSectionChecker
- Run all eight regardless of what earlier ones found — no checker short-circuits the sequence
- Aggregate every entry through ErrorReporter, which sorts by path and writes the report to stdout
- Derive exit status from the report ErrorReporter returns: 0 if valid, 1 otherwise

## Interface

```
spex validate [dir]
```

- `dir`: path to spec directory (default: `spec/`)
- Output is always structured JSON; pretty-printed when stdout is a TTY, compact otherwise
