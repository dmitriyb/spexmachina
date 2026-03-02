# ImpactClassifier

Classifies diff changes by their impact level.

## Responsibilities

- Analyze each change's path to determine the spec layer affected
- Classify into three impact levels
- Attach impact classification to each change for downstream consumption

## Impact Levels

| Level | Condition | Meaning |
|-------|-----------|---------|
| `impl_only` | Only `impl_*.md` or `flow_*.md` leaves changed within a module | Implementation detail changed, architecture stable |
| `arch_impl` | Any `arch_*.md` leaf changed (with or without impl changes) | Architecture changed, dependent modules may be affected |
| `structural` | `module.json` or `project.json` changed | Spec structure changed — new/removed nodes, changed edges |

## Interface

```go
type ClassifiedChange struct {
    Change
    Impact string // "impl_only", "arch_impl", "structural"
    Module string // which module is affected (empty for project-level)
}

func Classify(changes []Change) []ClassifiedChange
```

## Rules

1. If a change path starts with a module path and the filename matches `impl_*.md` or `flow_*.md` → `impl_only`
2. If a change path starts with a module path and the filename matches `arch_*.md` → `arch_impl`
3. If a change path ends with `module.json` or `project.json` → `structural`
4. If a module has changes at multiple levels, the highest level wins (structural > arch_impl > impl_only)
