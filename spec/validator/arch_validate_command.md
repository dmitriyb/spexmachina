# ValidateCommand

CLI entry point for `spex validate`. Orchestrates all validation checks on a spec directory.

## Responsibilities

- Parse CLI flags: spec directory path (positional or `--dir`), `--json` for structured output
- Discover and load `project.json` and all `module.json` files
- Run checks in order: SchemaChecker, ContentResolver, IDValidator, DAGChecker, OrphanDetector
- Aggregate errors through ErrorReporter
- Exit 0 if valid, exit 1 with structured JSON errors

## Interface

```
spex validate [dir] [--json]
```

- `dir`: path to spec directory (default: `spec/`)
- `--json`: output errors as JSON array instead of human-readable text
