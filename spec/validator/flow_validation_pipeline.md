# Validation Pipeline

## Data Flow

```
spec directory
     │
     ▼
┌────────────────────────────┐
│ SchemaChecker              │── validate JSON against schemas
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ ContentResolver            │── check content paths exist
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ IDValidator                │── ID uniqueness + cross-refs + preq_id + priority
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ DAGChecker                 │── detect cycles in dependency graphs
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ NameConsistencyChecker     │── project.json ↔ module.json names
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ TestCoverageChecker        │── every component described by a test_section
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ RequirementCoverageChecker │── preq_id derivation + implements coverage
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ CoupledSectionChecker      │── sections envelope + coupled module + section schema
└──────┬─────────────────────┘
       │ errors[]
       ▼
┌────────────────────────────┐
│ ErrorReporter              │── aggregate, sort by path, format, output
└──────┬─────────────────────┘
       │
       ▼
  JSON report (stdout)
  exit 0 or 1
```

## Execution Order

Eight checkers run in sequence, not in parallel, in exactly the order drawn above. That order is a property of the command, not of the output: ErrorReporter sorts the aggregated entries by path, so the checker sequence is not observable in the report.

Schema validation leads because the structural checks are meaningless on unparseable JSON. But no checker short-circuits the sequence — all eight run even when earlier ones report errors, so a single run produces the full report rather than one failure at a time.

## Error Accumulation

Each checker appends to a shared `[]ValidationError` slice. No checker short-circuits on errors from previous checkers. The final report contains all violations found across all checks.

## Data Shapes

### Checker input (the spec directory path)

- Every checker takes the resolved spec directory and nothing else. There is no
  shared parsed graph threaded between them: each loads `project.json` and the
  `module.json` files it needs for itself, reads them, and mutates nothing. The
  cost of re-reading is accepted in exchange for checkers that stay independent
  and individually testable.

### Checker output (appended to shared slice)

- ValidationError has exactly five fields, and no others:
  - check: string — which checker produced the entry. One of `schema`,
    `content`, `id`, `dag`, `name_consistency`, `test_coverage`,
    `requirement_coverage`, `coupled_section`
  - severity: string — always `error`. No checker can construct any other
    value, so severity never discriminates between entries
  - path: string — location in the spec: a file path (`alpha/module.json`),
    or a file plus JSON pointer (`project.json:/modules/0/name`), whichever
    is more specific
  - message: string — human-readable one-line summary
  - schema_path: string — the JSON Schema path that was violated. Present
    only on entries from the schema checker, omitted everywhere else

There is no `code`, no `node_id` and no `details` field. Identifying
information travels in `path` and `message`.

### ErrorReporter → stdout

- ValidationReport has exactly four fields, and no others:
  - valid: boolean — true when the sorted entry list is empty
  - error_count: integer — the length of that list. Every entry counts,
    because every entry is an error
  - warning_count: integer — always `0`. Nothing can produce a warning; the
    field stays in the contract because gates and CI assert on it
  - errors: list of ValidationError, sorted by path and by nothing else

There is no separate `warnings` list, no `checked_files` and no
`schema_version`.

Exit code: `0` if `valid == true`, `1` otherwise — read off the report the
reporter returns rather than re-derived from the slice. Shape changes here
cascade into downstream consumers (CI pipelines, `spex diff` — which builds
the merkle tree on demand and assumes a validated spec — and skills that
parse the report).
