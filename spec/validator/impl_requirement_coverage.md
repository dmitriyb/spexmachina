# Requirement Coverage Check Implementation

## Algorithm

1. Load all project requirements from `project.json`
2. Load all modules, their requirements, and their components
3. Build a set of project requirement IDs that are referenced by at least one module requirement's `preq_id`
4. For each project requirement not in the set → report uncovered
5. For each module, build a set of module requirement IDs that are referenced by at least one component's `implements`
6. For each module requirement not in the set → report uncovered

## Data structures

```go
// coveredProjectReqs: set of project requirement IDs with at least one preq_id reference
coveredProjectReqs := map[int]bool{}
for _, mod := range modules {
    for _, req := range mod.Requirements {
        coveredProjectReqs[req.PreqID] = true
    }
}

// Per-module: coveredModuleReqs: set of module requirement IDs with at least one implements reference
for _, mod := range modules {
    coveredModuleReqs := map[int]bool{}
    for _, comp := range mod.Components {
        for _, reqID := range comp.Implements {
            coveredModuleReqs[reqID] = true
        }
    }
    // Check each module requirement against the set
}
```

## Error format

Each uncovered requirement produces one structured error compatible with ErrorReporter:

```go
Error{
    Code:    "uncovered_project_requirement",
    Message: fmt.Sprintf("project requirement %d %q is not derived into any module requirement", req.ID, req.Title),
    Path:    "project.json",
}

Error{
    Code:    "uncovered_module_requirement",
    Message: fmt.Sprintf("%s requirement %d %q is not implemented by any component", moduleName, req.ID, req.Title),
    Path:    fmt.Sprintf("%s/module.json", modulePath),
}
```
