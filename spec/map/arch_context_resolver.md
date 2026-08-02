# ContextResolver

Given a [[205e67ca4aad|mapping record]], resolves all spec files needed to implement or review the
mapped component. That resolution is what it means to hold
[[40a3d3155131|the full spec context of a record]]: the record names a module, an identity hash and
an arch path, the module file follows from the module name, and the test and flow paths are
discovered by scanning that module's declarations for the hash.

## Responsibilities

- Read the module.json for the record's module
- Find test_sections whose `describes` array contains the component ID
- Find data_flows whose `uses` array contains the component ID
- Return all resolved file paths as a structured result

## Interface

Resolution takes two things — a spec directory and one mapping record — and produces one result: the
record handed back unchanged, the arch file, the test files, the flow files, and the module file.
[[3c8a43221ed2|`spex map context`]] prints that result, so the keys a caller
parses (`record`, `arch_file`, `test_files`, `flow_files`, `module_file`) are the result under
another name.

## Algorithm

1. Treat the record's `spec_node_id` as the component's identity hash directly — no parsing, no path decomposition
2. Read `<spec dir>/<record module>/module.json`
3. Scan `test_sections`: if `describes` contains the identity hash, prepend `<spec dir>/<module>/` to the section's `content` field
4. Scan `data_flows`: if `uses` contains the identity hash, same path resolution
5. The arch file is the record's `content_file`, returned exactly as the record stores it
6. The module file is `<spec dir>/<record module>/module.json`

## Error Handling

- A module.json that cannot be read, or cannot be parsed as JSON, is an error, and the error names the path that was tried. The usual cause is a record whose `module` no longer exists under that name.
- An identity hash the module declares nowhere is not an error. The result still carries the arch file and the module file; the discovered lists simply come back with nothing in them, because a component may legitimately have no flows and no tests.

## Design Notes

### Pure function

Resolution takes a spec directory and a record, reads files, and returns a result. No side effects, no state. This makes it testable and deterministic.

### Why a separate component?

Context resolution is reusable beyond the CLI — skills, review tooling and any future consumer need the same "give me everything about this component" capability. Keeping it out of MapCommand makes it callable as a library function.

### The result's shape follows the module, not the record

The record contributes an identity hash, a module and an arch path; every other path in the result is *discovered*, by scanning the module for declarations that name that hash. So the key set of the result is a function of which kinds of section `module.json` declares, not of anything stored on the record — one key per kind of section that can name a component, plus the arch leaf and the module file.

The practical consequence is that retiring a kind of section is a change to this component and to nothing downstream of it. One arm of the scan goes away and one key leaves the result; no record is touched, because no record ever pointed at a section — they point at components, and a component's arch leaf is the thing a record's `content_file` names. A resolver that had instead stored resolved section paths on the record would have needed a bead-map migration for the same change.

### Resolution reads the spec graph, not the tracker

A record and a spec directory are the whole of the input. Resolution never reads a bead, a changeset or a receipt.

The record contributes exactly three things: `module`, which locates the module directory and is authoritative for the `module.json` path; `spec_node_id`, matched against `describes` and `uses` arrays in that module.json; and `content_file`, returned as `arch_file`. `module` is the load-bearing one — every path in the result except `arch_file` is joined under `<spec dir>/<module>/`, so a wrong module on the record misdirects the whole resolution rather than degrading it. Everything else in the result is derived from the spec graph on disk. The record's `bead_id`, `bead_type` and `spec_hash` are echoed back for the caller's benefit and are never consulted during resolution.

That is why the result is stable against tracker churn: a bead can be closed, replaced by a modify-pair, or re-created under a new id, and `spex map context` returns the same files — because the record id survives the pair and the identity hash survives the rename. A resolver that consulted bead state would return different context depending on when it was asked, and the skills that consume it would lose the determinism they rely on.
