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

**Given** a `spex` binary built with version, commit and date injected through ldflags.
**Input**: `spex version` (built with ldflags above)
**Expected**: Exit 0. Output contains:
- `v1.2.3`
- `abc1234`
- `2026-01-01T00:00:00Z`
- A Go version string matching `go1.*`

### 2. Version with dev defaults

**Given** a `spex` binary built without ldflags, so no version, commit or date was injected.
**Input**: `spex version` (built without ldflags)
**Expected**: Exit 0. Output contains:
- `dev`
- `unknown` (for commit and date)
- A Go version string matching `go1.*`

### 3. Version --help

**Given** the compiled `spex` binary, invoked in any working directory.
**Input**: `spex version --help`
**Expected**: Exit 0. Stdout contains usage text for the version subcommand.

### 4. Version exits cleanly

**Given** the compiled `spex` binary, invoked in any working directory.
**Input**: `spex version`
**Expected**: Exit code 0. No output to stderr.

## Edge Cases

- **Extra arguments**: `spex version foo` — rejected, never silently ignored. Expect exit 1, nothing on stdout, and `unknown command "foo" for "spex version"` on stderr.
- **Version as flag**: `spex --version` — not supported (version is a subcommand, not a flag). Expect exit 1, nothing on stdout, and `unknown flag: --version` on stderr.
