# ID and Reference Validation Implementation

All IDs are 12-character hex identity hash strings. Every check below is implemented with `map[string]bool` set membership — never integer arithmetic, never path parsing.

## ID Uniqueness

For each array (requirements, components, impl_sections, data_flows, test_sections, modules, milestones, sections, test_plan scenarios):

```go
seen := make(map[string]bool)
for _, node := range arr {
    if seen[node.ID] {
        errors = append(errors, dupErr(arr, node.ID))
        continue
    }
    seen[node.ID] = true
}
```

A collision in the 48-bit hash space across distinct logical nodes is mathematically improbable, but a duplicate can still appear if a file was hand-edited or merged badly, so the check is enforced.

## Cross-Reference Validation

Build identity-hash sets for each type, then check all references with set membership:

```
projectReqHashes  = {req.ID for req in project.requirements}
moduleHashes      = {mod.ID for mod in project.modules}

For each module:
  reqHashes   = {req.ID for req in module.requirements}
  compHashes  = {comp.ID for comp in module.components}

  Check: comp.implements ⊆ reqHashes
  Check: comp.uses ⊆ compHashes
  Check: impl.describes ⊆ compHashes
  Check: flow.uses ⊆ compHashes
  Check: test.describes ⊆ compHashes
  Check: req.depends_on ⊆ reqHashes
  Check: req.preq_id ∈ projectReqHashes (mandatory — see below)

Check: mod.requires_module ⊆ moduleHashes
Check: milestone.groups ⊆ moduleHashes
Check: scenario.modules ⊆ moduleHashes
```

Each failed check produces an error with the source node, the reference field, and the dangling target identity hash.

## Mandatory preq_id Check

Every module requirement must have a non-empty `preq_id` field whose value is the identity hash of an existing project requirement:

```
For each module:
  For each requirement in module.requirements:
    If req.PreqID == "":
      Error: "module %s: requirement %s missing preq_id"
    Else if !projectReqHashes[req.PreqID]:
      Error: "module %s: requirement %s preq_id %s not found in project requirements"
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
