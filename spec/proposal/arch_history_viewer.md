# HistoryViewer

Shows proposal history — which proposals led to which spec changes and task actions — and so is
where [[2e38135487be|Show proposal history]] is realised. It is handed task records its caller has
already read and parsed, and performs no subprocess invocation: the caller (ProposalCommands) takes
`br list --json` on stdin, parses it, and passes the records to the viewer.

## Responsibilities

- Group tasks by `spec_proposal:<stem>` label (proposal filename without `.md`).
- For each group, read the proposal file from `spec/proposals/<stem>.md` to confirm it exists and pull the first-line title.
- Render the link chain: proposal → tasks created/closed/modified.

## Interface

The viewer consumes four things and reaches for nothing else: the task records its caller has
already parsed, the spec root under which `proposals/` is resolved (`spec/` unless `--spec-dir` says
otherwise), the destination the rendering is written to, and the choice between the two renderings —
human-readable text by default, the JSON envelope described below when the caller asks for it. Each
task record carries the task's id, its status, its labels, its title, and its creation and closing
timestamps where the tracker supplied them; of those, only the labels decide which proposal a task
is grouped under. The record type is the proposal module's own — the corpus calls it a task
record, and the Go type is named accordingly — and it is the one place inside the binary that
still reads the tracker's listing shape directly, because `spex log` is a read-only view outside
the pipeline and no adapter half sits in front of it.

The viewer does NOT fetch tasks itself. That responsibility is the caller's — the `ProposalCommands`
handler for `spex log` reads stdin, decodes the task JSON, and passes the records over.

That split is a declared fact of the graph, not something a reader has to infer from the call chain: [[44a9d44c7eec|`spex log`]] is declared with two providers rather than one, ProposalCommands and HistoryViewer, which is the graph's way of saying that neither half is the command on its own. The parse belongs to the command, the rendering belongs here, and a change to either changes what a user typing `spex log` sees. The other two surfaces this module exposes, `spex register` and `spex template`, pair ProposalCommands with Registrar and TemplateProvider the same way.

## Why Accept Parsed Data

Taking parsed data is a declared contract rather than a convenience: this component implements
[[76c339aebbcd|Task data via stdin]], and the proposal-level requirement "No runtime subprocesses"
(project req `58ea35f52b86`) forbids any pipeline surface running a tracker at all — the requirement's sole sanctioned exception, the upgrade command's embedded-installer drive, is nowhere near this path. The previous
lister type ran `br list --json` as a subprocess; it is retired. The viewer is a pure function
over data it's given.

This also makes the viewer trivial to test: feed it a fixture set of task records, assert the rendered output.

## Output Format

```
2026-02-23-spex-machina.md (project proposal)
  Created: spexmachina-abc (open)     schema: ProjectSchema
  Created: spexmachina-def (open)     validator: SchemaChecker
  ...

2026-04-12-data-flow-contract-layer.md (change proposal)
  Created: spexmachina-123 (closed)   merkle: DiffCommand
  Closed:  spexmachina-old (closed)   plan: NodeMatcher
  ...
```

The sequence is not incidental. Proposal groups come out in ascending order of proposal stem, which
reads as date order given the `YYYY-MM-DD-` prefix the registrar's naming convention puts first, and
within a group the tasks keep the order they arrived in. That ordering is settled before either
rendering is chosen, so the JSON envelope below lists proposals in exactly the same sequence as the
text above.

## Label Parsing

Each task's `spec_proposal:<stem>` label determines its group. Tasks without that label are skipped (not part of any proposal). If a task has multiple `spec_proposal:` labels, the first is used — that shouldn't happen in practice, but is handled defensively.

## Missing Proposal Files

If a task's `spec_proposal` label references a proposal file that doesn't exist in `spec/proposals/`, the viewer renders:

```
<unknown-proposal>.md (proposal file missing)
  Created: ...
```

rather than erroring. The task's provenance is still visible; the user can investigate whether the proposal file was renamed, moved, or never committed.

## JSON Output Mode

`spex log --json` flag: emit the grouped structure as machine-readable JSON:

```json
{
  "proposals": [
    {
      "filename": "2026-04-18-decouple-spex-from-br.md",
      "title": "Decouple spex binary from br/bd",
      "date": "2026-04-18",
      "tasks": [
        {"id": "spexmachina-abc", "status": "open", "action": "created", "summary": "emit: ChangesetBuilder"},
        ...
      ]
    }
  ]
}
```

Useful for piping into other tools. Not the default (human-readable text is default). The per-proposal array is keyed `tasks`; the envelope speaks the corpus vocabulary and emits no alias for the retired key.

`date` is the proposal's date as [[2e38135487be|Show proposal history]] asks for it, and it is read from the filename alone: the `YYYY-MM-DD` prefix the registrar's naming convention puts first, or the empty string when the stem carries no such prefix. Nothing else supplies it — not the file's modification time, not the tracker's timestamps — so the same records over the same directory always render the same envelope. The text rendering carries no separate date: the filename it prints already leads with the same prefix.

## Non-Responsibilities

- Does not reach into the tracker — the caller supplies task data.
- Does not modify tasks or labels.
- Does not create proposal files — that's Registrar.
