# ValidateCommand

CLI entry point for `spex validate`. Orchestrates all validation checks on a spec directory.

## Responsibilities

- Parse CLI flags: spec directory path (positional or `--dir`)
- Discover and load `project.json` and all `module.json` files
- Run checks in order: SchemaChecker, ContentResolver, IDValidator, DAGChecker, OrphanDetector
- Aggregate errors through ErrorReporter
- Exit 0 if valid, exit 1 with structured JSON errors

## Interface

```
spex validate [dir]
```

- `dir`: path to spec directory (default: `spec/`)
- Output is always structured JSON; pretty-printed when stdout is a TTY, compact otherwise
