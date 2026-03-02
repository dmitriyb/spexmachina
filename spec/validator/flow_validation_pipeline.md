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
