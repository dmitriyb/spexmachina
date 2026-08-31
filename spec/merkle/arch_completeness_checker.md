# CompletenessChecker

[[6f8284df92a2|Validates that a changed requirement leaf brought its
implementing components' content leaves with it]]. Ensures that spec edits are
complete before the plan pipeline processes them.

Findings from CompletenessChecker are **errors**, not warnings. They land in
the top-level `errors` array of the diff JSON output and DiffCommand
propagates them as a non-zero exit code. The terminology is consistent end to
end: the JSON field is `errors`, and any text-output rendering must label each
line with `error:` (never `warning:`). The
distinction matters because the downstream pipeline step (`spex plan`)
refuses to consume a diff with a non-empty `errors` array — a "warning"
label suggests advisory output that can be ignored, which is the opposite of
the contract.

## Responsibilities

- Read the current spec graph (project.json, module.json) to resolve which components implement which requirements
- For each requirement leaf change in the diff, check whether the implementing components' content leaves also changed
- For meta-only changes (no requirement leaf changes), check whether component content leaves changed
- Report errors for incomplete edits

## Checks

All checks operate on identity hashes. The diff change keys, the `implements` arrays in `module.json`, and the keys in the resolved component map are all hex strings — comparison is exact-match.

Which node types trigger the rules is the resolved profile's declaration, not a compiled-in pair: the requirement-leaf checks fire on the type the profile assigns that role, and the meta-envelope sweep on the fixed `meta` leaves — a rule of the frame, not a profile assignment. The implementing edge the checks resolve through (the `implements` arrays) and the component shape they sweep are this checker's own reading of `module.json`, not profile declarations. The default profile declares exactly the triggers described below, so every rule reads under it as it always has.

### Modified requirement → implementing component content must change

When a requirement leaf changed (a change whose key is the requirement's identity hash and whose node type is `requirement`), resolve which components implement that requirement by scanning every component's `implements` array for that hash. For each such component, its content leaf must also appear as a change in the diff (looked up by the component's identity hash).

For each component whose content leaf did NOT change, report an error:

```json
{
  "type": "incomplete_change",
  "message": "requirement 'Match changed nodes to beads' (plan) description changed but component NodeMatcher content leaf unchanged",
  "path": "7c5e2fa1b3d8",
  "related": ["a1b2c3d4e5f6"]
}
```

The `path` and `related` fields carry identity hashes — the same values used everywhere else in the pipeline.

### Added requirement → must be implemented and component content must change

When a requirement leaf is added, check that the requirement is implemented by at least one component (i.e., its identity hash appears in some `implements` array) and that those components' content leaves also changed. If no component implements it, report an error. For each implementing component whose content leaf did NOT change, report an error.

### Removed requirement → no component may still reference it

When a requirement leaf is removed, check that no component in the current module.json still has the removed requirement's identity hash in its `implements` array. For each component that still references it, report an error.

### Project-level requirement changed → chain must propagate

When a project-level requirement leaf changed, find all module requirements with `preq_id` equal to the project requirement's identity hash. If none exist, report an error. For each such module requirement, find all implementing components. For each component whose content leaf did NOT change, report an error. An added project-level requirement takes the same path. A removed one inverts it: nothing deriving is the clean outcome, and each module requirement that still names the removed hash as its `preq_id` is reported instead — the complaint names that module requirement, not a component.

### Component edges changed → component content must change

When `meta/<module-hash>` is modified but no requirement leaves in that module changed, the meta change is due to non-requirement modifications (component edges, module description, etc.). For each component in the current module.json, check whether its content leaf also changed (lookup by the component's identity hash). For each component whose content leaf did NOT change, report an error.

This is the widest obligation in the checker — one changed byte in a `module.json` obliges every component in that module — and the requirement-leaf condition is the only thing that narrows it: if any requirement in the same module also changed, the module is left to the per-requirement checks above and no whole-module sweep runs.

The project envelope carries no such obligation at all. Only meta changes that name a module are collected here, and `meta/project` names none, so editing `project.json` without touching a requirement leaf produces nothing from this check. What stands over `project.json` is the project-level requirement chain above, not a sweep of every component in the corpus.

## Interface

One call, over three inputs — the classified changes, the path to the spec directory, and the resolved profile whose declarations carry the per-type triggers — returning the errors it found, and an empty result when every change is complete. The profile is handed in by the caller: `spex diff` resolves it once per invocation and this checker, like the classifier, consults no profile file of its own.

It runs over the classified list but reads none of the classification. What it takes from each entry is four of the fields [[cb262b280963|DiffEngine]] sets — the key, the kind of change, the node type and the owning module's identity hash — and none of what [[f1a672216ce9|ImpactClassifier]] adds: the impact level goes unread, and the human-readable module name is passed over in favour of the identity hash underneath it, because the hash is what `module.json` can be looked up by. The two hash fields DiffEngine also sets, the old and the new content hash, are never read here either: that a leaf changed is the whole of what this check needs from it.

The spec it resolves against is the current one on disk, never the snapshot. The diff says what moved; `project.json` and the `module.json` files say who implements what. Two consequences follow from that split. A diff carrying no requirement leaf change and no module envelope change is answered without opening the spec at all. And a spec it cannot read yields no errors rather than a failure — an unreadable `project.json`, or a `module.json` that will not parse, is skipped and the changes touching it go unchecked, so this check reports nothing on a corpus that `spex validate` would already be rejecting.
