# Migrator

The component behind `spex migrate`: the mechanical half of the migration the format-version contract promised and the binary never performed. [[fae0f8a6db18|Migrate a spec to the current format]] is its contract.

## What it rewrites

Over a spec tree, in one pass:

- **the title-to-name rename** on every requirement at both scopes — the one deliberate break of spec format version 1, and the reason a pre-versioning document fails schema validation. Identity is value-derived, so the rename moves no id; each requirement leaf's serialization moves once, which the adoption refresh absorbs;
- **removal of every array the resolved profile does not declare** — `impl_sections`, `milestones`, `test_plan`, or anything else a document carries under a key no declared type owns. Each removed entry is reported by file, array and name, and where the entry named a content file, the file's path is reported as orphaned and the file is left on disk untouched: the tool cannot know which leaf the content belongs in, so a person folds it into an arch leaf or discards it, and `spex validate` does not read an unreferenced file;
- **`spec_version` stamped** in `project.json` at the current version, the one declaration per project the contract allows.

A migrated tree is one `spex validate` accepts as far as format age goes. It may still fail for an authoring defect the input carried — a module requirement with no `preq_id`, a component nothing describes — and the migrator never refuses on that account: it owns format age, not authoring, and it reports what it did whatever the validator will say next.

## Idempotence

A tree already at the current format is reported as current and left byte-identical: nothing is rewritten, so nothing is reformatted. A second run over a migrated tree is that case. The command therefore has no `--check`: running it is the check.

## Boundaries

The migrator needs no initialised project and reads nothing under `.spex/` — an adopter's spec is migrated and validated before it is ever initialised, which is the sequence the ingest adopt-mode proposal depends on. Its writes pass through [[b9e7b96f6aa7|ObligationReporter]] like every other write in this module, under the reporter's one rule: a refusal is a finding the change introduced. The rename and the removals introduce none, so the input's own defects — present before the command ran — reach the report as obligations rather than as a refusal, and the migration lands.
