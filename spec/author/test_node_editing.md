# Node editing tests

Integration and acceptance scenarios for NodeEditor, NodeRenamer and EdgeEditor as `spex node add`, `spex node remove`, `spex node rename`, `spex edge add` and `spex edge remove` drive them. Every scenario runs a command over a fixture tree, then reads the tree back through `spex validate` and `spex diff`, because the property under test is that a command's write is indistinguishable from a correct hand edit.

## Setup

A fixture spec under a temporary directory, built once per scenario from a committed copy:

```
tmp/spec/
  project.json          # two requirements; one module: alpha
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

The parity oracle shared by the refusal scenarios: the same change applied by hand to a copy of the fixture, then `spex validate` over the copy. The command's refusal and the validator's error must carry the same `check` value and the same message.

## Scenarios

### N1: Adding a component places it, derives its id and scaffolds its leaf

**Given** the fixture, and a snapshot of it.
**When** `spex node add` is run with type `component`, module `alpha` and name `Widget`.
**Then** exit 0. `alpha/module.json` carries a new entry in `components` whose `id` equals what `spex hash-id --type component --module alpha --name Widget` prints and whose `content` is `arch_widget.md`; that file exists and opens with the heading `# Widget`. `spex validate` is green. `spex diff --json` reports exactly one `added` change of node type `component` plus the `meta` change of module `alpha`, and the `errors` array contains the meta-obligation entries for Comp1 and Comp2 — the same entries the command printed under `obligations` on stdout.

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
**Then** exit 0. Comp1's entry and `arch_comp1.md` are gone; the same inbound references N6 listed are printed as dangling, and `spex validate` now reports each of them — `id` errors for the arrays, `link` errors for the leaves — which is exactly the list the command printed. `spex diff --json` reports one `removed` component and the `surviving_name` error for `Comp1`.

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
