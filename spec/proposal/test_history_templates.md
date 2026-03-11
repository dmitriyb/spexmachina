# History and Template Tests

Tests for the HistoryViewer (component 2) and TemplateProvider (component 3). HistoryViewer lists proposals and their linked spec changes and bead actions. TemplateProvider outputs project/change proposal templates to stdout.

## Setup

### HistoryViewer setup

Temporary directory with pre-populated proposals and a mock bead CLI:

```
tmpdir/
  spec/
    proposals/
      2026-02-23-spex-machina.md      # project proposal
      2026-03-01-add-caching.md        # change proposal
      2026-03-05-refactor-validator.md  # change proposal
```

Mock `br list --json` output returns beads with `metadata.spec_proposal` fields:

```json
[
  {"id": "spexmachina-abc", "title": "ProjectSchema", "status": "done",
   "metadata": {"spec_proposal": "2026-02-23-spex-machina.md", "module": "schema", "component": "ProjectSchema", "action": "created"}},
  {"id": "spexmachina-def", "title": "SchemaChecker", "status": "done",
   "metadata": {"spec_proposal": "2026-02-23-spex-machina.md", "module": "validator", "component": "SchemaChecker", "action": "created"}},
  {"id": "spexmachina-ghi", "title": "CacheLayer", "status": "in_progress",
   "metadata": {"spec_proposal": "2026-03-01-add-caching.md", "module": "merkle", "component": "CacheLayer", "action": "created"}},
  {"id": "spexmachina-jkl", "title": "SnapshotStore", "status": "done",
   "metadata": {"spec_proposal": "2026-03-01-add-caching.md", "module": "merkle", "component": "SnapshotStore", "action": "modified"}},
  {"id": "spexmachina-mno", "title": "DagChecker", "status": "ready",
   "metadata": {"spec_proposal": "2026-03-05-refactor-validator.md", "module": "validator", "component": "DagChecker", "action": "review"}},
  {"id": "spexmachina-pqr", "title": "Unlinked task", "status": "ready",
   "metadata": {}}
]
```

### TemplateProvider setup

No filesystem setup needed. TemplateProvider writes to an `io.Writer` (a `bytes.Buffer` in tests).

## Scenarios

### HistoryViewer Scenarios

#### S1: List all proposals in date order

**Given** three proposals in `spec/proposals/` as described in setup.
**When** `ShowHistory(ctx, "spec", &buf)` is called with default (human-readable) output.
**Then:**
- Output lists proposals in filename order (which is chronological due to date prefix).
- First entry: `2026-02-23-spex-machina.md`
- Second entry: `2026-03-01-add-caching.md`
- Third entry: `2026-03-05-refactor-validator.md`
- Each entry is followed by its linked beads.

#### S2: Show linked bead actions per proposal

**Given** the bead data from setup where two beads are tagged with `2026-02-23-spex-machina.md`.
**When** `ShowHistory(ctx, "spec", &buf)` is called.
**Then** output for the first proposal includes:
```
2026-02-23-spex-machina.md (project proposal)
  Created: spexmachina-abc (schema: ProjectSchema)
  Created: spexmachina-def (validator: SchemaChecker)
```
Each bead line shows the action (Created/Modified/Review), bead ID, module, and component.

#### S3: Proposal with no linked beads

**Given** proposals directory contains `2026-03-08-future-idea.md` which has no matching beads in the `br list` output.
**When** `ShowHistory(ctx, "spec", &buf)` is called.
**Then:**
- The proposal appears in the listing.
- It shows zero bead lines beneath it (just the proposal filename, no indented children).
- No error is returned.

#### S4: JSON output mode

**Given** three proposals with linked beads as in setup.
**When** `ShowHistory` is called with a JSON output flag.
**Then** output is a valid JSON array:
```json
[
  {
    "proposal": "2026-02-23-spex-machina.md",
    "type": "project",
    "date": "2026-02-23",
    "beads": [
      {"id": "spexmachina-abc", "action": "created", "module": "schema", "component": "ProjectSchema"},
      {"id": "spexmachina-def", "action": "created", "module": "validator", "component": "SchemaChecker"}
    ]
  },
  {
    "proposal": "2026-03-01-add-caching.md",
    "type": "change",
    "date": "2026-03-01",
    "beads": [
      {"id": "spexmachina-ghi", "action": "created", "module": "merkle", "component": "CacheLayer"},
      {"id": "spexmachina-jkl", "action": "modified", "module": "merkle", "component": "SnapshotStore"}
    ]
  },
  {
    "proposal": "2026-03-05-refactor-validator.md",
    "type": "change",
    "date": "2026-03-05",
    "beads": [
      {"id": "spexmachina-mno", "action": "review", "module": "validator", "component": "DagChecker"}
    ]
  }
]
```
- JSON is parseable by `json.Unmarshal`.
- Each proposal record includes `proposal`, `type`, `date`, and `beads` fields.
- Beads without `spec_proposal` metadata (like `spexmachina-pqr`) do not appear in any proposal's bead list.

#### S5: Beads with no spec_proposal metadata are excluded

**Given** bead `spexmachina-pqr` has empty metadata (no `spec_proposal` field).
**When** `ShowHistory` is called.
**Then:**
- `spexmachina-pqr` does not appear under any proposal.
- No error is raised for beads without proposal metadata.

#### S6: Empty proposals directory

**Given** `spec/proposals/` exists but contains no `.md` files.
**When** `ShowHistory(ctx, "spec", &buf)` is called.
**Then:**
- Output is empty (human-readable) or an empty JSON array `[]` (JSON mode).
- Function returns nil error.

#### S7: Bead CLI unavailable

**Given** the `br` binary is not on `$PATH` and no fallback is configured.
**When** `ShowHistory(ctx, "spec", &buf)` is called.
**Then:**
- Function returns an error wrapping the exec failure.
- Error message indicates the bead CLI could not be executed.
- Proposals are still listed (the filesystem scan succeeds), but bead linking fails.

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
- Bead matching still works using the full filename stem as the `spec_proposal` metadata value.

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
