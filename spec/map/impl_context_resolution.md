# Context Resolution Implementation

## Algorithm

1. Extract the component ID from `record.SpecNodeID` by parsing the trailing integer from the `<module>/<type>/<id>` format
2. Read `<specDir>/<record.Module>/module.json` and unmarshal into a `ModuleSpec`
3. Scan `impl_sections`: for each section whose `describes` array contains the component ID, resolve the content path as `<specDir>/<record.Module>/<section.Content>`
4. Scan `test_sections`: same filtering and path resolution logic
5. Scan `data_flows`: for each flow whose `uses` array contains the component ID, resolve the content path
6. Set `ArchFile` to `record.ContentFile` (already an absolute or spec-relative path)
7. Set `ModuleFile` to `<specDir>/<record.Module>/module.json`

```go
func ResolveContext(specDir string, record Record) (ContextResult, error) {
    compID, err := parseComponentID(record.SpecNodeID)
    if err != nil {
        return ContextResult{}, fmt.Errorf("parse spec_node_id %q: %w", record.SpecNodeID, err)
    }

    modPath := filepath.Join(specDir, record.Module, "module.json")
    mod, err := loadModule(modPath)
    if err != nil {
        return ContextResult{}, fmt.Errorf("load module %s: %w", record.Module, err)
    }

    result := ContextResult{
        Record:     record,
        ArchFile:   record.ContentFile,
        ModuleFile: modPath,
    }

    for _, impl := range mod.ImplSections {
        if containsID(impl.Describes, compID) {
            result.ImplFiles = append(result.ImplFiles,
                filepath.Join(specDir, record.Module, impl.Content))
        }
    }
    // Same pattern for TestSections and DataFlows
    return result, nil
}
```

## Spec Node ID Parsing

The `spec_node_id` format is `<module>/<type>/<id>`, e.g. `"schema/component/1"`. The parser splits on `/` and converts the last segment to an integer. The module segment is cross-checked against `record.Module` for consistency.

## Error Handling

- Missing module.json: return error with path (the module was likely deleted or renamed)
- Invalid spec_node_id format: return error with the raw value
- No matching impl/test/flow sections: not an error — the result simply has empty slices. A component may legitimately have no data_flows, for instance.
