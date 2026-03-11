# Preflight Algorithm

## Algorithm

```
function Check(beadID):
    record = store.GetByBead(beadID)
    if not found:
        return error("no mapping record for bead")

    // 1. Staleness check
    currentHash = merkle.HashNode(record.SpecNodeID)
    if currentHash != record.SpecHash:
        return PreflightResult{Status: "stale", StaleHash: currentHash}

    // 2. Module-level dependency check
    module = spec.GetModule(record.Module)
    blockers = []

    visited = set()
    checkModuleDeps(module, visited, &blockers)

    // 3. Component-level uses check
    if record is a component:
        component = spec.GetComponent(record.Module, record.SpecNodeID)
        for usedID in component.Uses:
            checkComponentReady(record.Module, usedID, &blockers)

    if len(blockers) > 0:
        return PreflightResult{Status: "blocked", Blockers: blockers}

    return PreflightResult{Status: "ready", Record: record}
```

## Module Dependency Walk

```
function checkModuleDeps(module, visited, blockers):
    if module.ID in visited:
        return error("cycle detected")
    visited.add(module.ID)

    for depID in module.RequiresModule:
        depModule = spec.GetModuleByID(depID)
        // Recurse into transitive deps first
        checkModuleDeps(depModule, visited, blockers)
        // Check all components in the dep module have closed beads
        for component in depModule.Components:
            specNodeID = fmt.Sprintf("%s/component/%d", depModule.Name, component.ID)
            record = store.GetBySpecNode(specNodeID)
            if not found or record.BeadStatus != "closed":
                blockers.append(Blocker{
                    SpecNodeID: specNodeID,
                    BeadID: record.BeadID,
                    Reason: "dependency not implemented",
                })
```

## Component Uses Check

For component-level `uses` edges within the same module, check that the used component's bead is closed. This ensures implementation dependencies are satisfied before starting work.

## Staleness Detection

Compare the spec_hash stored in the mapping record against the current merkle hash of the spec node. If they differ, the spec has been modified since the bead was created — the implementer should review the updated spec before starting work.

## Performance

The algorithm reads the mapping file once and the spec files once, then does in-memory graph traversal. No subprocess calls. Typical execution time is under 10ms for a spec with 100 modules.
