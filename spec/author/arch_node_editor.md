# NodeEditor

The component behind `spex node add` and `spex node remove`: it declares a node of any type the resolved profile knows and retires one, and it is the one place a new node's array, id and content path are decided. [[fe62312f507b|Add and remove nodes by profile]] is its contract.

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

## Removing a node

Removal deletes the node's entry and its content file, then sweeps the tree for what still names it: every reference field entry carrying the id, in every `project.json` and `module.json`, and every typed link naming the id in every content leaf, by file and line. While any remains, the removal is refused and the list is the error, each inbound reference paired with the `spex edge remove` invocation that would retarget it. `--force` performs the removal anyway and prints the same list as what it left dangling — the operator has said the retargeting is theirs to do, and `spex validate` will hold the tree red until they do it.

A module id is refused, forced or not. Modules are frame nodes: retiring one means the `project.json` entry, the directory and every `requires_module` edge naming it, and the fix names those three as the hand edit to make.

A removal of a name-declarable node — a component or an api under the default profile — is also a removal for the vocabulary sweep's purposes: the retired name still appears in the corpus wherever prose mentioned it, and `spex diff` reports each site as a `surviving_name` error until the mentions are rewritten. The editor prints the retired name so the sweep starts from the command's output rather than from memory.

## Formatting

A file the editor writes is written in one canonical form: two-space indent, the profile's key order for node fields, arrays in declaration order. A hand-formatted file is reformatted on its first write. Hashes and merkle leaves are computed from field values, not bytes, so the reformat moves no hash and `spex diff` reports nothing for it. Same tree, same command, same arguments — same files.

## Boundaries

Nothing under `.spex/` is read or written, and no initialised project is needed: a spec is authored, migrated and validated before it is ever initialised. Nothing outside `spec/` is touched. No subprocess runs.
