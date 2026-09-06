# Task Matching Tests

Integration and acceptance tests for TaskReader and NodeMatcher. These tests verify that task state is correctly read from the task-state artifact the caller supplies as a file — never from a tracker command this binary runs, and never from the tracker's own listing format — and that changed spec nodes are deterministically correlated with existing tasks using identity hashes.

## Setup

S4c exercises TaskReader against the shape it actually consumes: the task-state artifact (`arch_task_reader.md`'s "Input Shape"), not the task journal — TaskReader starts no process, contacts no tracker, and never touches the journal. It exercises NodeMatcher against journal-derived pairings, since folding the journal into pairings is `mapping.MappingStore`'s job (see `spec/map/test_mapping_store.md`), not TaskReader's.

Identity hashes in fixtures are placeholder constants (`SCHK_HASH`, `HASR_HASH`, etc.) so the test data stays readable; the values themselves are computed once at fixture-load time via `schema.IdentityHash`.

- A task-state artifact (the shape TaskReader's `--tasks` input takes). Only in-flight tasks appear in it — an artifact never lists a finished task, and there is no status value that could:

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

- A journal fixture pairing identity hashes with task IDs. The store validates each line against the journal-line schema on read; a malformed line is refused naming its number. The fixture seeds one `added` change event plus one `task_created` receipt per node:

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

### S4c: TaskReader's output joins onto the pairings, and NodeMatcher carries the result

**Given** Setup's journal pairings and merkle diff, and a task-state artifact listing spex-001 as `open` and spex-002 as `in_progress` and nothing else.

The one scenario that crosses both components. Parse, with TaskReader, an artifact listing spex-001 as `open` and spex-002 as `in_progress` and nothing else; join the entries it returns onto the fixture pairings by task id — the join PlanCommand performs — and hand the enriched pairings to NodeMatcher with the fixture diff. Assert the matched entry for `SCHK_HASH` carries `open` and the pairing for `HASR_HASH` (not in the diff) reaches no list, so the status never leaks past matching; then modify `HASR_HASH` in the diff and assert its matched entry carries `in_progress` verbatim. Assert the matched entry for `HTST_HASH` (spex-003) carries no status — not `closed`, not an error, simply unset — and that matching still pairs it. Absence is data the classifier reads downstream; matching neither invents a value for it nor drops the pairing. A reader that mislabeled the statuses, or a matcher that dropped unlisted pairings, fails here.

## Edge Cases

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
