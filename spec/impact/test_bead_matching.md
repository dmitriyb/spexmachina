# Bead Matching Tests

Integration and acceptance tests for BeadReader (component 1) and NodeMatcher (component 2). These tests verify that bead metadata is correctly read from the tracker listing the caller supplies as a file, never from a tracker command this binary runs, and that changed spec nodes are deterministically correlated with existing beads using identity hashes.

## Setup

Scenarios split by what they exercise. S1 and S2 exercise BeadReader alone, against the shape it actually consumes: a tracker listing (`arch_bead_reader.md`'s "Input Shape"), not the task journal — BeadReader starts no process and contacts no tracker, and never touches `.history.jsonl`. S3 onward exercise NodeMatcher against journal-derived pairings, since folding the journal into pairings is `mapping.MappingStore`'s job (see `spec/map/test_mapping_store.md`), not BeadReader's.

Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HASR_HASH`, etc.) so the test data stays readable; the values themselves are computed once at fixture-load time via `schema.IdentityHash`.

- A tracker listing (the shape BeadReader's `--beads` input takes) pairing identity hashes with bead ids, for S1/S2:

```json
{
  "issues": [
    {"id": "spex-001", "status": "open",        "labels": ["spex:<SCHK_HASH>", "commit:deadbeef"]},
    {"id": "spex-002", "status": "in_progress", "labels": ["spex:<HASR_HASH>"]},
    {"id": "spex-003", "status": "open",        "labels": ["spex:<HTST_HASH>"]}
  ]
}
```

- A journal fixture pairing identity hashes with task IDs, for S3 onward. The store validates each line against the journal-line schema on read; a malformed line is refused naming its number. The fixture seeds one `added` change event plus one `task_created` receipt per node:

```json
{"event":"added","eid":"<E1>","node":"<SCHK_HASH>","name":"SchemaChecker","node_type":"component","module":"validator","before":null,"after":"<SCHK_SPEC_SHA>","git_head":"<HEAD>","proposal":"<P>"}
{"event":"task_created","for":"<E1>","task_id":"spex-001"}
{"event":"added","eid":"<E2>","node":"<HASR_HASH>","name":"Hasher","node_type":"component","module":"merkle","before":null,"after":"<HASR_SPEC_SHA>","git_head":"<HEAD>","proposal":"<P>"}
{"event":"task_created","for":"<E2>","task_id":"spex-002"}
{"event":"added","eid":"<E3>","node":"<HTST_HASH>","name":"Hashing tests","node_type":"test_section","module":"merkle","before":null,"after":"<HTST_SPEC_SHA>","git_head":"<HEAD>","proposal":"<P>"}
{"event":"task_created","for":"<E3>","task_id":"spex-003"}
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

### S1: BeadReader extracts pairings correctly

Parse the tracker listing above. Assert each entry carries the four fields the interface promises (`ID`, `SpecNodeID`, `Status`, `Labels`), that `SpecNodeID` reads back the identity hash out of the bead's `spex:<spec_node_id>` label, and that every `SpecNodeID` matches the identity hash pattern `^[a-f0-9]{12}$`.

### S2: BeadReader returns empty slice when no pairings exist

A tracker listing whose beads carry no `spex:` label — and, separately, an empty `issues` array, and an empty bare array. Assert an empty slice rather than an error for each of the three: absence and emptiness are both first-class states, not error conditions.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, pairings)` with the fixture data. Expected:

- **Matched (2 entries):** `SCHK_HASH` matches spex-001, `HTST_HASH` matches spex-003
- **Unmatched (1 entry):** `NEW_COMP` (added, no pairing)
- **Orphaned:** none in this fixture (no removed node has a matching pairing)

### S4: NodeMatcher handles multiple beads per spec node

Append a modify pair for `SCHK_HASH` (second `task_created` after a `task_closed`). Assert the match carries the node's current pairing, with the lineage reachable through the journal history.

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

Run `MatchNodes` twice with the same inputs (shuffling pairing order between runs). Assert output is identical in content and order.

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

### S9: No rekeying — pairings and changes share one format

Confirm that no helper exists to rewrite a pairing's node key into a different shape before matching, and no helper exists to rewrite `change.Key` into a different shape before lookup. The `index[change.Key]` lookup is performed against the raw node keys as they appear in the journal. This is a regression guard for the deleted `buildMerkleIndex` function — re-introducing it would mean the dual-format problem has crept back in.

## Edge Cases

### E1: Change for a node whose module has no pairings

A change with an identity hash that does not appear in any pairing. Assert it appears as unmatched (not a panic).

### E2: Record for a node that has no changes

A pairing exists for an identity hash that does not appear in the diff. Assert this pairing does not appear in any output list.

### E3: Removed change with no matching pairing

A removed change for a spec node that has no pairing. Assert no orphan is created (removed + no pairing = no action).
