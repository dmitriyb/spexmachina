# Map Command Tests

## Setup

- Create a temporary spec directory with project.json, two modules, and a populated `.bead-map.json` with several records
- Build the `spex` binary with `go build`

## Scenarios

### spex map get — valid record ID

- **Input**: `spex map get 1`
- **Expected**: JSON output carrying the record's fields — id, spec_node_id, bead_id, bead_type, module, component, content_file, spec_hash — with node_type and bead_status appearing as well on a record that sets them. Exit code 0.

### spex map get — unknown record ID

- **Input**: `spex map get 999`
- **Expected**: Error message "mapping record not found: 999". Exit code 1.

### spex map list — all records

- **Input**: `spex map list`
- **Expected**: JSON array of all mapping records. Sorted by ID. Exit code 0.

### spex map list — empty mapping file

- **Input**: `spex map list` with no records in `.bead-map.json`
- **Expected**: Empty JSON array `[]`. Exit code 0.

### spex map context — valid record ID

- **Input**: `spex map context 1`
- **Expected**: JSON output with record, arch_file, test_files, flow_files, module_file. Exit code 0.

### spex map context — unknown record ID

- **Input**: `spex map context 999`
- **Expected**: Error message. Exit code 1.

### Output format consistency

- **Input**: Run `spex map get 1` and pipe through `jq .`
- **Expected**: Valid JSON. Fields match the mapping record schema.

## Edge Cases

### No mapping file exists

- **Input**: `spex map list` in a spec directory with no `.bead-map.json`
- **Expected**: Empty JSON array `[]`. Exit code 0. File is NOT created by read-only commands.

### Concurrent CLI invocations

- **Input**: Two parallel `spex map get` calls for different record IDs
- **Expected**: Both return correct results. No file locking errors.
