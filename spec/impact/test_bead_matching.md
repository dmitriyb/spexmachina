# Bead Matching Tests

Integration and acceptance tests for BeadReader (component 1) and NodeMatcher (component 2). These tests verify that bead metadata is correctly read from the bead CLI and that changed spec nodes are deterministically correlated with existing beads using identity hashes.

## Setup

All scenarios share a common fixture layout. Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HASR_HASH`, etc.) so the test data stays readable; the values themselves are computed once at fixture-load time via `schema.IdentityHash`.

- A set of mapping records linking identity hashes to bead IDs:

```json
[
  {"id": 1, "spec_node_id": "<SCHK_HASH>",  "bead_id": "spex-001", "module": "validator", "component": "SchemaChecker"},
  {"id": 2, "spec_node_id": "<HASR_HASH>",  "bead_id": "spex-002", "module": "merkle",    "component": "Hasher"},
  {"id": 3, "spec_node_id": "<HCMP_HASH>",  "bead_id": "spex-003", "module": "merkle",    "component": "Hash computation"}
]
```

Where:
- `SCHK_HASH = IdentityHash("validator", "component", "SchemaChecker")`
- `HASR_HASH = IdentityHash("merkle", "component", "Hasher")`
- `HCMP_HASH = IdentityHash("merkle", "impl_section", "Hash computation")`

- A merkle diff with classified changes (the `path` field is the same identity hash that appears in `spec_node_id`):

```json
[
  {"path": "<SCHK_HASH>",  "type": "modified", "impact": "arch_impl", "module": "validator", "node_type": "component"},
  {"path": "<HCMP_HASH>",  "type": "modified", "impact": "impl_only", "module": "merkle",    "node_type": "impl_section"},
  {"path": "<NEW_COMP>",   "type": "added",    "impact": "arch_impl", "module": "validator", "node_type": "component"},
  {"path": "<REMOVED>",    "type": "removed",  "impact": "arch_impl", "module": "merkle",    "node_type": "component"}
]
```

The `node_type` field is now part of the change record because identity hashes do not embed type information; downstream consumers (ActionClassifier, ApplyCommand) read it from this field.

## Scenarios

### S1: BeadReader extracts mapping records correctly

Call the mapping store to list records. Assert the returned records contain the expected fields (spec_node_id, bead_id, module, component) and that every `spec_node_id` matches the identity hash pattern `^[a-f0-9]{12}$`.

### S2: BeadReader returns empty slice when no records exist

Empty mapping file. Assert an empty slice, not an error.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, records)` with the fixture data. Expected:

- **Matched (2 entries):** `SCHK_HASH` matches spex-001, `HCMP_HASH` matches spex-003
- **Unmatched (1 entry):** `NEW_COMP` (added, no record)
- **Orphaned:** none in this fixture (no removed node has a matching record)

### S4: NodeMatcher handles multiple beads per spec node

Add a second record with `spec_node_id: SCHK_HASH`. Assert the match contains both records.

### S5: NodeMatcher uses direct identity-hash comparison

`SCHK_HASH` matches by exact string equality, not by parsing or rebuilding any path. Different identity hashes never match, even when they reference logically related nodes (e.g., a component and an impl_section in the same module). Two distinct spec nodes always have distinct identity hashes by construction.

### S6: Structural changes produce zero matches

Add structural changes to the diff:

```json
{"path": "meta/project",       "type": "modified", "impact": "structural", "module": ""},
{"path": "meta/<MOD_HASH>",    "type": "modified", "impact": "structural", "module": "validator"}
```

Assert that structural changes produce zero matches, zero unmatched, zero orphans. They are filtered out before the matching loop. The synthetic `meta/` prefix is the only key in the merkle tree that is not a pure identity hash.

### S7: Deterministic matching — identical inputs produce identical output

Run `MatchNodes` twice with the same inputs (shuffling record order between runs). Assert output is identical in content and order.

### S8: Structural changes coexist with leaf-level changes

Diff contains both structural and leaf-level changes:

```json
[
  {"path": "meta/project",     "type": "modified", "impact": "structural", "module": ""},
  {"path": "meta/<MOD_HASH>",  "type": "modified", "impact": "structural", "module": "validator"},
  {"path": "<SCHK_HASH>",      "type": "modified", "impact": "arch_impl",  "module": "validator", "node_type": "component"}
]
```

Assert that only the leaf-level change (`SCHK_HASH`) produces a match. The two structural changes are skipped. Total matches: 1.

### S9: No rekeying — records and changes share one format

Confirm that no helper exists to rewrite `record.SpecNodeID` into a different shape before matching, and no helper exists to rewrite `change.Key` into a different shape before lookup. The `index[change.Key]` lookup is performed against the raw `record.SpecNodeID` strings as they appear on disk. This is a regression guard for the deleted `buildMerkleIndex` function — re-introducing it would mean the dual-format problem has crept back in.

## Edge Cases

### E1: Change for a node whose module has no mapping records

A change with an identity hash that does not appear in any record. Assert it appears as unmatched (not a panic).

### E2: Record for a node that has no changes

A record exists for an identity hash that does not appear in the diff. Assert this record does not appear in any output list.

### E3: Removed change with no matching record

A removed change for a spec node that has no mapping record. Assert no orphan is created (removed + no record = no action).
