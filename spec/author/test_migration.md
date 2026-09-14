# Migration tests

Acceptance scenarios for Migrator as `spex migrate` drives it over pre-versioning spec trees, read back through `spex validate` and `spex diff`.

## Setup

Three fixture trees under a temporary directory, each with no `.spex/` and no `spec/profile.json`:

- `tmp/titled/` — a spec whose requirements, at both scopes, carry `title` instead of `name`, and whose `project.json` has no `spec_version`.
- `tmp/legacy/` — `tmp/titled/` plus an `impl_sections` array on module `alpha` with two entries whose `content` paths resolve to `impl_a.md` and `impl_b.md`, and a `milestones` array in `project.json`.
- `tmp/current/` — a valid spec format version 1 tree, `spec_version` stamped.

Every command is invoked as `spex migrate --spec-dir <fixture>`. Before each run, `spex validate` over the fixture is recorded; `tmp/titled/` and `tmp/legacy/` fail it with `schema` errors and `tmp/current/` passes.

## Scenarios

### M1: The title-to-name rename yields a document the validator accepts

**Given** `tmp/titled/`.
**When** `spex migrate` is run, then `spex validate`.
**Then** migrate exits 0 and reports each renamed requirement by file and JSON pointer; no requirement carries `title` afterwards; `project.json` carries `spec_version: 1`; and `spex validate` is green. Every `id` is byte-identical to the input's, because identity is value-derived and the rename moves no identity hash.

### M2: Undeclared arrays are removed and their orphans reported by path

**Given** `tmp/legacy/`.
**When** `spex migrate` is run, then `spex validate`.
**Then** migrate exits 0. `alpha/module.json` has no `impl_sections` and `project.json` has no `milestones`; stdout lists, for each removed entry, its array, its name and — where it had one — the content file it orphaned, `alpha/impl_a.md` and `alpha/impl_b.md`, which are left on disk untouched. `spex validate` is green: the orphaned files are not referenced by any `content` field and no checker reads them.

### M3: A second run is a no-op

**Given** `tmp/titled/` after M1.
**When** `spex migrate` is run again.
**Then** exit 0, stdout says the spec is already at the current format, and the tree is byte-identical.

### M4: A current spec is untouched

**Given** `tmp/current/`.
**When** `spex migrate` is run.
**Then** exit 0 with the same already-current report as M3, and the tree is byte-identical — including formatting, since a no-op writes nothing.

## Edge cases

### M5: A tree the migration cannot make valid still reports what it did

**Given** `tmp/titled/` with one module requirement additionally lacking `preq_id`.
**When** `spex migrate` is run, then `spex validate`.
**Then** migrate exits 0 and performs the rename; `spex validate` reports the missing `preq_id` and nothing else. Migration owns format age, not authoring defects, and it never refuses because the destination will fail validation for another reason.

### M6: Migration needs no initialised project and touches nothing outside spec/

**Given** `tmp/legacy/` with no `.spex/`, and a copy carrying a healthy `.spex/`.
**When** `spex migrate` is run over both.
**Then** both exit 0 with byte-identical trees under `spec/`, and the copy's `.spex/` is byte-identical before and after.
