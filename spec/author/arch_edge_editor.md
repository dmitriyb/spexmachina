# EdgeEditor

The component behind `spex edge add` and `spex edge remove`: one entry of one reference field on one node, added or removed, with the checks that make the edit safe run before the file is touched. [[2b6317fa1307|Edit reference fields with graph checks]] is its contract.

## One entry, four checks

The caller names a source node, a field and a target node, all by id and field name. Before writing, the editor asks the resolved profile and the tree:

| Check | Refusal, with the fix it carries |
|---|---|
| the target exists | the array that was searched, by file and key |
| the source type declares the field, as a reference kind | the reference fields the profile declares on that type |
| the field permits the target's type | the target types the field permits |
| a module-local field stays module-local — `provided_by` under the default profile | the components of the source's own module |

Then the after-state goes through [[b9e7b96f6aa7|ObligationReporter]], which is where the fifth check lives: every cycle-checked field — `uses` and the frame's `requires_module` among them, any reference field the profile does not mark `cyclic` — must stay acyclic, and the refusal is the validator's `dag` entry naming the cycle. A field the profile marks `cyclic` is exempt there exactly as it is in `spex validate`. The editor holds no cycle logic of its own.

`requires_module` is the one field that is not a profile declaration: modules are frame nodes and the edge belongs to the frame. The editor carries it as the frame's own field on the module entry in `project.json`, target type `module`, cycle-checked, so a module dependency is added the same way a component dependency is.

## Idempotence

Adding an entry the field already holds changes nothing and says so. Removing one the field does not hold changes nothing and says so. A field with cardinality one — `preq_id` — holds one target: add sets it, and adding a different target replaces the one held, the write report carrying the replaced target under `replaced_target` so that the retarget is visible rather than silent. Remove clears it, and for a required field that is a refusal carrying the validator's `schema` and `id` entries — the same two a hand edit of the absent field earns from `spex validate` — so a required cardinality-one field is retargeted by one add, never through a cleared state it cannot reach.

## Obligations

An edge is a `module.json` or `project.json` change, so it moves a `meta` leaf; the report lists what the completeness rules now oblige — every component in the module unless a requirement in it also changed in the same diff, and, for a component whose own edges changed, its own leaf. A leaf the edit obliges also owes a link: a component leaf carries one typed link per entry in its `uses`, its `implements`, and every api whose `provided_by` names it, so an added edge is usually followed by a line of prose in the leaf it obliges.
