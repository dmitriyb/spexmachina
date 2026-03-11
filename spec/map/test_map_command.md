# Map Command Tests

## Setup

- Create a temporary spec directory with project.json, two modules, and a populated `.bead-map.json` with several records
- Build the `spex` binary with `go build`

## Scenarios

### spex map get — valid record ID

- **Input**: `spex map get 1`
- **Expected**: JSON output with all fields: id, spec_node_id, bead_id, module, component, content_file, spec_hash. Exit code 0.

### spex map get — unknown record ID

- **Input**: `spex map get 999`
- **Expected**: Error message "mapping record not found: 999". Exit code 1.

### spex map list — all records

- **Input**: `spex map list`
- **Expected**: JSON array of all mapping records. Sorted by ID. Exit code 0.

### spex map list — empty mapping file

- **Input**: `spex map list` with no records in `.bead-map.json`
- **Expected**: Empty JSON array `[]`. Exit code 0.

### spex check — bead with ready dependencies

- **Input**: `spex check <bead-id>` where all dependencies are satisfied
- **Expected**: JSON output with status "ready", resolved spec node info. Exit code 0.

### spex check — bead with blocked dependencies

- **Input**: `spex check <bead-id>` where a dependency module is not yet implemented
- **Expected**: JSON output with status "blocked", list of blockers. Exit code 1.

### spex check — unknown bead

- **Input**: `spex check unknown-bead-id`
- **Expected**: Error message "no mapping record for bead unknown-bead-id". Exit code 1.

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
