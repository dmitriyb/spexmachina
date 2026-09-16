# NodeEditor

The component behind `spex node add`, `spex node set` and `spex node remove`: it declares a node of any type the resolved profile knows, changes a declared value on one that exists, and retires one, and it is the one place a new node's array, id and content path are decided. [[fe62312f507b|Add and remove nodes by profile]] is its contract for the first and the last; [[6b69d1984dbe|Set declared field values]] for the edit in between.

## Adding a node

The caller names a type, a name, a module where the type is module-scoped, and one value per declared field. The editor then decides everything structural from the profile and the identity contract, so that nothing about the placement is the caller's to get wrong:

| Decision | Source |
|---|---|
| which file — `project.json` or `<module>/module.json` | the type's declared scope; a type declared at both scopes lands by whether a module was named |
| which array | the type's declared plural key |
| the `id` | the identity hash of scope, type name and name — what `spex hash-id` prints for the same three |
| which fields are required, and what kind each is | the type's field declarations; a required field absent is a refusal naming it and its kind |
| the `content` path | the type's declared content prefix joined to a snake-case slug of the name, for a content-bearing type only |
| the leaf on disk | scaffolded through [[b1a81efbd240|LeafScaffolder]] from the same profile, for a content-bearing type only |

The editor names no type, key, field or prefix itself. A type the profile does not declare is refused with the declared list as the fix; a field the type does not declare is refused with the type's fields as the fix. That indifference is the whole design: a project whose profile declares `endpoint` and `resource` runs the same command with the same behaviour, and the golden test over a second, test-only profile is what holds it to that.

`--type module` is the one addition outside the profile's types, because modules are frame nodes, not declared ones. It appends the entry to `project.json`'s `modules` — name, path equal to the name, the derived module id — and writes a `module.json` skeleton declaring the name and nothing else, so the module is visible to every gate from its first moment: a directory `project.json` does not name is invisible to `spex validate`, `spex diff` and `spex render` alike, and creating the two together is what closes that hole.

The write goes through [[b9e7b96f6aa7|ObligationReporter]] first: the after-state is checked by the validator's own predicate, a refusal names its fix, and a write reports the leaves it obliges — which, for a new node in an existing module, is every component in that module, since the module's `meta` leaf moved and no requirement did.

## Setting a field

The caller names an id and one or more declared fields with a value each, one or more fields to unset, or both in one invocation — a name given to both is refused. The editor finds the entry at either scope, in whichever file holds it, and replaces the values in place: nothing else on the entry moves, and the id, which is the name's hash, cannot. What it accepts is decided by the profile the same way adding is: a field the node's type declares as text, integer or enumeration, and the envelope's `description`. A value is converted by kind exactly as `spex node add` converts one, so an integer field refuses a non-integer and an enumerated field refuses a value outside its enumeration, each with the validator's own `schema` entry and the declared kind or enumeration as the fix. `--unset` removes an optional field; on a field the validator requires, the refusal is the entry a hand edit deleting it would earn — the `schema` entry for a field the type declares required, the `id` entry for a project requirement's `priority`, which the schema leaves optional and the validator's presence check does not.

Every field this command will not touch has a surface that owns it, and the refusal names that surface, with the contract-refusal exit code and the tree untouched:

| Field | Refused with |
|---|---|
| `name` | `spex node rename` — the name is the identity, and a rename moves the id, every reference and the leaf with it |
| `id`, `content` | that both are derived — from the name and from the type's content prefix — and nothing sets them |
| a reference field — `implements`, `uses`, `describes`, `provided_by`, `preq_id`, `depends_on`, or any the profile declares | `spex edge add` and `spex edge remove` |
| a field the type does not declare | the fields it does declare |
| a module id | the hand edit, as removing and renaming refuse a module id too |

One field, one write path. That is why the reference fields are refused here rather than accepted as a convenience: two commands able to write the same array would let it be edited without the graph checks the edge commands apply. It is also why `spex node add` was not stretched into an upsert — add creates and refuses a duplicate, and a command that either creates or changes depending on what the tree already holds answers "what did this run do" with the tree's prior state rather than with its name.

Setting a field to the value it already holds changes nothing and says so; unsetting a field that is absent does the same. A real change is written through [[b9e7b96f6aa7|ObligationReporter]] like every other write, and the report carries what the moved hash obliges under the completeness rules: a module requirement's description reaches every implementing component's leaf; a project requirement's walks down through every deriving module requirement to every implementing component; a field the profile declares unhashed — `derivation` under the default profile — moves no requirement leaf and obliges nothing, though the project envelope's own `meta` leaf still records the byte change, inert as it is everywhere else. This is the obligation that, before the command existed, a hand edit of the same entry left for `spex diff` to find at the end of the session, and printing it at the moment the edit is made is most of what the command is for.

## Removing a node

Removal deletes the node's entry and its content file, then sweeps the tree for what still names it: every reference field entry carrying the id, in every `project.json` and `module.json`, and every typed link naming the id in every content leaf, by file and line. While any remains, the removal is refused and the list is the error, each inbound reference paired with the `spex edge remove` invocation that would retarget it. `--force` performs the removal anyway and prints the same list as what it left dangling — the operator has said the retargeting is theirs to do, and `spex validate` will hold the tree red until they do it.

A module id is refused, forced or not. Modules are frame nodes: retiring one means the `project.json` entry, the directory and every `requires_module` edge naming it, and the fix names those three as the hand edit to make.

A removal of a name-declarable node — a component or an api under the default profile — is also a removal for the vocabulary sweep's purposes: the retired name still appears in the corpus wherever prose mentioned it, and `spex diff` reports each site as a `surviving_name` error until the mentions are rewritten. The editor prints the retired name so the sweep starts from the command's output rather than from memory.

## Formatting

A file the editor writes is written in one canonical form: two-space indent, the profile's key order for node fields, arrays in declaration order. A hand-formatted file is reformatted on its first write. Hashes and merkle leaves are computed from field values, not bytes, so the reformat moves no hash and `spex diff` reports nothing for it. Same tree, same command, same arguments — same files.

## Boundaries

Nothing under `.spex/` is read or written, and no initialised project is needed: a spec is authored, migrated and validated before it is ever initialised. Nothing outside `spec/` is touched. No subprocess runs.
