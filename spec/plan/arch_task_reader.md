# TaskReader

[[6ffc29a566ce|Reading task state]] happens here and nowhere else: a caller hands over the bytes of the task-state artifact, and TaskReader hands back one entry per in-flight task. It starts no process and contacts no tracker — and it parses nothing the artifact's schema does not declare: no label, no title, no field of the tracker's own. The input is spex's own versioned document, not a tracker listing, and the reader refuses anything else.

## Responsibilities

- Validate the document against the task-state schema — version `1`, a `tasks` array, each entry a `task_id` and a `status` of `open` or `in_progress`, nothing more — and refuse any document that fails it, naming the constraint.
- Decode the validated document into one entry per task, carrying each entry's `task_id` and `status` through untouched, so that [[8987ef169e48|the cleanup gate for a removed spec node]] downstream can tell a finished task from a live one, and the retarget split can tell an open task from a claimed one.
- Return an empty result, not an error, for a document whose `tasks` array is empty: nothing in flight is the normal state between epics, and the artifact says so explicitly rather than by absence.
- Return the entries in the order the input array gave them. A caller wanting any other order sorts after the call.

Which tasks are spec-managed is not this component's question: pairings come from the journal fold carrying task ids, live status joins onto them by task id, and a task the fold never names simply never joins. Nor is completion its question — the artifact cannot express it, and the reader reports only what is listed.

## Interface

One call, taking either a stream to read from or the bytes already in hand. Each entry it returns carries two things: the tracker's own task id (`spexmachina-abc` and the like) and the task's live status exactly as the input spelled it. A failure is returned to the caller rather than printed, and every message begins `plan: read tasks:`.

## Input Shape

The artifact is the version-1 task-state document the schema module's TaskStateSchema declares:

```json
{
  "version": 1,
  "tasks": [
    {"task_id": "spexmachina-abc", "status": "open"},
    {"task_id": "spexmachina-def", "status": "in_progress"}
  ]
}
```

Only in-flight tasks belong in it: a handful of entries mid-epic, an empty list after one. It is bounded by work in progress, never by tracker history. There is no `closed` value and no third status: a task the document does not list has no live work, and what that means for the task's node — a plain create, a cleanup — is decided downstream from the journal pairing plus the absence, never from a status carried here.

The document is produced by the adapter's export half, the component that owns the tracker-format boundary. For the reference adapter that is one script projecting `br list` onto these two fields; a change in the tracker's own output touches that script and never this reader. The retired input — the tracker's raw listing, hundreds of entries carrying a dozen and more fields each, of which two were read — is refused as any other malformed document: it has no `version` and no `tasks`, and no legacy branch recognises it.

## Error Handling

- Malformed JSON → `"plan: read tasks: parse: <json err>"`.
- A document failing the schema — wrong or missing `version`, a status outside the enum, an undeclared property, a missing `task_id` — → `"plan: read tasks: <constraint>"`, naming the violated constraint and, for an entry, its index.

## No Subprocess

The reader never runs the tracker's own listing command itself. Callers supply the artifact as a file instead, which is the whole of [[ac02e911999d|accepting task state as a `--tasks` input]] — a required input, because a run without it would read every task as finished and re-create in-flight work.

TaskReader is the inbound half of the project-level rule that `spex` never invokes a tracker CLI: tracker state enters the plan pipeline here, as a file the adapter's export half produced, and leaves as changeset ops that the adapter's apply half executes. It is not the binary's only tracker-state inlet — `spex ingest` reads `receipts.json`, whose `task_id` and `was_existing` are tracker-assigned and load-bearing (see `spec/ingest/arch_reconciler.md`, "One write path, no tracker"). What both inlets share is the shape: tracker state always arrives as a file some other process produced, in a format spex declares, and no code path in between ever runs a tracker command. The practical consequence for this component is that it owns **no** freshness guarantee: the `status` it carries is as current as the file it was handed, and the responsibility for that file being a live read belongs to the caller. Re-adding a subprocess here to "make sure the data is fresh" would reintroduce the tracker coupling the whole pipeline is shaped to avoid, and would make `spex plan` non-deterministic over its inputs.

The status the caller supplies decides both of plan's decisions: whether a changed node's task is retargeted, refuses the run, or is followed by a plain create; and whether a removed node's task is closed, refuses the run, or is followed by a cleanup. A stale artifact cannot corrupt anything — the run it feeds is still deterministic over its inputs — but it can refuse a run that would have passed against a live export, retarget a task an operator claimed a minute ago, or re-create work that was still open when the export was older than it should be; the tracker's own state at adapter time is what the update lands against, and the adapter's receipts are what ingest believes.

## The journal carries the linkage; the artifact carries a status

A task's link back into the spec lives in the task journal, not in the artifact: the fold pairs each node's identity hash with the task id the tracker minted for it, and TaskReader's entries join onto those pairings by task id. The artifact contributes exactly one fact the journal cannot know — whether the task is still live, and whether someone has claimed it — and TaskReader reads that fact and stops: whether the paired node still exists, and what kind it is, are discovered later, by NodeMatcher joining the fold against the diff, not by reading anything more off the entry.

The consequence is that nothing about the spec graph, and nothing about the label scheme, ever needs a tracker rewrite to stay joinable. Moving content between files, retiring a kind of section, re-describing a node, or changing what plan stamps into `idempotency.label` leaves every existing pairing valid, because the join key is the task id the receipt recorded at creation time. Rewriting a live tracker is precisely what the pipeline has no way to do, since `spex` never invokes a tracker CLI; keying the join on the journal is what makes it never need to.

## Testing

- Unit-level: canned JSON fixtures exercise each extraction path and each refusal.
- See `test_task_matching.md` for integration tests that feed TaskReader output into NodeMatcher.
