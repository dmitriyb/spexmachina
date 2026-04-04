# Bead Matching Tests

Integration and acceptance tests for BeadReader (component 1) and NodeMatcher (component 2). These tests verify that bead metadata is correctly read from the bead CLI and that changed spec nodes are deterministically correlated with existing beads.

## Setup

All scenarios share a common fixture layout:

- A set of mapping records linking spec node IDs to bead IDs:

```json
[
  {"id": 1, "spec_node_id": "module/2/component/1", "bead_id": "spex-001", "module": "validator", "component": "SchemaChecker"},
  {"id": 2, "spec_node_id": "module/3/component/1", "bead_id": "spex-002", "module": "merkle", "component": "Hasher"},
  {"id": 3, "spec_node_id": "module/3/impl_section/1", "bead_id": "spex-003", "module": "merkle", "component": "Hash computation"}
]
```

- A merkle diff with classified changes:

```json
[
  {"path": "module/2/component/1", "type": "modified", "impact": "arch_impl", "module": "validator"},
  {"path": "module/3/impl_section/1", "type": "modified", "impact": "impl_only", "module": "merkle"},
  {"path": "module/2/component/4", "type": "added", "impact": "arch_impl", "module": "validator"},
  {"path": "module/3/component/4", "type": "removed", "impact": "arch_impl", "module": "merkle"}
]
```

## Scenarios

### S1: BeadReader extracts mapping records correctly

Call the mapping store to list records. Assert the returned records contain the expected fields (spec_node_id, bead_id, module, component).

### S2: BeadReader returns empty slice when no records exist

Empty mapping file. Assert an empty slice, not an error.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, records)` with the fixture data. Expected:

- **Matched (2 entries):** `module/2/component/1` matches spex-001, `module/3/impl_section/1` matches spex-003
- **Unmatched (1 entry):** `module/2/component/4` (added, no record)
- **Orphaned:** none in this fixture (no removed node has a matching record)

### S4: NodeMatcher handles multiple beads per spec node

Add a second record for `module/2/component/1`. Assert the match contains both records.

### S5: NodeMatcher uses direct ID comparison

The spec node ID `module/2/component/1` matches by exact string comparison, not by parsing paths or resolving names. Different IDs never match, even if they reference the same logical component.

### S6: Structural changes produce zero matches

Add structural changes to the diff:

```json
{"path": "project/meta", "type": "modified", "impact": "structural", "module": ""},
{"path": "module/2/meta", "type": "modified", "impact": "structural", "module": "validator"}
```

Assert that structural changes produce zero matches, zero unmatched, zero orphans. They are filtered out before the matching loop.

### S7: Deterministic matching — identical inputs produce identical output

Run `MatchNodes` twice with the same inputs (shuffling record order between runs). Assert output is identical in content and order.

### S8: Structural changes coexist with leaf-level changes

Diff contains both structural and leaf-level changes:

```json
[
  {"path": "project/meta", "type": "modified", "impact": "structural", "module": ""},
  {"path": "module/2/meta", "type": "modified", "impact": "structural", "module": "validator"},
  {"path": "module/2/component/1", "type": "modified", "impact": "arch_impl", "module": "validator"}
]
```

Assert that only the leaf-level change (`module/2/component/1`) produces a match. The two structural changes are skipped. Total matches: 1.

## Edge Cases

### E1: Change path with no corresponding module in the modules map

A change referencing a module not present in mapping records. Assert it appears as unmatched (not a panic).

### E2: Bead references a module that has no changes

A record for `module/7` exists but no changes affect module 7. Assert this record does not appear in any output list.

### E3: Removed change with no matching record

A removed change for a spec node that has no mapping record. Assert no orphan is created (removed + no record = no action).
