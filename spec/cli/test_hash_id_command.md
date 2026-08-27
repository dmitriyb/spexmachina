# Hash ID Command Tests

Integration and acceptance tests for HashIDCommand (component 3).

## Setup

Tests invoke the compiled `spex` binary (or the cobra command directly via Go test) with various flag combinations. Most scenarios run with no `spec/profile.json` under the spec directory, so the built-in default profile resolves; E4b is the one that places a profile file. Given the resolved profile, the output is a pure function of the flags.

## Scenarios

### S1: Component hash matches schema.IdentityHash

Run `spex hash-id --module plan --type component --name NodeMatcher`.

Assert stdout equals `schema.IdentityHash("plan", "component", "NodeMatcher")` and exit code is 0.

### S2: Module hash (no --module flag)

Run `spex hash-id --type module --name schema`.

Assert stdout equals `schema.IdentityHash("module", "schema")` and exit code is 0.

### S3: Project-level requirement (no --module flag)

Run `spex hash-id --type requirement --name "Validate spec structure"`.

Assert stdout equals `schema.IdentityHash("project", "requirement", "Validate spec structure")` and exit code is 0.

### S4: Module-level requirement (with --module flag)

Run `spex hash-id --module validator --type requirement --name "ID uniqueness"`.

Assert stdout equals `schema.IdentityHash("validator", "requirement", "ID uniqueness")` and exit code is 0.

### S5: Retired node types are rejected

For each type in `[milestone, scenario]`:

Run `spex hash-id --type <type> --name x`.

Assert exit code is non-zero and the error mentions an unknown type. Both types were deleted from the schema; a command that still minted ids for them would hand authors ids for nodes the schema rejects.

### S6: (retired — absorbed into S5)

S6 asserted that `--type scenario` produced `test_plan/scenario/<name>` and exited 0. The `scenario` node type no longer exists, so the assertion inverted and now runs as one of S5's two cases beside `milestone`; the test that carried S6 was deleted rather than rewritten. The number is left vacant instead of reused, because S7 through S9 name the tests that carry them and renumbering would break that correspondence.

### S7: All module-scoped types produce correct hashes

For each type in `[component, data_flow, test_section, api]`:

Run `spex hash-id --module alpha --type <type> --name Foo`.

Assert stdout equals `schema.IdentityHash("alpha", "<type>", "Foo")`.

### S8: Output matches hex pattern

Run 8 different invocations with varying inputs.

Assert every output line matches `^[a-f0-9]{12}$` — exactly 12 lowercase hex characters, no trailing newline junk.

### S9: Deterministic — same input produces same output

Run `spex hash-id --module plan --type component --name NodeMatcher` twice.

Assert both outputs are identical.

## Error Scenarios

### E1: Missing --type flag

Run `spex hash-id --name Foo`.

Assert exit code is 1 and stderr contains a usage error mentioning `--type`.

### E2: Missing --name flag

Run `spex hash-id --type component`.

Assert exit code is 1 and stderr contains a usage error mentioning `--name`.

### E3: Module-scoped type without --module

Run `spex hash-id --type component --name Foo`.

Assert exit code is 1 and stderr contains `--module is required for type "component"`.

### E4: Unknown --type value

Run `spex hash-id --type bogus --name Foo`.

Assert exit code is 1 and stderr lists valid types. The valid list is the node types the resolved profile declares plus the fixed `module` type, not a fixed switch: under the default profile it is today's list (requirement, component, data_flow, test_section, api, module), and the assertion compares stderr against the resolved profile's declaration plus `module` rather than a literal.

### E4b: Profile-declared type accepted

With a `spec/profile.json` declaring an `endpoint` type, run `spex hash-id --module alpha --type endpoint --name Foo`.

Assert stdout equals `schema.IdentityHash("alpha", "endpoint", "Foo")` and exit code is 0 — the identity string uses the declared type name as its middle part, through the same unchanged IdentityHash function. Without the profile file the same invocation exits 1 as an unknown type.

### E5: --module flag silently ignored for project-level types

Run `spex hash-id --module ignored --type module --name schema`.

Assert stdout equals `schema.IdentityHash("module", "schema")` (the `--module` flag value is not part of the identity string). Exit code is 0.
