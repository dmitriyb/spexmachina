# Change Proposal: Mapping Layer

## Context

Manual testing of `spex` revealed two architectural gaps that block self-hosting:

1. **Fragile traceability.** Spec metadata lives in bead labels (`spec_module:`, `spec_component:`), coupling spex to `br`'s label format. The `/implement` skill parses these labels to do preflight checks non-deterministically. Any label format change in `br` or naming convention change in the spec breaks the pipeline.

2. **Path-based diffing.** The merkle module keys tree nodes by file path, not by spec ID. Renaming a module directory or content file produces a remove + add instead of a single modification. Since every spec node already has a stable numeric ID, the diff should compare by ID — making renames a non-issue.

3. **No spec↔bead bridge.** Spec and beads are independent worlds. There is no spex-owned data structure that records which spec node maps to which bead. Without this, preflight checks, mapping queries, and skill workflows all require reverse-engineering bead metadata.

## Proposed change

### Merkle: ID-based tree keying

Change the merkle module to key tree nodes by spec ID instead of file path.

**What changes:**
- `tree_builder.go`: Nodes carry the spec ID (e.g. module ID, component ID) as their key, not the file path.
- `snapshot_store.go`: Snapshots are keyed by ID. The flat map becomes `id → {hash, type, ...}` instead of `path → {hash, type, ...}`.
- `diff_engine.go`: Diff compares IDs between current tree and snapshot. Same ID + different hash = modified. ID in current but not snapshot = added. ID in snapshot but not current = removed.
- `impact_classifier.go`: Classification uses node metadata (type, module association) instead of parsing path prefixes.

**What this fixes:**
- Renaming a module directory or content file with the same ID is detected as a modification, not a delete + add.
- The diff is anchored to stable identifiers, not filesystem layout.

### New module: Map

Owns the spec↔bead correlation layer. The mapping file is the single source of truth — spec stays pure, beads stay generic, spex owns the bridge.

**Mapping file** (`spec/.bead-map.json`):
- Each record links a spec node ID to a bead ID, plus structured metadata: module name, component/section name, content file path, spec hash at time of mapping.
- Committed to git alongside `.snapshot.json`.
- NOT hashed into the merkle tree (it is metadata about the relationship, not spec content).
- Maintained by `spex apply` (writes on create, updates on review, removes on close).
- Read by skills via `spex map get <record-id>`.

**Record format** (illustrative):
```json
{
  "id": 42,
  "spec_node_id": "impact/component/3",
  "bead_id": "abc-123",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

**Components:**

| Component | Purpose |
|-----------|---------|
| MappingStore | Read/write `spec/.bead-map.json`. CRUD operations on mapping records. |
| PreflightChecker | Deterministic preflight for `/implement` and `/review`: resolve bead → record → spec node, check dependency readiness, report status. |
| MapCommand | CLI: `spex map get <record-id>` returns structured spec info. `spex map list` shows all mappings. `spex check <bead-id>` runs preflight. |

**Bead creation flow:**
1. `spex apply` processes the impact report.
2. For each new bead: create bead via `br` using the component description as bead description.
3. Create a mapping record in `.bead-map.json` linking the spec node to the new bead.
4. Store only the mapping record ID in the bead label (e.g. `spex:42`). One label, one predictable format.

**Skill consumption flow:**
1. Skill reads the bead, extracts the record ID from the label.
2. Skill calls `spex map get <record-id>`.
3. Gets back structured JSON: module name, component name, content file path, metadata.
4. Reads the actual spec content using the returned file path.

No string concatenation, no case manipulation, no coupling to spec naming conventions.

**Changes to existing modules:**

- **Apply**: After bead operations, writes/updates/removes mapping records in `.bead-map.json`. Sets the bead label to the record ID.
- **Impact**: No longer needs label-parsing logic. Consumes ID-based merkle diff directly.
- **`/implement` and `/review` skills**: Call `spex check <bead-id>` and `spex map get <record-id>` instead of parsing bead labels.

**Module dependency**: Map depends on Schema (mapping file format). Apply consumes Map.

## Impact expectation

**New beads:**
- Map module: 3 components (MappingStore, PreflightChecker, MapCommand) + requirements, impl sections, data flows, tests.

**Modified beads:**
- Merkle module: tree_builder, snapshot_store, diff_engine, impact_classifier — switch from path-based to ID-based keying.
- Apply module: ApplyCommand — integrate mapping file maintenance.
- `/implement` skill — replace label-parsing preflight with `spex check` / `spex map get`.
- `/review` skill — same label-parsing replacement.
