# SchemaChecker

Validates JSON files against the embedded JSON Schemas.

## Responsibilities

- Load the embedded project and module schemas from the schema package
- Parse and validate `spec/project.json` against the project schema
- Parse and validate each `spec/<module>/module.json` against the module schema
- Collect all schema violations as structured errors

## Interface

```go
func CheckSchema(specDir string) []ValidationError
```

## Behavior

1. Read `project.json` from the spec directory
2. Validate it against the project schema
3. For each module declared in `project.json`, read `<module.path>/module.json`
4. Validate each against the module schema
5. Return all violations — do not stop at the first error

## Error Format

Each error includes:
- `path`: JSON path to the violating field (e.g., `project.modules[0].name`)
- `message`: Human-readable description of the violation
- `schema_path`: JSON Schema path that was violated
