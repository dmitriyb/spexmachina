# Version Command Tests

Integration and acceptance tests for the VersionCommand component.

## Setup

- Build with ldflags to inject known version values:
  ```sh
  go build -ldflags "-X main.version=v1.2.3 -X main.commit=abc1234 -X main.date=2026-01-01T00:00:00Z" -o bin/spex ./cmd/spex/
  ```
- For dev build tests, build without ldflags: `go build -o bin/spex ./cmd/spex/`.

## Scenarios

### 1. Version with injected values

**Input**: `spex version` (built with ldflags above)
**Expected**: Exit 0. Output contains:
- `v1.2.3`
- `abc1234`
- `2026-01-01T00:00:00Z`
- A Go version string matching `go1.*`

### 2. Version with dev defaults

**Input**: `spex version` (built without ldflags)
**Expected**: Exit 0. Output contains:
- `dev`
- `unknown` (for commit and date)
- A Go version string matching `go1.*`

### 3. Version --help

**Input**: `spex version --help`
**Expected**: Exit 0. Stdout contains usage text for the version subcommand.

### 4. Version exits cleanly

**Input**: `spex version`
**Expected**: Exit code 0. No output to stderr.

## Edge Cases

- **Extra arguments**: `spex version foo` — should be ignored or produce a clean error (cobra default is to ignore extra args unless `Args: cobra.NoArgs` is set). If `NoArgs` is set, expect exit 1 with "unknown command" error.
- **Version as flag**: `spex --version` — not supported (version is a subcommand, not a flag). Cobra prints help or "unknown flag" error.
