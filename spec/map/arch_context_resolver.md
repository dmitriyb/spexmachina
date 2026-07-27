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

1. Treat `record.SpecNodeID` as the component's identity hash directly — no parsing, no path decomposition
2. Read `<specDir>/<record.Module>/module.json`
3. Scan `impl_sections`: if `describes` contains the identity hash, prepend `<specDir>/<module>/` to the section's `content` field
4. Scan `test_sections`: same logic
5. Scan `data_flows`: if `uses` contains the identity hash, same path resolution
6. `ArchFile` is `record.ContentFile` (already a full path)
7. `ModuleFile` is `<specDir>/<record.Module>/module.json`

## Design Notes

### Pure function

ResolveContext takes a spec directory and a record, reads files, and returns a result. No side effects, no state. This makes it testable and deterministic.

### Why a separate component?

Context resolution is reusable beyond the CLI — skills, review tooling and any future consumer need the same "give me everything about this component" capability. Keeping it out of MapCommand makes it callable as a library function.

### The result's shape follows the module, not the record

The record contributes an identity hash, a module and an arch path; every other path in the result is *discovered*, by scanning the module for declarations that name that hash. So the key set of a `ContextResult` is a function of which kinds of section `module.json` declares, not of anything stored on the record — one key per kind of section that can name a component, plus the arch leaf and the module file.

The practical consequence is that retiring a kind of section is a change to this component and to nothing downstream of it. One arm of the scan goes away and one key leaves the result; no record is touched, because no record ever pointed at a section — they point at components, and a component's arch leaf is the thing a record's `content_file` names. A resolver that had instead stored resolved section paths on the record would have needed a bead-map migration for the same change.

### Resolution reads the spec graph, not the tracker

`ResolveContext` takes a record and a spec directory, and that is the whole of its input. It never reads a bead, a changeset or a receipt.

The record contributes exactly three things: `record.Module`, which locates the module directory and is authoritative for the `module.json` path; `record.SpecNodeID`, matched against `describes` and `uses` arrays in that module.json; and `record.ContentFile`, returned as `ArchFile`. `record.Module` is the load-bearing one — every path in the result except `ArchFile` is joined under `<specDir>/<record.Module>/`, so a wrong module on the record misdirects the whole resolution rather than degrading it. Everything else in the result is derived from the spec graph on disk. The record's `bead_id`, `bead_type` and `spec_hash` are passed through in `Record` for the caller's benefit and are never consulted during resolution.

That is why the result is stable against tracker churn: a bead can be closed, replaced by a modify-pair, or re-created under a new id, and `spex map context` returns the same files — because the record id survives the pair and the identity hash survives the rename. A resolver that consulted bead state would return different context depending on when it was asked, and the skills that consume it would lose the determinism they rely on.
