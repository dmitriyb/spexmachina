# Validation Pipeline

## Data Flow

```
spec directory
     │
     ▼
┌─────────────┐
│ SchemaChecker│── validate JSON against schemas
└──────┬──────┘
       │ errors[]
       ▼
┌──────────────┐
│ContentResolver│── check content paths exist
└──────┬───────┘
       │ errors[]
       ▼
┌────────────┐
│ IDValidator │── check ID uniqueness + cross-refs
└──────┬─────┘
       │ errors[]
       ▼
┌────────────┐
│ DAGChecker  │── detect cycles in dependency graphs
└──────┬─────┘
       │ errors[]
       ▼
┌───────────────┐
│OrphanDetector │── find unreferenced nodes
└──────┬────────┘
       │ warnings[]
       ▼
┌────────────────────────┐
│NameConsistencyChecker  │── project.json ↔ module.json names
└──────┬─────────────────┘
       │ errors[]
       ▼
┌──────────────┐
│ErrorReporter │── aggregate, format, output
└──────┬───────┘
       │
       ▼
  JSON report (stdout)
  exit 0 or 1
```

## Execution Order

Checkers run in sequence, not in parallel. Schema validation must pass before structural checks can run (you can't check DAGs if the JSON doesn't parse). However, all checkers run even if earlier ones find errors — the full error report is always produced.

## Error Accumulation

Each checker appends to a shared `[]ValidationError` slice. No checker short-circuits on errors from previous checkers. The final report contains all violations found across all checks.

## Data Shapes

### Checker input (shared SpecGraph)

- SpecGraph (same shape as consumed by render — see render/flow_render_pipeline.md).
  Validators read the graph in place; they do not mutate nodes.

### Checker output (appended to shared slice)

- ValidationError:
  - severity: string enum — `error` | `warning`
  - code: string — stable machine-readable identifier
    (e.g., `schema.invalid_id`, `dag.cycle_detected`, `orphan.impl_section`,
    `content.missing_file`, `name.mismatch_project_module`)
  - message: string — human-readable one-line summary
  - path: string — JSON pointer or file path, whichever is more specific
  - node_id: string — 12-char hex identity hash of the offending node (empty
    if the error is file-level and predates parse)
  - details: map[string]string — optional structured context (e.g.,
    `{"cycle": "A→B→C→A"}`)

### ErrorReporter → stdout

- ValidationReport:
  - valid: boolean — false if any severity=error entry exists
  - errors: list of ValidationError (severity=error)
  - warnings: list of ValidationError (severity=warning)
  - checked_files: list of string — relative paths of files read
  - schema_version: string

Exit code: `0` if `valid == true`, `1` otherwise. Shape changes here cascade
into downstream consumers (CI pipelines, `spex diff` — which builds the
merkle tree on demand and assumes a validated spec — and skills that parse
the report).
