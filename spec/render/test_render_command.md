# Render Command Tests

## Setup

All scenarios invoke the `spex render` CLI command against a temporary spec directory containing a valid spec fixture. The fixture is the same multi-module spec described in the SpecReader and Renderer test setups:

- `project.json` with 2 modules (alpha, beta), project-level requirements, and milestones
- Module alpha: 2 components with content leaves, 1 impl_section, 1 data_flow
- Module beta: 1 component with content leaf, 1 impl_section

The command is executed as a subprocess using `os/exec` to test the full CLI path including flag parsing, argument handling, exit codes, and stdout/stderr behavior.

**Binary path:** Tests build `spex` from source or use a pre-built binary. The spec directory is passed as a positional argument.

## Scenarios

### S1: Default format produces markdown

**Given** a valid spec directory.

**When** `spex render <dir>` is executed with no `--format` flag.

**Then:**
- Exit code is 0
- Stdout contains markdown output starting with `# ` (project heading)
- Output matches the same content as `spex render <dir> --format markdown`

### S2: Explicit markdown format

**Given** a valid spec directory.

**When** `spex render <dir> --format markdown` is executed.

**Then:**
- Exit code is 0
- Stdout contains a collated markdown document
- Output includes project heading, project requirements, and per-module sections
- Output contains inlined content from `arch_*.md`, `impl_*.md`, and `flow_*.md` files
- Stderr is empty

### S3: DOT format output

**Given** a valid spec directory.

**When** `spex render <dir> --format dot` is executed.

**Then:**
- Exit code is 0
- Stdout starts with `digraph spec {`
- Output contains `subgraph cluster_alpha` and `subgraph cluster_beta`
- Output ends with `}`
- Stderr is empty

### S4: JSON format output

**Given** a valid spec directory.

**When** `spex render <dir> --format json` is executed.

**Then:**
- Exit code is 0
- Stdout is valid JSON
- Parsed JSON has top-level `"nodes"` and `"edges"` arrays
- Nodes array contains project, module, requirement, component, impl_section, and data_flow entries
- Stderr is empty

### S5: Output is written to stdout only (composable)

**Given** a valid spec directory.

**When** `spex render <dir> --format json` is executed.

**Then:**
- All rendered content appears on stdout
- No files are created or modified in the spec directory or working directory
- The command produces no side effects beyond stdout output

### S6: Piping to downstream tools

**Given** a valid spec directory.

**When** `spex render <dir> --format json | jq '.nodes | length'` is executed in a shell.

**Then:**
- The pipeline completes with exit code 0
- Output is a single integer representing the total node count
- This verifies the JSON output is well-formed and pipeable

**When** `spex render <dir> --format dot | dot -Tsvg` is executed (if graphviz is available).

**Then:**
- The pipeline completes with exit code 0
- Output is valid SVG
- This verifies the DOT output is syntactically correct

### S7: Spec directory as positional argument

**Given** a valid spec directory at `/tmp/test-spec`.

**When** `spex render /tmp/test-spec` is executed.

**Then:**
- The command reads from the specified directory
- Exit code is 0
- Output is the rendered spec from that directory

### S8: Current directory as implicit spec root

**Given** the working directory is a valid spec root (contains `project.json` and module subdirectories).

**When** `spex render` is executed with no positional argument.

**Then:**
- The command uses the current working directory as the spec root
- Exit code is 0
- Output is the rendered spec

### S9: Markdown output round-trip consistency

**Given** a valid spec directory.

**When** `spex render <dir> --format markdown` is executed twice.

**Then:**
- Both invocations produce byte-identical output
- This validates determinism: same input always produces same output, with no timestamps, random values, or non-deterministic ordering

### S10: JSON output determinism

**Given** a valid spec directory.

**When** `spex render <dir> --format json` is executed twice.

**Then:**
- Both invocations produce byte-identical JSON
- Node ordering is deterministic (declaration order, not random map iteration)
- Edge ordering is deterministic

## Edge Cases

### E1: Invalid format flag

**Given** a valid spec directory.

**When** `spex render <dir> --format xml` is executed.

**Then:**
- Exit code is 1 (non-zero)
- Stderr contains an error message indicating `xml` is not a valid format
- Stdout is empty (no partial output)
- The error message lists the valid formats: markdown, dot, json

### E2: Non-existent spec directory

**Given** the path `/tmp/nonexistent-spec-dir` does not exist.

**When** `spex render /tmp/nonexistent-spec-dir` is executed.

**Then:**
- Exit code is 1
- Stderr contains an error message indicating the directory does not exist
- Stdout is empty

### E3: Spec directory missing project.json

**Given** a directory exists but contains no `project.json`.

**When** `spex render <dir>` is executed.

**Then:**
- Exit code is 1
- Stderr contains an error message about missing `project.json`
- Stdout is empty (no partial output on error)

### E4: Spec with broken content reference

**Given** a spec where a component references `arch_missing.md` which does not exist on disk.

**When** `spex render <dir>` is executed.

**Then:**
- Exit code is 1
- Stderr identifies the missing content file and which component references it
- Stdout is empty

### E5: Large spec performance

**Given** a spec with 15 modules, each containing 8 components with content leaves averaging 2KB each.

**When** `spex render <dir> --format json` is executed.

**Then:**
- The command completes within 2 seconds
- All modules, components, and content are present in the output
- Memory usage stays reasonable (no unbounded buffering)

### E6: Stderr and stdout separation

**Given** a spec with a validation warning (e.g., a component with no `implements` edges).

**When** `spex render <dir> --format markdown` is executed.

**Then:**
- Any warnings or diagnostics go to stderr
- The rendered output goes to stdout
- A downstream pipe reading stdout receives only the rendered content, not mixed with diagnostics

### E7: Exit code contract

**Given** various inputs.

**Then** the exit code follows the convention:
- 0: success, rendered output on stdout
- 1: error (missing files, invalid format, parse failure), error details on stderr, nothing on stdout

No other exit codes are used. The command never exits with code 2 or higher.

### E8: Empty flag value

**Given** a valid spec directory.

**When** `spex render <dir> --format ""` is executed.

**Then:**
- Exit code is 1
- Stderr contains an error message indicating the format is invalid or empty
- Alternatively, the empty string is treated as the default (markdown) -- either behavior is acceptable as long as it is consistent and documented

### E9: Help flag

**When** `spex render --help` is executed.

**Then:**
- Exit code is 0
- Output describes the render command usage
- Lists the available `--format` options (markdown, dot, json)
- Documents the positional `[dir]` argument
