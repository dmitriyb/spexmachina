# Leaf and profile tests

Integration and acceptance scenarios for LeafScaffolder and ProfileInspector as `spex leaf scaffold` and `spex profile show` drive them, and as `spex node add` drives the scaffolder on its way through. The two components meet in every scenario: what the scaffolder writes is what the profile the inspector prints declares.

## Setup

The fixture of the node editing tests — `tmp/spec/` with module `alpha`, components Comp1 and Comp2, test section T1 and api `demo run` — plus a second fixture, `tmp/custom/`, identical in layout but carrying a `spec/profile.json` at profile format version 2 whose types share no name with the defaults: a module-scoped content-bearing `endpoint` with content prefix `ep_` and leaf sections `Contract` and `Errors`, and a module-scoped `resource` with no leaf sections. Every command is invoked with `--spec-dir` naming the fixture. A skeleton is asserted by reading the file back: its first line, its `##` headings in order, and its placeholder lines.

## Scenarios

### L1: The skeleton carries the profile's headings and one placeholder per owed link

**Given** `tmp/spec/` with `arch_comp2.md` deleted and `alpha/module.json` unchanged, so Comp2 is a declared component whose leaf is missing.
**When** `spex leaf scaffold` is run with Comp2's id.
**Then** exit 0. `arch_comp2.md` exists, opens with `# Comp2`, carries in order the `##` headings the default profile declares under `leaf_sections` for `component`, and carries one placeholder line holding a typed link to Comp1 for the one `uses` edge Comp2 declares. `spex validate` is green, and `scripts/link-check.sh tmp/spec diff.json` passes over the diff the write produced.

### L2: A non-empty leaf is never overwritten

**Given** `tmp/spec/` intact, where `arch_comp2.md` holds prose.
**When** `spex leaf scaffold` is run with Comp2's id.
**Then** non-zero exit, the file is byte-identical, and the `fix` names the file and says to empty or move it. The same run over a leaf that is present but zero bytes long exits 0 and writes the skeleton.

### L3: Scaffolding is idempotent

**Given** `tmp/spec/` after L1.
**When** `spex leaf scaffold` is run again with Comp2's id.
**Then** exit 0, stdout says the leaf already carries the skeleton, and the file is byte-identical.

### L4: The headings come from the profile, not the command

**Given** `tmp/custom/` with an `endpoint` node declared in `alpha/module.json` and no leaf on disk.
**When** `spex leaf scaffold` is run with the endpoint's id, and `spex profile show` is run over the same fixture.
**Then** the leaf is written at `ep_<slug>.md` with the headings `## Contract` and `## Errors` and nothing else, and those two strings appear, in that order, under the `endpoint` type's `leaf_sections` in what `spex profile show` printed. A `resource` node scaffolded the same way gets its title and its placeholder lines and no `##` heading at all.

### L5: The printed profile is the resolved profile

**Given** `tmp/spec/` with no `spec/profile.json`.
**When** `spex profile show` is run.
**Then** exit 0 and stdout is one JSON document equal, as a JSON value, to the built-in default profile, with `profile_version` 2, `content_prefix` and `leaf_sections` present on every content-bearing type. Writing that document to `tmp/spec/profile.json` and running `spex profile show` again prints an equal document; `spex validate` over the fixture with that file in place is green.

### L6: A version 1 profile still resolves, a version 3 one is refused

**Given** `tmp/spec/` with a `spec/profile.json` at `profile_version` 1 declaring the default types without `content_prefix` or `leaf_sections`, and a copy declaring `profile_version` 3.
**When** `spex profile show` is run over each.
**Then** the first exits 0 and prints a document in which every content-bearing type carries the default conventions — `arch_`, `flow_`, `test_` and the default headings — filled in; the second exits non-zero with the one message naming the file, version 3 and the supported range 1 to 2, and `spex validate` over the same fixture fails with the same message.

## Edge cases

### L7: A node the profile marks as not content-bearing has no leaf to scaffold

**Given** `tmp/spec/`.
**When** `spex leaf scaffold` is run with the api `demo run`'s id.
**Then** non-zero exit, no file written, and the error names the type as carrying no content leaf, the `fix` listing the content-bearing types the profile declares.

### L8: Adding a node scaffolds through the same path

**Given** `tmp/spec/`.
**When** `spex node add` is run with type `data_flow`, module `alpha`, name `Demo flow`, and then `spex leaf scaffold` with the new node's id.
**Then** `flow_demo_flow.md` exists after the add with the `data_flow` headings the profile declares, and the scaffold run reports the skeleton already present and changes nothing.
