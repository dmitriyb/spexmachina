# History and Template Tests

Tests for the HistoryViewer (component 2) and TemplateProvider (component 3). HistoryViewer lists proposals and their linked spec changes and bead actions. TemplateProvider outputs project/change proposal templates to stdout.

## Setup

### HistoryViewer setup

Temporary directory with pre-populated proposals. There is no mock bead CLI: `ShowHistory` takes parsed `[]BeadRecord` and starts no process (`proposal/history.go:71`).

```
tmpdir/
  spec/
    proposals/
      2026-02-23-spex-machina.md      # project proposal
      2026-03-01-add-caching.md        # change proposal
      2026-03-05-refactor-validator.md  # change proposal
```

Bead records carry a `spec_proposal:<stem>` entry in their `labels` array:

```json
[
  {"id": "spexmachina-abc", "title": "ProjectSchema", "status": "closed",
   "labels": ["spec_proposal:2026-02-23-spex-machina"]},
  {"id": "spexmachina-def", "title": "SchemaChecker", "status": "closed",
   "labels": ["spec_proposal:2026-02-23-spex-machina"]},
  {"id": "spexmachina-ghi", "title": "CacheLayer", "status": "in_progress",
   "labels": ["spec_proposal:2026-03-01-add-caching"]},
  {"id": "spexmachina-jkl", "title": "SnapshotStore", "status": "closed",
   "labels": ["spec_proposal:2026-03-01-add-caching"]},
  {"id": "spexmachina-mno", "title": "DagChecker", "status": "open",
   "labels": ["spec_proposal:2026-03-05-refactor-validator"]},
  {"id": "spexmachina-pqr", "title": "Unlinked task", "status": "open",
   "labels": []}
]
```

### TemplateProvider setup

No filesystem setup needed. TemplateProvider writes to an `io.Writer` (a `bytes.Buffer` in tests).

## Scenarios

### HistoryViewer Scenarios

#### S1: List all proposals in date order

**Given** three proposals in `spec/proposals/` as described in setup.
**When** `ShowHistory(beads)` is called on a viewer with `JSON: false`.
**Then:**
- Output lists one entry per proposal named by a `spec_proposal:` label on those beads, in ascending stem order — chronological here because of the date prefix. A proposal with no labelled bead is not listed; the proposals directory is never scanned.
- First entry: `2026-02-23-spex-machina.md`
- Second entry: `2026-03-01-add-caching.md`
- Third entry: `2026-03-05-refactor-validator.md`
- Each entry is followed by its linked beads.

#### S2: Show linked bead actions per proposal

**Given** the bead data from setup, where two beads carry `spec_proposal:2026-02-23-spex-machina`.
**When** `ShowHistory(beads)` is called on a viewer with `JSON: false`.
**Then** output for the first proposal includes:
```
2026-02-23-spex-machina.md (project proposal)
  Closed: spexmachina-abc (closed)	ProjectSchema
  Closed: spexmachina-def (closed)	SchemaChecker
```
Each bead line is `"  %s: %s (%s)\t%s\n"` — action, bead ID, the bead's status in parentheses, then a tab and the bead's title as summary (`proposal/history.go:229-231`). Action is title-cased from `deriveAction`, which yields only `created` or `closed` (`:201-206`); there is no `Modified` or `Review`. Neither module nor component appears.

#### S3: Proposal with no linked beads

**Given** `2026-03-08-future-idea.md` exists under `spec/proposals/` and no supplied bead carries `spec_proposal:2026-03-08-future-idea`.
**When** `ShowHistory(beads)` is called.
**Then:**
- The proposal does NOT appear: the listing is driven by beads' `spec_proposal:` labels, not by a scan of `spec/proposals/`.
- No error is returned.

#### S4: JSON output mode

**Given** three proposals with linked beads as in setup.
**When** `ShowHistory` is called with a JSON output flag.
**Then** output is the envelope `{"proposals": [...]}`, each entry carrying `filename`, `title` and a `beads` array of `{id, status, action, summary}`:
```json
{
  "proposals": [
    {
      "filename": "2026-02-23-spex-machina.md",
      "title": "Project Proposal: Spex Machina",
      "beads": [
        {"id": "spexmachina-abc", "status": "closed", "action": "closed", "summary": "ProjectSchema"},
        {"id": "spexmachina-def", "status": "closed", "action": "closed", "summary": "SchemaChecker"}
      ]
    },
    {
      "filename": "2026-03-01-add-caching.md",
      "title": "Change Proposal: Add caching",
      "beads": [
        {"id": "spexmachina-ghi", "status": "in_progress", "action": "created", "summary": "CacheLayer"},
        {"id": "spexmachina-jkl", "status": "closed", "action": "closed", "summary": "SnapshotStore"}
      ]
    },
    {
      "filename": "2026-03-05-refactor-validator.md",
      "title": "Change Proposal: Refactor validator",
      "beads": [
        {"id": "spexmachina-mno", "status": "open", "action": "created", "summary": "DagChecker"}
      ]
    }
  ]
}
```
- JSON is parseable by `json.Unmarshal`.
- Each proposal record includes `filename`, `title` and `beads`; there is no `proposal`, `type` or `date` key, and a bead entry carries `id`, `status`, `action` and `summary`, not `module`/`component`.
- `action` is derived from status and takes only two values — `closed` for a closed bead, `created` for anything else (`proposal/history.go:201-206`).
- Groups come out in ascending stem order (`proposal/history.go:78`), and `title` is the proposal file's H1, empty when the file is missing.
- Beads with no `spec_proposal:` label (like `spexmachina-pqr`) do not appear in any proposal's bead list.

#### S5: Beads with no spec_proposal label are excluded

**Given** bead `spexmachina-pqr` has an empty `labels` array (no `spec_proposal:` entry).
**When** `ShowHistory` is called.
**Then:**
- `spexmachina-pqr` does not appear under any proposal.
- No error is raised for beads without a `spec_proposal:` label; `groupBeadsByProposal` skips them (`proposal/history.go:125-134`).

#### S6: No bead carries a spec_proposal label

**Given** no supplied bead carries a `spec_proposal:` label.
**When** `ShowHistory(beads)` is called.
**Then:**
- Output is empty (human-readable) or `{"proposals": []}` (JSON mode).
- Function returns nil error.

#### S7: No tracker subprocess is ever started

**Given** `$PATH` is emptied so any `exec.Command` would fail.
**When** `ShowHistory(beads)` is called.
**Then:**
- Function returns nil and renders normally — `HistoryViewer` takes parsed `[]BeadRecord` and runs no external command.

### TemplateProvider Scenarios

#### S8: Output project proposal template

**Given** no preconditions.
**When** `Template("project", &buf)` is called.
**Then:**
- `buf` contains the project proposal template.
- Template includes all four required H2 sections: `## Vision`, `## Modules`, `## Key requirements`, `## Design decisions`.
- Template starts with `# Project Proposal: <Project Name>`.
- Each section contains placeholder text (angle-bracket markers like `<Describe the project vision and motivation>`).
- Function returns nil error.

#### S9: Output change proposal template

**Given** no preconditions.
**When** `Template("change", &buf)` is called.
**Then:**
- `buf` contains the change proposal template.
- Template includes all three required H2 sections: `## Context`, `## Proposed change`, `## Impact expectation`.
- Template starts with `# Change Proposal: <Title>`.
- Each section contains placeholder text.
- Function returns nil error.

#### S10: Invalid template type

**Given** no preconditions.
**When** `Template("rfc", &buf)` is called.
**Then:**
- Function returns an error: `proposal: unknown template type: "rfc"`.
- Nothing is written to `buf`.

#### S11: Project template contains all sections needed for registration

**Given** no preconditions.
**When** `Template("project", &buf)` is called and the output is passed through `detectType` and section validation.
**Then:**
- `detectType` identifies the template as type "project".
- All four required sections are found by the section validator.
- A filled-in version of this template (with placeholders replaced by real content) would pass `Register` validation. This confirms that the template and the registrar agree on what sections are required.

#### S12: Change template contains all sections needed for registration

**Given** no preconditions.
**When** `Template("change", &buf)` is called and the output is passed through `detectType` and section validation.
**Then:**
- `detectType` identifies the template as type "change".
- All three required sections are found by the section validator.
- Template and registrar are consistent.

## Edge Cases

### E1: Proposals directory contains non-markdown files

**Given** `spec/proposals/` contains `notes.txt`, `diagram.png`, and `2026-02-23-spex-machina.md`.
**When** `ShowHistory` is called.
**Then:**
- Only `.md` files are listed. Non-markdown files are silently ignored.
- Output contains one proposal entry.

### E2: Proposal filename does not follow date convention

**Given** `spec/proposals/` contains `random-notes.md` (no date prefix).
**When** `ShowHistory` is called.
**Then:**
- The file is still listed as a proposal.
- The `date` field in JSON output is empty or derived from file modification time.
- Bead matching still works using the full filename stem as the `spec_proposal:` label value; `firstProposalStem` also tolerates a trailing `.md` (`proposal/history.go:139-152`).

### E3: Bead CLI returns malformed JSON

**Given** `br list --json` returns invalid JSON (e.g., truncated output).
**When** `ShowHistory` is called.
**Then:**
- Function returns an error wrapping the JSON parse failure.
- Error message includes context about what was being parsed ("bead list output").

### E4: Template output is deterministic

**Given** no preconditions.
**When** `Template("project", &buf1)` and `Template("project", &buf2)` are called sequentially.
**Then:**
- `buf1.String() == buf2.String()` (byte-for-byte identical).
- Templates are embedded constants, so output never varies.

### E5: Very large number of proposals

**Given** `spec/proposals/` contains 500 `.md` files and `br list --json` returns 2000 beads.
**When** `ShowHistory` is called.
**Then:**
- Function completes without error.
- Bead listing is done once (single `br list --json` call). Filtering is done in-memory.
- Output correctly groups beads by proposal.

### E6: Empty string template type

**Given** no preconditions.
**When** `Template("", &buf)` is called.
**Then:**
- Function returns an error: `proposal: unknown template type: ""`.
- Nothing is written to `buf`.

### E7: Concurrent calls to ShowHistory

**Given** two goroutines call `ShowHistory` simultaneously with the same spec directory.
**When** both calls complete.
**Then:**
- Both return nil error.
- Each produces correct output independently. No shared mutable state between calls.
