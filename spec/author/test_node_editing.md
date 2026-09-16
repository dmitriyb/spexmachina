# Node editing tests

Integration and acceptance scenarios for NodeEditor, NodeRenamer and EdgeEditor as `spex node add`, `spex node set`, `spex node remove`, `spex node rename`, `spex edge add` and `spex edge remove` drive them. Every scenario runs a command over a fixture tree, then reads the tree back through `spex validate` and `spex diff`, because the property under test is that a command's write is indistinguishable from a correct hand edit.

## Setup

A fixture spec under a temporary directory, built once per scenario from a committed copy:

```
tmp/spec/
  project.json          # two requirements: P1, and P2 declaring derivation pending; one module: alpha
  alpha/
    module.json         # requirement R1 (preq → project requirement P1)
                        # components Comp1 (implements R1), Comp2 (uses Comp1)
                        # test_section T1 describing Comp1 and Comp2
                        # api "demo run" provided_by Comp1
    arch_comp1.md       # links [[R1]] and is linked from arch_comp2.md
    arch_comp2.md       # links [[Comp1]]
    test_t1.md          # links [[Comp1]] and [[Comp2]]
```

The fixture has no `spec/profile.json`, so the default profile applies, and no `.spex/` directory, because the authoring commands need none. Every command is invoked as `spex <command> --spec-dir tmp/spec/`; where a scenario reads the tree back, `spex validate --spec-dir tmp/spec/` and `spex diff --spec-dir tmp/spec/ --json` are run against a snapshot taken of the fixture before the command ran. A refusal is asserted on three things together: a non-zero exit, no byte of the tree changed, and an error document on stdout whose `fix` field is what the scenario names.

The parity oracle shared by the refusal scenarios: the same change applied by hand to a copy of the fixture, then `spex validate` over the copy. The command's refusal and the validator's error must carry the same `check` value and the same message. The same oracle reads the other way for an accepted write: the same change made by hand, then `spex diff --json` over the copy against the fixture's snapshot, must carry the entries the command printed under `obligations`, entry for entry.

## Scenarios

### N1: Adding a component places it, derives its id and scaffolds its leaf

**Given** the fixture, and a snapshot of it.
**When** `spex node add` is run with type `component`, module `alpha` and name `Widget`.
**Then** exit 0. `alpha/module.json` carries a new entry in `components` whose `id` equals what `spex hash-id --type component --module alpha --name Widget` prints and whose `content` is `arch_widget.md`; that file exists and opens with the heading `# Widget`. `spex validate` reports the one error an undescribed component earns under the default profile's coverage chain — `test_coverage` for `Widget`, since T1 describes Comp1 and Comp2 only — and the command's stdout carried that finding under `obligations`, not as a refusal. `spex diff --json` reports exactly one `added` change of node type `component` plus the `meta` change of module `alpha`, and its `errors` array holds the meta-obligation entries for Comp1 and Comp2 — the entries the command printed under `obligations` beside the coverage finding.

### N2: A project-scoped type lands in project.json

**Given** the fixture.
**When** `spex node add` is run with type `requirement`, no module, name `New rule`, type field `functional` and priority `2`.
**Then** exit 0. `project.json` gains the entry under `requirements` with the derived id; no `module.json` changed. `spex validate` reports the one error a new project requirement earns — that nothing derives from it — and the command's stdout carried the same finding under `obligations` before the write, not as a refusal, because the validator would have written it too.

### N3: A module is registered and skeletoned by one command

**Given** the fixture.
**When** `spex node add` is run with type `module` and name `beta`.
**Then** exit 0. `project.json`'s `modules` gains an entry with `name` and `path` `beta` and the derived module id; `tmp/spec/beta/module.json` exists, declares `name: "beta"` and nothing else; `spex validate` is green, and `spex render --format json --slim` lists `beta` among the modules.

### N4: An undeclared type is refused with the declared list as the fix

**Given** the fixture.
**When** `spex node add` is run with type `impl_section`, module `alpha` and name `Anything`.
**Then** non-zero exit, no file changed, and the error's `fix` lists the types the default profile declares — `requirement`, `component`, `data_flow`, `test_section`, `api` — and `module`. The message names the type as undeclared, never a fixed list compiled into the command: the same run over a fixture carrying a `spec/profile.json` that declares `impl_section` succeeds.

### N5: A name the tokenizer would not reproduce is refused with the declarable form

**Given** the fixture.
**When** `spex node add` is run with type `api`, module `alpha`, name `demo run [--json]`.
**Then** non-zero exit, no file changed, and the `fix` shows `demo run --json` as the form to declare. The parity oracle: the same entry written by hand fails `spex validate` with the same message.

### N6: Removing a referenced node refuses and lists every inbound reference

**Given** the fixture.
**When** `spex node remove` is run with Comp1's id.
**Then** non-zero exit and no file changed. The error lists every inbound reference by file and field: Comp2's `uses`, T1's `describes`, the api's `provided_by`, and the links in `arch_comp2.md` and `test_t1.md` by file and line. The `fix` names `--force` and, for each reference, the `spex edge remove` invocation that would retarget it.

### N7: A forced removal lists what it left dangling

**Given** the fixture.
**When** `spex node remove` is run with Comp1's id and `--force`.
**Then** exit 0. Comp1's entry and `arch_comp1.md` are gone; the same inbound references N6 listed are printed as dangling, and `spex validate` now reports each of them — `id` errors for the arrays, `link` errors for the leaves — and the `requirement_coverage` error for R1, which Comp1 alone implemented; that is exactly the list the command printed. `spex diff --json` reports one `removed` component. Whether a `surviving_name` error fires for the display text of the dangling links in `arch_comp2.md` and `test_t1.md` is the removal sweep's rule and is not asserted here, as in N8; a variant of the fixture whose `arch_comp2.md` also mentions `Comp1` in plain prose before the removal gets the `surviving_name` error for that line.

### N8: A rename is one transaction across arrays, leaves and the content file

**Given** the fixture, and a snapshot of it.
**When** `spex node rename` is run with Comp1's id and the new name `Core`.
**Then** exit 0. `alpha/module.json` carries `Core` with the id `spex hash-id --type component --module alpha --name Core` prints; Comp2's `uses`, T1's `describes` and the api's `provided_by` carry the new id; `arch_comp2.md` and `test_t1.md` link the new id with their display text unchanged; `arch_core.md` exists with `arch_comp1.md`'s content and `arch_comp1.md` is gone. Stdout names `Comp1` as the retired name. `spex validate` is green, and `spex diff --json` reports one `removed` and one `added` component and nothing dangling. Whether a `surviving_name` error fires for the mentions of `Comp1` the fixture's prose still carries is the removal sweep's rule and is not asserted here.

### N9: An edge is checked against the profile before it is written

**Given** the fixture.
**When** `spex edge add` is run with source Comp2, field `describes`, target Comp1.
**Then** non-zero exit and no file changed: the profile declares `describes` on `test_section`, not on `component`, and the `fix` lists the reference fields the profile does declare on `component` — `implements` and `uses`. A second run with field `uses` and target R1 is refused likewise, the `fix` naming `component` as the only target type `uses` permits.

### N10: A cycle is refused before it is written

**Given** the fixture, where Comp2 already uses Comp1.
**When** `spex edge add` is run with source Comp1, field `uses`, target Comp2.
**Then** non-zero exit, no file changed, and the refusal is the validator's `dag` error naming the cycle. The parity oracle: the same entry written by hand fails `spex validate` with the same `dag` message.

### N11: An existing edge is a no-op, and its removal restores the tree

**Given** the fixture, and a snapshot of it.
**When** `spex edge add` is run with source Comp2, field `uses`, target Comp1, and then `spex edge remove` with the same three values.
**Then** the add exits 0, prints that the entry already exists, and leaves the tree byte-identical; the remove exits 0, drops the entry, and `spex diff --json` against the snapshot reports the `meta` change of `alpha` with the meta obligations on Comp1 and Comp2 — printed by the remove under `obligations` before it wrote.

### N16: Setting a module requirement's description obliges exactly the implementing leaves

**Given** the fixture, and a snapshot of it.
**When** `spex node set` is run with R1's id and `--field description=<new text>`.
**Then** exit 0. `alpha/module.json` carries the new description on R1 and every other field of the entry as it was; no other file changed. The write report's `obligations` holds exactly one entry, of type `incomplete_change` with `path` R1's id and `related` naming Comp1 alone — the requirement-changed rule — and no meta obligation for Comp2, because a requirement in the module changed. `spex diff --json` against the snapshot reports R1 as `modified` beside the `meta` change of `alpha`, and its `errors` array holds that same single entry. The parity oracle in its accepting direction: the same description written by hand into a copy's `module.json` gives `spex diff --json` the same entry, so the command discovered nothing the hand edit would not have earned — it only printed it at the moment the edit was made.

### N17: A project requirement's field lands in project.json and the walk down is the obligation

**Given** the fixture, where P1 carries `priority` `1`, and a snapshot of it.
**When** `spex node set` is run with P1's id and `--field priority=2`.
**Then** exit 0. `project.json` carries `priority` `2` on P1 as a number, not a string; no `module.json` changed. `obligations` holds the walk the completeness rules make from a changed project requirement — through R1, which derives from P1, to one `incomplete_change` entry with `path` P1's id and `related` naming Comp1 — and `spex diff --json` against the snapshot reports P1 as `modified` with the same entry. `--unset priority` on P1 is refused with the tree unchanged, the error carrying the validator's `id` entry for a project requirement missing its priority, since the schema leaves the field optional and the presence check does not. A second run with `--field priority=five` is refused with the tree unchanged: the error document carries the validator's `schema` entry for P1's `priority`, its `fix` naming the field as an integer in the range `0` to `4`. A run over R1 with `--field type=optional` is refused likewise, the `fix` listing `functional` and `non_functional`. The parity oracle: the hand copy with `"priority": "five"` fails `spex validate` with the same `schema` entry.

### N18: Removing the pending mark once a module derives the requirement

**Given** the fixture, where P2 declares `derivation: pending` and `spex validate` printed the `pending_derivation` note for it until a prior `spex node add` declared a module requirement R2 in `alpha` with `preq_id` P2 and a prior `spex edge add` made Comp2 implement R2 — after which the note is already gone, since it is printed for an underived pending requirement only; a snapshot taken after those two commands.
**When** `spex node set` is run with P2's id and `--unset derivation`.
**Then** exit 0. P2's entry in `project.json` no longer carries `derivation` and every other field of it is unchanged; `obligations` is empty, because the default profile declares `derivation` unhashed and no requirement leaf moved. `spex validate` is green with no note. `spex diff --json` against the snapshot reports exactly one change, the `meta/project` envelope leaf, and `errors: []` — the byte change to `project.json` is recorded, and nothing behind it moved. A second `--unset derivation` on P2 exits 0, prints that the field is absent, and leaves the tree byte-identical.

## Edge cases

### N12: A cross-module target is refused on a module-local field

**Given** the fixture with a second module `beta` declaring component `Other`.
**When** `spex edge add` is run with source `alpha`'s api `demo run`, field `provided_by`, target `Other`.
**Then** non-zero exit and no file changed; the error is the validator's `provided_by is module-local` message, and the `fix` names the components of `alpha` as the permissible targets.

### N13: A hand-formatted file is reformatted, and no hash moves

**Given** the fixture with `alpha/module.json` rewritten by hand as one line with four-space indentation, and a snapshot taken after that rewrite.
**When** `spex edge add` is run with source Comp2, field `implements`, target R1 — an entry the field does not yet hold.
**Then** exit 0 and the file is now in canonical form — two-space indent, the profile's key order for node fields, arrays in declaration order. `spex diff --json` against the snapshot reports the `meta` change of `alpha` and its obligations and nothing else: the reformat moved no leaf hash, because leaves hash from field values, not bytes, and the same add over a copy left hand-formatted produces a byte-identical file.

### N14: The commands need no initialised project

**Given** the fixture with no `.spex/` directory, and a copy of it with an initialised, healthy `.spex/`.
**When** N1's `spex node add` is run over both.
**Then** both exit 0 with byte-identical trees under `spec/`, and the initialised copy's `.spex/` is byte-identical before and after.

### N15: A cardinality-one field is retargeted by one add, and a required one is never cleared

**Given** the fixture, where R1's `preq_id` holds P1, and a snapshot of it.
**When** `spex edge add` is run with source R1, field `preq_id`, target P2, and then `spex edge remove` with source R1, field `preq_id`, target P2.
**Then** the add exits 0 and R1's `preq_id` holds P2; the write report carries P1 under `replaced_target`, and its `obligations` carry the completeness entry for R1 — `requirement R1 (…) changed but component Comp1 content leaf unchanged`, since a module requirement's leaf hashes its `preq_id` — and the validator's `requirement_coverage` finding for P1, now derived by nothing: an obligation, not a refusal, because the validator accepts the hand copy to the same degree. `spex diff --json` reports R1 as `modified` alongside the `meta` change of `alpha`. The remove is refused with the tree unchanged: the error document carries the validator's `schema` entry for the missing required `preq_id`, its `fix` naming the field and its kind, and the `id` entry for the requirement missing its `preq_id`; the parity oracle holds — the same entry with `preq_id` deleted by hand fails `spex validate` with the same two `check` values and messages. A required field of cardinality one has no cleared state to pass through; it is retargeted by the add alone.

### N19: A required field is never unset

**Given** the fixture.
**When** `spex node set` is run with R1's id and `--unset type`.
**Then** non-zero exit and no file changed. The error document carries the validator's `schema` entry for the required `type` missing from R1, its `fix` naming the field and its kind — an enumerated text, `functional` or `non_functional`. The parity oracle: the same entry with `type` deleted by hand fails `spex validate` with the same `check` value and message. Unsetting the `preq_id` of R1 is refused too, but as a reference field, by N20's rule, before requiredness is ever considered.

### N20: Identity, derived and reference fields each name the command that owns them

**Given** the fixture.
**When** `spex node set` is run over Comp1 four times — `--field name=Core`, `--field id=000000000000`, `--field content=arch_other.md`, `--field implements=<R1's id>` — then over R1 with `--field preq_id=<P2's id>`, then over Comp1 with `--field colour=red`, then with `alpha`'s module id and `--field description=anything`.
**Then** every run exits with the contract-refusal code, the same code a reporter refusal takes, with no file changed, and each `fix` names the surface that owns the field: `spex node rename` for `name`; for `id` and `content`, that the field is derived and nothing sets it; `spex edge add` and `spex edge remove` for `implements` and for `preq_id`; the fields the profile declares on `component` for `colour`, as the validator's `schema` entry for an undeclared property carries them; and for the module id, the hand edit — the `project.json` entry and the `module.json` — exactly as `spex node remove` and `spex node rename` refuse a module id. The refusal for `colour` is the one N4's parity oracle would find in a hand copy carrying the field; the other refusals are ownership refusals, the one kind that is not the validator's predicate, and each is asserted on its exit code and its `fix`. A run over Comp1 with `--field description=x --unset description` is refused likewise, the `fix` saying the name was given to both.

### N21: The current value is a no-op, and the write is canonical

**Given** the fixture, and a snapshot of it; R1's description is D.
**When** `spex node set` is run with R1's id and `--field description=D`, the value the entry already holds.
**Then** exit 0, the report says the field already holds the value, `obligations` is empty, the tree is byte-identical and `spex diff --json` against the snapshot reports no change. Given instead the fixture with `alpha/module.json` rewritten by hand as one line with four-space indentation, as in N13, and a snapshot taken after that rewrite: `--field description=D2` exits 0, the file is now in canonical form, and `spex diff --json` reports R1 as `modified` with the one Comp1 obligation of N16 and nothing else — the reformat moved no hash, and the same set over a copy left hand-formatted produces a byte-identical file.
