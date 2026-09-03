# Task State Schema Tests

Integration and acceptance tests for TaskStateSchema. These tests verify that the task-state JSON Schema accepts every well-formed in-flight listing, rejects every document that is not one — the tracker's raw listing above all — and admits no status the lifecycle cannot act on.

## Setup

The JSON Schema validator is initialized with the task-state schema loaded from the schema package's task-state read.

**Fixture: valid artifact with two entries:**

```json
{"version": 1, "tasks": [
  {"task_id": "spexmachina-abc", "status": "open"},
  {"task_id": "spexmachina-def", "status": "in_progress"}
]}
```

**Fixture: valid empty artifact:**

```json
{"version": 1, "tasks": []}
```

## Scenarios

### S1: Both fixtures pass validation

**Input:** the two fixtures above.
**Expected:** both pass. The empty artifact passes on its own merits: an empty `tasks` array is the explicit nothing-in-flight state and the normal state between epics, not a degenerate case.

### S2: The status enum is exactly open and in_progress

**Input:** the two-entry fixture with one entry's status set, in turn, to `closed`, `done`, `blocked`, `""`, and `OPEN`.
**Expected:** every variant fails naming the enum constraint. `closed` is the arm that matters: the artifact cannot express completion, so no consumer can branch on it.

### S3: Version is required and must be 1

**Input:** the fixture with `"version": 2`; with `"version": "1"` (a string); with the key absent.
**Expected:** all three fail. Unlike the journal-line format, whose readers accept every version forward because the file is append-only and permanent, this document is regenerated from the tracker before every run — so a consumer refuses what it does not speak, and absence is not read as version 1.

### S4: An entry admits nothing but task_id and status

**Input:** an entry carrying an extra `labels` array; an entry carrying `title`; an entry carrying `id` in place of `task_id`.
**Expected:** all three fail — `additionalProperties: false` on the entry, and `task_id` required. The first two are the tracker's own fields; the third is the raw listing's spelling of the id. Neither may cross into the binary, and the schema is where the boundary is enforced rather than the export script.

### S5: The envelope admits nothing but version and tasks

**Input:** the fixture with an extra top-level `generated_at`; the fixture with `tasks` absent; a document whose only top-level key is `tasks`.
**Expected:** all three fail.

### S6: The raw tracker listing is refused, not recognised

**Input:** `{"issues": [{"id": "spexmachina-abc", "status": "open", "labels": []}]}` — the shape the retired input took — and a bare array of the same objects.
**Expected:** both fail. The refusal is the ordinary schema refusal — no `version`, no `tasks` — and no reader treats either as a legacy form to be adapted.

### S7: task_id is a non-empty string

**Input:** an entry with `"task_id": ""`; an entry with `"task_id": 42`.
**Expected:** both fail.

## Edge Cases

### E1: Empty object

`{}` fails: `version` and `tasks` are both required.

### E2: The schema file itself is embedded

The schema package's task-state read returns a compilable JSON Schema document — the embed is validated by compiling it, which catches a truncated or stale `task-state.schema.json` at test time rather than on first use.
