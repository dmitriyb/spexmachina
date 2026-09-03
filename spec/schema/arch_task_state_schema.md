# TaskStateSchema

The task-state JSON Schema definition, shipped as `schema/task-state.schema.json`. It is the contract for the one input `spex plan` takes from the tracker — the document it reads through its required `--tasks` flag — and [[54f935712fd1|the whole of what that input may say]]: which tasks are in flight, and whether each is claimed. The document is spex's own — versioned like the changeset and the receipts — so the tracker's listing format never crosses into the binary. An adapter's export half projects the tracker onto this shape; plan's TaskReader validates against it and reads nothing the schema does not admit.

## Scope

Defines the JSON Schema for one task-state document:

- **Envelope**: `version`, required, an integer that must be `1`; `tasks`, required, an array. No other property is admitted.
- **Entry**: `task_id`, required, a non-empty string — the tracker's own id for the task, the same value a receipt's `task_id` and a journal receipt's `task_id` carry, so the join onto a journal pairing is a string comparison; `status`, required, drawn from exactly `open` and `in_progress`. No other property is admitted on an entry.
- **What is absent**: there is no `closed`, no `done`, no third status of any kind, and no way to add one without a version bump. The artifact lists in-flight work only — a handful of entries mid-epic, an empty array after one — and an empty `tasks` array is a legal, explicit statement that nothing is in flight. A task the document does not list has no live work; what that means for the task's node is plan's decision, made from the journal pairing plus the absence, never from a status.

```json
{
  "version": 1,
  "tasks": [
    {"task_id": "spexmachina-abc", "status": "open"},
    {"task_id": "spexmachina-def", "status": "in_progress"}
  ]
}
```

## Design Notes

### Bounded by work in progress, not by history

The input this document replaced was the tracker's raw listing: every task the project ever had, every field the tracker keeps, growing with history forever and owned by a format spex did not control — measured at several hundred entries carrying a dozen and more fields each, of which plan read two. This schema inverts every one of those properties. The document is bounded by the work currently open, it carries the two fields plan reads and nothing else, and its shape is fixed here rather than inherited: a change to the tracker's own output touches the adapter's export script and never the binary.

### Why no closed status

Completion is not a fact the artifact carries, because carrying it would let a consumer branch on it — and the lifecycle this contract serves has no such branch. A task in the artifact is live: open work is retargeted or closed, claimed work refuses the run. A task absent from the artifact is finished, and the journal pairing that still names it is the record of what it did. Stating absence as the only way to say "done" is what keeps the two decisions plan makes decidable from one bounded input; a `closed` value would be a third decision hiding in a status enum.

### Versioned and refused, not tolerated

`version` is required and the consumer refuses any value but the one it speaks. A reader that tolerated an unknown version would have to guess at the entry shape, and a wrong guess on this input re-creates in-flight work or leaves finished work uncancelled. Refusal is the same rule the changeset and receipts follow, for the same reason.

### Closed against the tracker's fields

`additionalProperties: false` on the entry and on the envelope is the mechanism that keeps the tracker's vocabulary out. An export script that leaked a `labels` array, a `title` or a tracker-specific `issue_type` into the document would fail validation at plan's read, not pass silently and grow a dependency on a field nobody declared. The boundary is enforced by the schema, not by the export script's discipline.
