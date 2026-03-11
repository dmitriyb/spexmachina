# Test Coverage Check Implementation

## Algorithm

```
for each module in project.modules:
    module_json = read(specDir / module.path / "module.json")
    component_ids = {c.id for c in module_json.components}
    covered_ids = {}
    for ts in module_json.test_sections:
        covered_ids = covered_ids ∪ set(ts.describes)
    uncovered = component_ids - covered_ids
    for id in uncovered:
        emit ValidationError{
            module: module.name,
            component: lookup(id).name,
            message: "component has no test_section coverage"
        }
```

## Integration with ValidateCommand

TestCoverageChecker plugs into the existing validation pipeline alongside SchemaChecker, ContentResolver, DAGChecker, OrphanDetector, IDValidator, and NameConsistencyChecker. It runs after SchemaChecker (needs valid JSON) but has no ordering dependency on other checkers.

All errors are collected and passed to ErrorReporter for aggregated output.

## Relationship to OrphanDetector

OrphanDetector checks that every requirement has an implementing component and every component has a describing impl_section. TestCoverageChecker adds a parallel check: every component must also have a describing test_section. These are independent checks — a component can pass orphan detection (has an impl_section) but fail test coverage (no test_section).
