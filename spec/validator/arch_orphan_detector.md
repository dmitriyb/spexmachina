# OrphanDetector

Finds spec nodes that are not referenced by any other node, indicating incomplete spec coverage.

## Responsibilities

- Find requirements not implemented by any component (`implements` edge)
- Find components not described by any impl_section (`describes` edge)
- Report orphans with their module context

## Interface

```go
func CheckOrphans(modules map[string]*schema.Module) []ValidationError
```

## Behavior

For each module:

1. Build a set of all requirement IDs
2. Build a set of all requirement IDs referenced by any component's `implements` array
3. Orphan requirements = set difference (all - referenced)
4. Build a set of all component IDs
5. Build a set of all component IDs referenced by any impl_section's `describes` array
6. Orphan components = set difference (all - referenced)

## Severity

Orphan detection errors are warnings, not hard failures. A spec in progress may have requirements not yet assigned to components. The validator reports them but the exit code policy (warning vs error) is configurable.
