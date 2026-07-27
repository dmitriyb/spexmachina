# HistoryViewer

Shows proposal history — which proposals led to which spec changes and bead actions. Accepts parsed `[]BeadRecord` from its caller; performs no subprocess invocation. The caller (ProposalCommands) pipes `br list --json` from stdin, parses it into `BeadRecord`, and hands the slice to the viewer.

## Responsibilities

- Group beads by `spec_proposal:<stem>` label (proposal filename without `.md`).
- For each group, read the proposal file from `spec/proposals/<stem>.md` to confirm it exists and pull the first-line title.
- Render the link chain: proposal → beads created/closed/modified.

## Interface

```go
type HistoryViewer struct {
    SpecDir string // default "./spec"
    Out     io.Writer
}

// ShowHistory groups the supplied beads by their spec_proposal label and renders.
// beads is typically parsed from "br list --json" (or equivalent) piped on stdin.
func (h *HistoryViewer) ShowHistory(beads []BeadRecord) error

type BeadRecord struct {
    ID            string
    Status        string
    Labels        []string
    Title         string
    CreatedAt     string
    ClosedAt      string
}
```

The viewer does NOT fetch beads itself. That responsibility is the caller's — the `ProposalCommands` cobra handler for `spex log` reads stdin, JSON-decodes into `[]BeadRecord`, and invokes `ShowHistory`.

That split is declared, not just described: the api `spex log` names both ProposalCommands and HistoryViewer in its `provided_by` array. HistoryViewer is half of one external surface rather than a component that happens to be called by another — the parse belongs to the command, the rendering belongs here, and neither half is `spex log` on its own. The other two surfaces this module exposes, `spex register` and `spex template`, pair ProposalCommands with Registrar and TemplateProvider the same way.

## Why Accept Parsed Data

The proposal-level requirement "No runtime subprocesses" (project req `58ea35f52b86`) forbids `exec.Command` inside the spex binary. The previous `CLIBeadLister` type did `br list --json` as a subprocess; it is retired. The viewer is a pure function over data it's given.

This also makes the viewer trivial to test: feed a fixture `[]BeadRecord`, assert the rendered output.

## Output Format

```
2026-02-23-spex-machina.md (project proposal)
  Created: spexmachina-abc (open)     schema: ProjectSchema
  Created: spexmachina-def (open)     validator: SchemaChecker
  ...

2026-04-12-data-flow-contract-layer.md (change proposal)
  Created: spexmachina-123 (closed)   merkle: DiffCommand
  Closed:  spexmachina-old (closed)   apply: BeadCreator
  ...
```

## Label Parsing

Each bead's `spec_proposal:<stem>` label determines its group. Beads without that label are skipped (not part of any proposal). If a bead has multiple `spec_proposal:` labels, the first is used — that shouldn't happen in practice, but is handled defensively.

## Missing Proposal Files

If a bead's `spec_proposal` label references a proposal file that doesn't exist in `spec/proposals/`, the viewer renders:

```
<unknown-proposal>.md (proposal file missing)
  Created: ...
```

rather than erroring. The bead's provenance is still visible; the user can investigate whether the proposal file was renamed, moved, or never committed.

## JSON Output Mode

`spex log --json` flag: emit the grouped structure as machine-readable JSON:

```json
{
  "proposals": [
    {
      "filename": "2026-04-18-decouple-spex-from-br.md",
      "title": "Decouple spex binary from br/bd",
      "beads": [
        {"id": "spexmachina-abc", "status": "open", "action": "created", "summary": "emit: ChangesetBuilder"},
        ...
      ]
    }
  ]
}
```

Useful for piping into other tools. Not the default (human-readable text is default).

## Non-Responsibilities

- Does not reach into the tracker — the caller supplies bead data.
- Does not modify beads or labels.
- Does not create proposal files — that's Registrar.
