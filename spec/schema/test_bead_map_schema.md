# Bead Map Schema Tests

Integration and acceptance tests for BeadMapSchema (component 4). These tests verify that the
journal-line JSON Schema correctly accepts valid lines of the task journal, rejects invalid
ones, and enforces all field constraints — one self-contained object at a time, because the
journal has no envelope.

## Setup

The JSON Schema validator is initialized with the journal-line schema loaded from
`BeadMapSchema()`.

**Fixture: valid change event:**

```json
{"event":"added","eid":"cafe1234:op-7","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}
```

**Fixture: valid task receipts:**

```json
{"event":"task_created","for":"cafe1234:op-7","task_id":"spexmachina-abc"}
{"event":"task_retargeted","for":"cafe1234:op-9","task_id":"spexmachina-abc"}
```

**Fixture: valid registered event:**

```json
{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage",
 "proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234"}
```

**Fixture: legacy epic receipt (admitted as inert history):**

```json
{"event":"task_created","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-0lk"}
```

**Fixture: valid refresh receipt:**

```json
{"event":"refresh","git_head":"cafe1234","absorbed":["cafe1234:op-7"]}
```

## Scenarios

### S1: Each fixture line passes validation

**Input:** the six fixtures above, validated one line at a time.
**Expected:** all pass. Every fixture is a complete, self-contained object — the legacy epic
receipt included, because pre-migration lines validate as inert history.

### S2: Unknown event value fails

**Input:** the change-event fixture with `"event": "renamed"`.
**Expected:** validation fails naming the enum constraint — the admitted values are exactly
`added`, `removed`, `modified`, `registered`, `task_created`, `task_closed`, `task_retargeted`,
`refresh`.

### S3: Change event missing `node` fails

**Input:** the change-event fixture without its `node` field.
**Expected:** validation fails; `node` is required on change events.

### S4: `node` must match the identity-hash pattern

**Input:** a change event with `"node": "not-a-hash"`, and another with `"node": "A1B2C3D4E5F6"`
(uppercase).
**Expected:** both fail — `node` is constrained to `^[a-f0-9]{12}$`. Slug-shaped references
live in `proposal` fields (the registered event's, or a legacy receipt's) and inside the
registered `eid`'s `<git_head>:<slug>` form — never in `node`.

### S5: `before`/`after` admit null but not absence

**Input:** an `added` event with `"before": null` (passes), and one omitting `before` entirely.
**Expected:** the null form passes — absence-of-a-hash is a stated value, not a missing field;
omitting the key fails.

### S6: Task receipt requires exactly one referent

**Input:** a `task_created` with both `for` and `proposal`; another with neither.
**Expected:** both fail. A receipt answers exactly one referent — an event's `eid` via `for`, or
(legacy shape only, never appended anew) a proposal slug.

### S6c: task_retargeted takes the strict shape only

**Input:** the `task_retargeted` fixture; a variant omitting `for`; a variant carrying `proposal`
in place of `for`.
**Expected:** the fixture passes; both variants fail — the legacy slug arm belongs to
`task_created` alone, because no `task_retargeted` line predates the `for` field.

### S6b: Registered event shape

**Input:** the registered fixture; a variant omitting `proposal`; a variant omitting `eid`; a
variant carrying a `node` field.
**Expected:** the fixture passes; all three variants fail — registration opens a proposal's
lifecycle before any spec change exists, so the shape requires `eid`, `proposal` and `git_head`
and admits no `node`.

### S7: `node_type` is a closed enum

**Input:** a change event with `"node_type": "impl_section"`, and another with
`"node_type": "requirement"`.
**Expected:** the first fails — the enum admits exactly `component`, `data_flow`,
`test_section`, `requirement`, `api`; retired kinds have no entry. The second passes: refresh
absorption puts requirement and api events on the record.

### S8: Refresh receipt shape

**Input:** the refresh fixture; a variant with `"absorbed": []`; a variant omitting `absorbed`.
**Expected:** the first two pass (an empty absorbed list is legal in the schema even though
ingest never writes one — the no-drift run appends nothing at all); the third fails.

### S8b: Optional `path` on change events

**Input:** the change-event fixture with `"path": "impact/arch_action_classifier.md"`, and the
same fixture without the key.
**Expected:** both pass — the path is present when the node has a content leaf and absent when it
does not (requirement and api events).

### S9: No integer ids anywhere

**Input:** a line shaped like a retired bead-map record (`{"id": 42, "spec_node_id": "..."}`).
**Expected:** validation fails — the format has no `id`, no `next_id`, no `records` envelope.
This is the regression guard against the retired format creeping back through a stale embed.

## Edge Cases

### E1: Extra properties rejected

A change event carrying an undeclared field (`"color": "red"`) fails —
`additionalProperties: false` on every line shape, so drift in the writer surfaces at the write
boundary rather than accumulating silently.

### E2: Empty object

`{}` fails: `event` is required, and every shape hangs off its value.

### E3: The schema file itself is embedded

`BeadMapSchema()` returns a compilable JSON Schema document — the embed is validated by
compiling it, which catches a truncated or stale `bead-map.schema.json` at test time rather than
on first use.
