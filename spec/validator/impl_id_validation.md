# ID and Reference Validation Implementation

## ID Uniqueness

For each array (requirements, components, impl_sections, data_flows, modules, milestones):
1. Build a map of ID → count
2. Any ID with count > 1 is a duplicate — emit error with the array location and ID

## Cross-Reference Validation

Build ID sets for each type, then check all references:

```
projectReqIDs  = {id for req in project.requirements}
moduleIDs      = {id for mod in project.modules}

For each module:
  reqIDs   = {id for req in module.requirements}
  compIDs  = {id for comp in module.components}

  Check: comp.implements ⊆ reqIDs
  Check: comp.uses ⊆ compIDs
  Check: impl.describes ⊆ compIDs
  Check: flow.uses ⊆ compIDs
  Check: req.depends_on ⊆ reqIDs
  Check: req.preq_id ∈ projectReqIDs (if set)

Check: mod.requires_module ⊆ moduleIDs
Check: milestone.groups ⊆ moduleIDs
```

Each failed check produces an error with the source node, the reference field, and the dangling target ID.

## Ordering

Run ID uniqueness first — if IDs are duplicated, cross-reference checks may be misleading (which duplicate does the reference point to?).
