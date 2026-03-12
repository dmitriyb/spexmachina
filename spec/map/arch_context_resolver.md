# ContextResolver

Given a mapping record, resolves all spec files needed to implement or review the mapped component.

## Responsibilities

- Read the module.json for the record's module
- Find impl_sections whose `describes` array contains the component ID
- Find test_sections whose `describes` array contains the component ID
- Find data_flows whose `uses` array contains the component ID
- Return all resolved file paths as a structured result

## Interface

```go
type ContextResult struct {
    Record     Record   `json:"record"`
    ArchFile   string   `json:"arch_file"`
    ImplFiles  []string `json:"impl_files"`
    TestFiles  []string `json:"test_files"`
    FlowFiles  []string `json:"flow_files"`
    ModuleFile string   `json:"module_file"`
}

func ResolveContext(specDir string, record Record) (ContextResult, error)
```

## Algorithm

1. Parse the component ID from `record.SpecNodeID` (e.g. `"schema/component/1"` → 1)
2. Read `<specDir>/<record.Module>/module.json`
3. Scan `impl_sections`: if `describes` contains the component ID, prepend `<specDir>/<module>/` to the section's `content` field
4. Scan `test_sections`: same logic
5. Scan `data_flows`: if `uses` contains the component ID, same path resolution
6. `ArchFile` is `record.ContentFile` (already a full path)
7. `ModuleFile` is `<specDir>/<record.Module>/module.json`

## Design Notes

### Pure function

ResolveContext takes a spec directory and a record, reads files, and returns a result. No side effects, no state. This makes it testable and deterministic.

### Why a separate component?

Context resolution is reusable beyond the CLI — Apply, skills, and other tools need the same "give me everything about this component" capability. Keeping it out of MapCommand makes it callable as a library function.
