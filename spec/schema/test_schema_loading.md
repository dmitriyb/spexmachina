# Schema Loading Tests

Integration and acceptance tests for SchemaLoader, ProfileLoader, JournalLineSchema and TaskStateSchema — the two document schemas are exercised through the commands that validate journal lines and task-state artifacts against them (J1, T1). The SchemaLoader is the Go package (`schema/schema.go`) that embeds the schema frame, the default profile document `defaultProfile.json`, the journal-line schema `journal-line.schema.json` and the task-state schema `task-state.schema.json` via `go:embed`, composes the effective project and module schemas, and exposes them through `ProjectSchema()`, `ModuleSchema()`, `JournalLineSchema()` and `TaskStateSchema()` functions — the two composed reads take no arguments and always compose from the built-in default profile, consulting no file. It also exposes the `IdentityHash(parts ...string) string` function that defines what spec node IDs look like. ProfileLoader owns the file-backed resolution: it reads `spec/profile.json` when present, falls back to the built-in default profile otherwise, validates the document, and exposes the resolved profile. A caller that needs a project's own profile reflected in the composed schemas takes the resolve-then-compose path — ProfileLoader's resolution handed to the composition entry points directly — which the P-scenarios below exercise; the zero-argument reads are the default-profile convenience over the same composition.

These tests verify that the embedding is deterministic, that the composed schemas reproduce the shipped static documents under the default profile, and that profile resolution and composition behave as declared.

## Setup

### Build Preconditions


- The `schema` package compiles successfully (`go build ./schema/...`).
- The `go:embed` directive references `project.schema.json`, `module.schema.json`, `journal-line.schema.json`, `task-state.schema.json` and `defaultProfile.json`, all of which exist in the `schema/` directory at build time.
- No external file system access is needed at runtime — schemas are baked into the binary.

## Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.

## Edge Cases

### E3: Schema content is deterministic across builds

**Given** the same `schema` package source built twice, with no runtime file system access.

**Steps:**
1. Build the binary, call `ProjectSchema()`, store the SHA-256 hash.
2. Build again (same source), call `ProjectSchema()`, compute hash.

**Expected:** Hashes are identical.

**Verifies:** The embed process is deterministic. This matters because the merkle module will hash schema files — non-deterministic embedding would break snapshot reproducibility.

## IdentityHash Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.

## ProfileLoader Scenarios

These scenarios cover profile resolution and the composition acceptance criterion. The spec directory used is a fixture; "the default profile" means the built-in declaration of today's ontology.

### P2: Composed schemas equal the shipped static documents (golden test)

**Given** the built-in default profile and the shipped static project and module schema documents standing as the golden records.

**Steps:** Resolve the default profile, compose the project and module schemas from it, and compare each against the shipped static schema document as a golden record — equal as JSON values, independent of formatting and key order, since the shipped copies are hand-formatted and the composer emits compact key-sorted JSON.
**Expected:** Both composed documents reproduce the shipped documents' content; a composition that does not is a test failure, not a tolerated drift.
**Verifies:** The acceptance criterion that the default profile reproduces the shipped documents exactly. The one deliberate on-disk change for existing project.json and module.json files is the requirement type's title-to-name rename — spec format version 1's break, which this comparison does not exercise.

### P4: A declared custom type reaches the composed schema

**Given** a valid `spec/profile.json` declaring a module-scoped `endpoint` type with plural key `endpoints`, a required content leaf, a text field carrying an enumeration and a reference field targeting `component`.

**Steps:** Place a valid `spec/profile.json` declaring an `endpoint` type, module-scoped, plural key `endpoints`, content leaf required, with a text field carrying an enumeration and a reference field targeting `component`. Resolve and compose the module schema.
**Expected:** The composed module schema's `properties` contains an `endpoints` array validated with the same envelope constraints (identity-hash id, name, required content) the built-in types get, and its entry definition carries one property per declared field — enum-constrained for the text field, an array of identity hashes for the reference field; `additionalProperties: false` still rejects any array the profile does not declare and any field the type does not.

### P5: Resolution is deterministic

**Given** one profile in both forms — the built-in default and a file-backed `spec/profile.json`.

**Steps:** Resolve the same profile (default and file-backed) twice.
**Expected:** Byte-identical resolved profiles and byte-identical composed schemas across runs.

### P6: A new reference field on a built-in type composes like any other

**Given** a `spec/profile.json` declaring the default ontology plus an `audits` reference field on the built-in `component` type, targeting `component`.

**Steps:** Place a `spec/profile.json` declaring the default ontology plus an `audits` reference field — one the built-in shape never carried — on the built-in `component` type, targeting `component`. Resolve the profile and compose the module schema.
**Expected:** Resolution succeeds and the composed component definition carries an `audits` array-of-identity-hash property beside the built-in fields.
**Verifies:** The interim rule refusing a new edge kind sourced at a built-in type is retired: built-in types compose from field declarations through the same path declared types take, so there is no frame-fixed definition left for a new field to be unable to reach. The v1 scenario asserting that refusal is superseded by this one.


## JournalLineSchema Scenarios

### J1: A journal line that fails the schema stops every command that folds the journal
**Given** an initialised project whose `.spex/history.jsonl` carries, as its third line, a well-formed JSON object that violates the journal-line schema — an `event` outside the declared set.
**When** `spex map list` and then `spex plan` are executed over it.
**Then** each exits with the not-a-spex-project code before reading the store, stderr naming `spex doctor` (for `spex map list`, the offending line as well), and no output document is produced. The same line as valid JSON with a declared `event` is accepted by both.

## TaskStateSchema Scenarios

### T1: A task-state artifact that fails the schema is refused by plan
**Given** a `--tasks` file that is valid JSON but violates the task-state schema in one of three ways: an entry with `status: "closed"`, an envelope with `version: 2`, or the tracker's raw listing shape instead of `{version, tasks}`.
**When** `spex plan --diff <diff> --tasks <file>` is executed.
**Then** exit code is 1 and stderr names the input that failed — the `--tasks` file; no changeset is written. The same file with `version: 1` and only `open` / `in_progress` statuses is accepted.

