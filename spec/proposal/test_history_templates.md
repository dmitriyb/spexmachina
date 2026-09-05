# History and Template Tests

Tests for the HistoryViewer (component 2) and TemplateProvider (component 3). HistoryViewer lists proposals and their linked spec changes and task actions. TemplateProvider outputs project/change proposal templates to stdout.

## Setup

### HistoryViewer setup

Temporary directory with pre-populated proposals. There is no mock tracker CLI: `ShowHistory` takes parsed task records and starts no process (`proposal/history.go:71`).

```
tmpdir/
  spec/
    proposals/
      2026-02-23-spex-machina.md      # project proposal
      2026-03-01-add-caching.md        # change proposal
      2026-03-05-refactor-validator.md  # change proposal
```

Task records carry a `spec_proposal:<stem>` entry in their `labels` array:

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
**When** `ShowHistory(tasks)` is called on a viewer with `JSON: false`.
**Then:**
- Output lists one entry per proposal named by a `spec_proposal:` label on those tasks, in ascending stem order — chronological here because of the date prefix. A proposal with no labelled task is not listed; the proposals directory is never scanned.
- First entry: `2026-02-23-spex-machina.md`
- Second entry: `2026-03-01-add-caching.md`
- Third entry: `2026-03-05-refactor-validator.md`
- Each entry is followed by its linked tasks.

#### S2: Show linked task actions per proposal

**Given** the task data from setup, where two tasks carry `spec_proposal:2026-02-23-spex-machina`.
**When** `ShowHistory(tasks)` is called on a viewer with `JSON: false`.
**Then** output for the first proposal includes:
```
2026-02-23-spex-machina.md (project proposal)
  Closed: spexmachina-abc (closed)	ProjectSchema
  Closed: spexmachina-def (closed)	SchemaChecker
```
Each task line is `"  %s: %s (%s)\t%s\n"` — action, task ID, the task's status in parentheses, then a tab and the task's title as summary (`proposal/history.go:229-231`). Action is title-cased from `deriveAction`, which yields only `created` or `closed` (`:201-206`); there is no `Modified` or `Review`. Neither module nor component appears.

#### S3: Proposal with no linked tasks

**Given** `2026-03-08-future-idea.md` exists under `spec/proposals/` and no supplied task carries `spec_proposal:2026-03-08-future-idea`.
**When** `ShowHistory(tasks)` is called.
**Then:**
- The proposal does NOT appear: the listing is driven by tasks' `spec_proposal:` labels, not by a scan of `spec/proposals/`.
- No error is returned.

#### S4: JSON output mode

**Given** three proposals with linked tasks as in setup.
**When** `ShowHistory` is called with a JSON output flag.
**Then** output is the envelope `{"proposals": [...]}`, each entry carrying `filename`, `title`, `date` and a `tasks` array of `{id, status, action, summary}`:
```json
{
  "proposals": [
    {
      "filename": "2026-02-23-spex-machina.md",
      "title": "Project Proposal: Spex Machina",
      "date": "2026-02-23",
      "tasks": [
        {"id": "spexmachina-abc", "status": "closed", "action": "closed", "summary": "ProjectSchema"},
        {"id": "spexmachina-def", "status": "closed", "action": "closed", "summary": "SchemaChecker"}
      ]
    },
    {
      "filename": "2026-03-01-add-caching.md",
      "title": "Change Proposal: Add caching",
      "date": "2026-03-01",
      "tasks": [
        {"id": "spexmachina-ghi", "status": "in_progress", "action": "created", "summary": "CacheLayer"},
        {"id": "spexmachina-jkl", "status": "closed", "action": "closed", "summary": "SnapshotStore"}
      ]
    },
    {
      "filename": "2026-03-05-refactor-validator.md",
      "title": "Change Proposal: Refactor validator",
      "date": "2026-03-05",
      "tasks": [
        {"id": "spexmachina-mno", "status": "open", "action": "created", "summary": "DagChecker"}
      ]
    }
  ]
}
```
- JSON is parseable by `json.Unmarshal`.
- Each proposal record includes `filename`, `title`, `date` and `tasks`; there is no `proposal` or `type` key, and a task entry carries `id`, `status`, `action` and `summary`, not `module`/`component`. The array is keyed `tasks`: the envelope speaks the corpus vocabulary, and the retired key is not emitted as an alias.
- `date` is the filename's `YYYY-MM-DD` prefix, and nothing else supplies it.
- `action` is derived from status and takes only two values — `closed` for a closed task, `created` for anything else (`proposal/history.go:201-206`).
- Groups come out in ascending stem order (`proposal/history.go:78`), and `title` is the proposal file's H1, empty when the file is missing.
- Tasks with no `spec_proposal:` label (like `spexmachina-pqr`) do not appear in any proposal's task list.

#### S5: Tasks with no spec_proposal label are excluded

**Given** task `spexmachina-pqr` has an empty `labels` array (no `spec_proposal:` entry).
**When** `ShowHistory` is called.
**Then:**
- `spexmachina-pqr` does not appear under any proposal.
- No error is raised for tasks without a `spec_proposal:` label; the grouping step skips them (`proposal/history.go:125-134`).

#### S6: No task carries a spec_proposal label

**Given** no supplied task carries a `spec_proposal:` label.
**When** `ShowHistory(tasks)` is called.
**Then:**
- Output is empty (human-readable) or `{"proposals": []}` (JSON mode).
- Function returns nil error.

#### S7: No tracker subprocess is ever started

**Given** `$PATH` is emptied so any `exec.Command` would fail.
**When** `ShowHistory(tasks)` is called.
**Then:**
- Function returns nil and renders normally — `HistoryViewer` takes parsed task records and runs no external command.

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

**Given** `spec/proposals/` contains `notes.txt`, `diagram.png`, and `2026-02-23-spex-machina.md`, and the supplied tasks carry only `spec_proposal:2026-02-23-spex-machina`.
**When** `ShowHistory(tasks)` is called.
**Then:**
- Output contains one proposal entry, `2026-02-23-spex-machina.md`. The other files are never consulted: the listing is driven by the tasks' labels, and only `<stem>.md` for each labelled stem is read.

### E2: Proposal filename does not follow date convention

**Given** `spec/proposals/` contains `random-notes.md` (no date prefix), and a supplied task carries `spec_proposal:random-notes`.
**When** `ShowHistory(tasks)` is called.
**Then:**
- The file is still listed as a proposal.
- The `date` field in JSON output is the empty string: no prefix, no date — file modification time is never consulted.
- Task matching still works using the full filename stem as the `spec_proposal:` label value; `firstProposalStem` also tolerates a trailing `.md` (`proposal/history.go:139-152`).

### E4: Template output is deterministic

**Given** no preconditions.
**When** `Template("project", &buf1)` and `Template("project", &buf2)` are called sequentially.
**Then:**
- `buf1.String() == buf2.String()` (byte-for-byte identical).
- Templates are embedded constants, so output never varies.

### E5: Very large number of proposals

**Given** `spec/proposals/` contains 500 `.md` files and 2000 task records are supplied, labelled across those proposals.
**When** `ShowHistory(tasks)` is called.
**Then:**
- Function completes without error.
- Grouping is done in-memory over the supplied records; nothing is fetched.
- Output correctly groups tasks by proposal.

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
