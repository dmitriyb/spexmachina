# Action Classification Rules

## Node-Type Gate (runs before the state transition table)

Each change first passes through a type gate. Gates produce no actions for nodes that do not correspond to beads.

```
def produces_bead(change, module_spec):
    if change.NodeType == "component":
        return True
    if change.NodeType == "data_flow":
        return True
    if change.NodeType == "test_section":
        ts = module_spec.find_test_section(change.SpecNodeID)
        return len(ts.describes) >= 2
    if change.NodeType == "impl_section":
        return False
    # structural types (meta, requirement) are filtered earlier by NodeMatcher
    return False
```

The `len(describes) >= 2` rule for test_sections: when a test_section describes only one component, it is a unit/component test naturally coupled with that component's TDD workflow — the implement skill reads the test_section's content as part of the component feature bead's work, so producing a separate task bead would create a redundant hand-off. When it describes two or more components, it is a cross-component integration test that cannot be bundled into any single component bead.

The rule reads `describes` from the current module.json (post-change). For `removed` test_sections, the describes array is read from the previous snapshot's spec graph; if it has already been lost, the classifier falls back to assuming a bead may exist and defers the decision to mapping lookup.

## State Transition Table

| Change Type | Has Matching Bead? | Action |
|-------------|-------------------|--------|
| added | no | create |
| added | yes (unexpected) | obsolete old + create new |
| modified | no | create (spec changed, no tracking bead) |
| modified | yes | obsolete old + create new |
| removed | no | no action (nothing to obsolete) |
| removed | yes, open/in_progress | obsolete |
| removed | yes, closed | obsolete + create (cleanup) |

The "review" action from the previous model is eliminated entirely. Modified nodes always trigger obsolete+create — no in-place metadata patching.

## OldBeadID Propagation

When a modified or unexpectedly-matched-added node generates both an obsolete and a create action, the create action carries `OldBeadID` set to the obsoleted bead's ID. Emit reads it twice: IdempotencyLabeler uses it to look the existing record id back up (`MappingStore.GetByBead`) so the modify pair reuses one record, and ChangesetBuilder turns it into a `{"ref":"bead","bead_id":"<old-bead-id>","type":"blocks"}` dep on the create op. The adapter renders that dep as `--deps blocks:<old-bead-id>`, which is where lineage tracking actually lands in the tracker.

## Reason Generation

Each action includes a human-readable reason:
- create (new): `"New spec node: {module}/{node_name}"`
- create (modified): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`

## Cleanup Classification

When a spec node is removed and its bead is closed, the code has already shipped to main. This means there is code in the repository that no longer corresponds to any spec node — it needs to be deleted. The classifier generates an additional "create" action for a cleanup bead.

When the bead is open or in_progress, no code has shipped to main, so only the obsolete action is needed — there is nothing to clean up.
