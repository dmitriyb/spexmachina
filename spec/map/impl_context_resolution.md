# Context Resolution Implementation

## Algorithm

1. Use `record.SpecNodeID` directly as the component's identity hash — no parsing, no path splitting, no integer extraction
2. Read `<specDir>/<record.Module>/module.json` and unmarshal into a `ModuleSpec`
3. Scan `impl_sections`: for each section whose `describes` array contains the identity hash, resolve the content path as `<specDir>/<record.Module>/<section.Content>`
4. Scan `test_sections`: same filtering and path resolution logic
5. Scan `data_flows`: for each flow whose `uses` array contains the identity hash, resolve the content path
6. Set `ArchFile` to `record.ContentFile` (already an absolute or spec-relative path)
7. Set `ModuleFile` to `<specDir>/<record.Module>/module.json`

```go
func ResolveContext(specDir string, record Record) (ContextResult, error) {
    compHash := record.SpecNodeID // already the 12-char hex identity hash

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
        if containsHash(impl.Describes, compHash) {
            result.ImplFiles = append(result.ImplFiles,
                filepath.Join(specDir, record.Module, impl.Content))
        }
    }
    // Same pattern for TestSections and DataFlows
    return result, nil
}
```

`containsHash` is a string `slices.Contains` over the `[]string` `Describes` field — both sides are identity hashes, so the comparison is exact-match. There is no `parseComponentID` helper anymore; it would have nothing to parse.

## Error Handling

- Missing module.json: return error with path (the module was likely deleted or renamed)
- Identity hash present in `record.SpecNodeID` but not found in any component: not an error from the resolver's perspective — the result is just `ImplFiles: nil, TestFiles: nil, FlowFiles: nil`. The validator catches dangling references at spec-validation time, before any record can reach this code path.
- No matching impl/test/flow sections: not an error — the result simply has empty slices. A component may legitimately have no data_flows, for instance.
