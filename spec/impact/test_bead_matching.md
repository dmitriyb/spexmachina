# Bead Matching Tests

Integration and acceptance tests for BeadReader (component 1) and NodeMatcher (component 2). These tests verify that bead metadata is correctly read from the tracker listing the caller supplies as a file, never from a tracker command this binary runs, and that changed spec nodes are deterministically correlated with existing beads using identity hashes.

## Setup

All scenarios share a common fixture layout. Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HASR_HASH`, etc.) so the test data stays readable; the values themselves are computed once at fixture-load time via `schema.IdentityHash`.

- A bead-map document linking identity hashes to bead IDs. The mapping store schema-validates the file before parsing it, so the fixture is an object with `next_id` and `records`, and each record carries `bead_type`, `content_file` and `spec_hash` alongside the four fields the scenarios read. A bare array of records is refused with `map: schema validation …` and exit 1. `content_file` and `spec_hash` carry placeholders here, as the identity hashes do:

```json
{
  "next_id": 4,
  "records": [
    {"id": 1, "spec_node_id": "<SCHK_HASH>", "bead_id": "spex-001", "bead_type": "task", "module": "validator", "component": "SchemaChecker",    "content_file": "<SCHK_FILE>", "spec_hash": "<SCHK_SPEC_SHA>"},
    {"id": 2, "spec_node_id": "<HASR_HASH>", "bead_id": "spex-002", "bead_type": "task", "module": "merkle",    "component": "Hasher",           "content_file": "<HASR_FILE>", "spec_hash": "<HASR_SPEC_SHA>"},
    {"id": 3, "spec_node_id": "<HTST_HASH>", "bead_id": "spex-003", "bead_type": "task", "module": "merkle",    "component": "Hashing tests",    "content_file": "<HTST_FILE>", "spec_hash": "<HTST_SPEC_SHA>"}
  ]
}
```

Where:
- `SCHK_HASH = IdentityHash("validator", "component", "SchemaChecker")`
- `HASR_HASH = IdentityHash("merkle", "component", "Hasher")`
- `HTST_HASH = IdentityHash("merkle", "test_section", "Hashing tests")`

- A merkle diff with classified changes (the `path` field is the same identity hash that appears in `spec_node_id`):

```json
[
  {"path": "<SCHK_HASH>",  "type": "modified", "impact": "arch_impl", "module": "validator", "node_type": "component"},
  {"path": "<HTST_HASH>",  "type": "modified", "impact": "impl_only", "module": "merkle",    "node_type": "test_section"},
  {"path": "<NEW_COMP>",   "type": "added",    "impact": "arch_impl", "module": "validator", "node_type": "component"},
  {"path": "<REMOVED>",    "type": "removed",  "impact": "arch_impl", "module": "merkle",    "node_type": "component"}
]
```

The `node_type` field is now part of the change record because identity hashes do not embed type information; downstream consumers (ActionClassifier, and emit's ChangesetBuilder via the action's `NodeType`) read it from this field.

## Scenarios

### S1: BeadReader extracts mapping records correctly

Call the mapping store to list records. Assert the returned records contain the expected fields (spec_node_id, bead_id, module, component) and that every `spec_node_id` matches the identity hash pattern `^[a-f0-9]{12}$`.

### S2: BeadReader returns empty slice when no records exist

A mapping file holding no records — a bead-map document whose `records` array is empty — and, separately, no file at the mapping path at all. Assert an empty slice rather than an error for each. A zero-byte file is a different case: it is not a bead-map document, and it is refused.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, records)` with the fixture data. Expected:

- **Matched (2 entries):** `SCHK_HASH` matches spex-001, `HTST_HASH` matches spex-003
- **Unmatched (1 entry):** `NEW_COMP` (added, no record)
- **Orphaned:** none in this fixture (no removed node has a matching record)

### S4: NodeMatcher handles multiple beads per spec node

Add a second record with `spec_node_id: SCHK_HASH`. Assert the match contains both records.

### S5: NodeMatcher uses direct identity-hash comparison

`SCHK_HASH` matches by exact string equality, not by parsing or rebuilding any path. Different identity hashes never match, even when they reference logically related nodes (e.g., a component and a test_section in the same module). Two distinct spec nodes always have distinct identity hashes by construction.

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
