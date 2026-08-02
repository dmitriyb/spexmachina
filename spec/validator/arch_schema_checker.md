# SchemaChecker

Checks `project.json` and every `module.json` against the JSON Schemas the tool ships with, which is the whole of [[8599f07272ad|JSON schema conformance]].

## Responsibilities

- Load the project and module schemas embedded in the binary — a spec directory supplies neither
- Parse and validate `spec/project.json` against the project schema
- Parse and validate each `spec/<module>/module.json` against the module schema
- Collect all schema violations as structured errors

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when every file conforms. It reads `project.json` first and takes the set of modules to check from that file's `modules` array, so a `project.json` that cannot be read or cannot be parsed as JSON yields that single entry and no module.json is opened. A `project.json` that parses but violates the schema is different: its violations are reported and the modules it declares are still checked. The checker never mutates the spec and never writes output — aggregation, sorting and formatting belong to ErrorReporter.

The spec directory is an input rather than a fixed location, and no path inside it is treated specially, so spex-machina's own spec directory is checked by the same call as any other. That is the whole of [[b95862937dd0|self-validation]]: there is no separate mode to enter.

## Behavior

1. Read `project.json` from the spec directory
2. Validate it against the project schema
3. For each module declared in `project.json`, read `<module.path>/module.json`
4. Validate each against the module schema
5. Return all violations — do not stop at the first error

The embedded schemas are compiled once per process and then reused, and each file is validated in a single pass, so a spec of a hundred modules costs one compilation and one validation call per file — well inside the budget [[b42c5cdf874b|fast validation]] sets.

## Error Format

Each error includes:
- `path`: the file's display path — `project.json`, or `<module-path>/module.json` — followed by a JSON pointer to the violating field when the violation locates one (e.g., `project.json:/modules/0/name`)
- `message`: Human-readable description of the violation
- `schema_path`: JSON Schema path that was violated
