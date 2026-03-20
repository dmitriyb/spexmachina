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
  Check: test.describes ⊆ compIDs
  Check: req.depends_on ⊆ reqIDs
  Check: req.preq_id ∈ projectReqIDs (mandatory — see below)

Check: mod.requires_module ⊆ moduleIDs
Check: milestone.groups ⊆ moduleIDs
```

Each failed check produces an error with the source node, the reference field, and the dangling target ID.

## Mandatory preq_id Check

Every module requirement must have a `preq_id` field, and it must reference an existing project requirement ID:

```
For each module:
  For each requirement in module.requirements:
    If req.preq_id == 0 (unset):
      Error: "module %s: requirement %d missing preq_id"
    Else if req.preq_id ∉ projectReqIDs:
      Error: "module %s: requirement %d preq_id %d not found in project requirements"
```

This enforces that no orphan module requirements exist — every module requirement must derive from a project goal.

## Priority Presence Check

Every project requirement must have a `priority` field (integer 0-4):

```
For each requirement in project.requirements:
  If req.priority is not set:
    Error: "project requirement %d missing priority"
  Else if req.priority < 0 or req.priority > 4:
    Error: "project requirement %d priority %d out of range (must be 0-4)"
```

Migration note: existing project requirements without a `priority` field will fail validation. Users must add priority values manually. Default recommendation: P1 (high).

## Ordering

Run ID uniqueness first — if IDs are duplicated, cross-reference checks may be misleading (which duplicate does the reference point to?).
