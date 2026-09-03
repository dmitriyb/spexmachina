# Task Matching Tests

Integration and acceptance tests for TaskReader and NodeMatcher. These tests verify that task state is correctly read from the task-state artifact the caller supplies as a file — never from a tracker command this binary runs, and never from the tracker's own listing format — and that changed spec nodes are deterministically correlated with existing tasks using identity hashes.

## Setup

Scenarios split by what they exercise. S1, S2 and S2b exercise TaskReader alone, against the shape it actually consumes: the task-state artifact (`arch_task_reader.md`'s "Input Shape"), not the task journal — TaskReader starts no process, contacts no tracker, and never touches the journal. S3 onward exercise NodeMatcher against journal-derived pairings, since folding the journal into pairings is `mapping.MappingStore`'s job (see `spec/map/test_mapping_store.md`), not TaskReader's.

Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HASR_HASH`, etc.) so the test data stays readable; the values themselves are computed once at fixture-load time via `schema.IdentityHash`.

- A task-state artifact (the shape TaskReader's `--tasks` input takes), for S1/S2. Only in-flight tasks appear in it — an artifact never lists a finished task, and there is no status value that could:

```json
{
  "version": 1,
  "tasks": [
    {"task_id": "spex-001", "status": "open"},
    {"task_id": "spex-002", "status": "in_progress"},
    {"task_id": "spex-003", "status": "open"}
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

The `node_type` field is part of the change record because identity hashes do not embed type information; downstream consumers (ActionClassifier, and ChangesetBuilder via the action's `NodeType`) read it from this field.

## Scenarios

### S1: TaskReader carries id and status, parses nothing else

Parse the artifact above. Assert each entry carries the two fields the interface promises (`ID`, `Status`), in input order, and that no entry exposes anything beyond them — the linkage lives in the journal fold, and status joins onto it by task id. The `in_progress` status on `spex-002` is carried through verbatim: the retarget/refuse split downstream reads it exactly as the input spelled it.

### S2: TaskReader returns empty slice on an empty artifact

`{"version": 1, "tasks": []}`. Assert an empty slice rather than an error: an empty list is the explicit statement that nothing is in flight, and it is the normal state of a project between epics.

### S2b: TaskReader refuses every document that is not a version-1 task-state artifact

Four inputs, each refused with an error beginning `plan: read tasks:` and naming the constraint, none returning entries:

- `{"version": 2, "tasks": []}` — the version is not the one the reader speaks; a reader that guessed would silently misread a future shape.
- `{"version": 1, "tasks": [{"task_id": "spex-004", "status": "closed"}]}` — `closed` is not a value the format has. This is the arm that pins the design: the artifact cannot express completion, so no consumer can ever branch on it.
- `{"version": 1, "tasks": [{"task_id": "spex-005", "status": "open", "labels": []}]}` — an undeclared property on an entry. The tracker's listing fields never enter the binary, and the schema's `additionalProperties: false` is what keeps them out.
- `{"issues": [{"id": "spex-006", "status": "open"}]}` — a raw tracker listing, the shape the retired input took. It has no `version` and no `tasks`, and it is refused as any other malformed document is, not recognised as a legacy form.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, pairings)` with the fixture data. Expected:

- **Matched (2 entries):** `SCHK_HASH` matches spex-001, `HTST_HASH` matches spex-003
- **Unmatched (1 entry):** `NEW_COMP` (added, no pairing)
- **Orphaned:** none in this fixture (no removed node has a matching pairing)

### S4: NodeMatcher handles multiple tasks per spec node

Append a second `modified` + `task_created` pair for `SCHK_HASH` — a successor task after the first completed; no `task_closed` separates the two, because the journal never records completion. Assert the match carries the node's current pairing, with the lineage reachable through the journal history — the fold answers latest-wins, so one pairing is what it hands over here.

Then assert the interface contract directly, independent of what the fold happens to produce: hand `MatchNodes` two pairings that both store `SCHK_HASH` and assert the match carries **both**, in task-id order, not the first alone. The fold's latest-wins rule is why the list holds one entry in practice, and matching is specified to neither rely on that nor enforce it — so the plural case has to be constructed to be tested at all, and a matcher that returned the first entry would pass every fold-derived fixture.

### S4b: A retargeted pairing matches like any other

Append a `modified` event for `SCHK_HASH` plus a `task_retargeted` receipt naming spex-001. Assert the match still pairs `SCHK_HASH` with spex-001 — the fold moved the pairing's sourcing event forward, the task id did not change, and NodeMatcher sees one current pairing exactly as before. Retargeting is invisible to matching; only the sourcing event's `after` hash (consulted downstream by the already-tracked cell) moved.

### S4c: TaskReader's output joins onto the pairings, and NodeMatcher carries the result

The one scenario that crosses both components. Parse, with TaskReader, an artifact listing spex-001 as `open` and spex-002 as `in_progress` and nothing else; join the entries it returns onto the fixture pairings by task id — the join PlanCommand performs — and hand the enriched pairings to NodeMatcher with the fixture diff. Assert the matched entry for `SCHK_HASH` carries `open` and the pairing for `HASR_HASH` (not in the diff) reaches no list, so the status never leaks past matching; then modify `HASR_HASH` in the diff and assert its matched entry carries `in_progress` verbatim. Assert the matched entry for `HTST_HASH` (spex-003) carries no status — not `closed`, not an error, simply unset — and that matching still pairs it exactly as S3 does. Absence is data the classifier reads downstream; matching neither invents a value for it nor drops the pairing. A reader that mislabeled the statuses, or a matcher that dropped unlisted pairings, fails here and nowhere in S1–S4b.

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

Confirm that no helper exists to rewrite a pairing's node key into a different shape before matching, and no helper exists to rewrite `change.Key` into a different shape before lookup. The lookup is performed against the raw node keys as they appear in the journal. This is a regression guard for the deleted `buildMerkleIndex` function — re-introducing it would mean the dual-format problem has crept back in.

## Edge Cases

### E1: Change for a node whose module has no pairings

A change with an identity hash that does not appear in any pairing. Assert it appears as unmatched (not a panic).

### E2: Record for a node that has no changes

A pairing exists for an identity hash that does not appear in the diff. Assert this pairing does not appear in any output list.

### E3: Removed change with no matching pairing

A removed change for a spec node that has no pairing. Assert no orphan is created (removed + no pairing = no action).
