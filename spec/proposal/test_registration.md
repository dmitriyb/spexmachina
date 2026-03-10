# Registration Tests

Tests for the Registrar component: proposal file copying, section validation, naming conventions, and git-native constraints.

## Setup

Temporary directory structure created per test:

```
tmpdir/
  spec/
    proposals/          # initially empty target directory
  input/
    valid-project.md    # project proposal with all required sections
    valid-change.md     # change proposal with all required sections
    partial-project.md  # project proposal missing "Design decisions"
    partial-change.md   # change proposal missing "Impact expectation"
    empty.md            # empty file
    no-headings.md      # prose with no H2 headings
    mixed-case.md       # required sections with inconsistent casing (e.g., "## VISION", "## key Requirements")
    already-dated.md    # file named "2026-05-10-caching.md" following the date convention
    unicode-title.md    # H1 title with unicode characters (e.g., "# Umlauts: Ubersicht")
```

`valid-project.md` contains all four required project sections (Vision, Modules, Key requirements, Design decisions) with substantive placeholder content under each heading.

`valid-change.md` contains all three required change sections (Context, Proposed change, Impact expectation) with substantive placeholder content.

## Scenarios

### S1: Register a valid project proposal

**Given** `valid-project.md` with all four required H2 sections.
**When** `Register(ctx, "input/valid-project.md", "spec")` is called.
**Then:**
- File is copied to `spec/proposals/YYYY-MM-DD-<slug>.md` where YYYY-MM-DD is today's date.
- The slug is derived from the H1 heading of the proposal (lowercased, spaces replaced with hyphens, non-alphanumeric characters stripped).
- The copied file's content is byte-for-byte identical to the source.
- File permissions on the copy are 0644.
- Function returns nil error.

### S2: Register a valid change proposal

**Given** `valid-change.md` with all three required H2 sections.
**When** `Register(ctx, "input/valid-change.md", "spec")` is called.
**Then:**
- File is copied to `spec/proposals/YYYY-MM-DD-<slug>.md`.
- Proposal type is detected as "change" (because it contains `## Proposed change` and not `## Vision`).
- Function returns nil error.

### S3: Reject project proposal with missing sections

**Given** `partial-project.md` containing `## Vision`, `## Modules`, `## Key requirements` but missing `## Design decisions`.
**When** `Register(ctx, "input/partial-project.md", "spec")` is called.
**Then:**
- Function returns an error.
- Error message includes the name of every missing section ("Design decisions").
- No file is written to `spec/proposals/`.

### S4: Reject change proposal with missing sections

**Given** `partial-change.md` containing `## Context`, `## Proposed change` but missing `## Impact expectation`.
**When** `Register(ctx, "input/partial-change.md", "spec")` is called.
**Then:**
- Function returns an error.
- Error message includes "Impact expectation".
- No file is written to `spec/proposals/`.

### S5: Report all missing sections, not just the first

**Given** a file containing only `## Vision` (missing Modules, Key requirements, Design decisions).
**When** `Register(ctx, "input/one-section.md", "spec")` is called.
**Then:**
- Error lists all three missing sections: "Modules", "Key requirements", "Design decisions".
- The error uses `errors.Join` or equivalent multi-error aggregation so each missing section is individually inspectable.

### S6: Preserve existing date-prefixed filename

**Given** `already-dated.md` is a valid change proposal file named `2026-05-10-caching.md`.
**When** `Register(ctx, "input/2026-05-10-caching.md", "spec")` is called.
**Then:**
- File is copied to `spec/proposals/2026-05-10-caching.md` preserving the original filename.
- The registrar does not prepend a second date.

### S7: Generate slug from H1 heading when filename lacks date prefix

**Given** `valid-project.md` with H1 heading `# Project Proposal: Add Caching Layer`.
**When** `Register(ctx, "input/valid-project.md", "spec")` is called.
**Then:**
- Target filename is `YYYY-MM-DD-add-caching-layer.md` (date is today, slug derived from the H1 after stripping the "Project Proposal:" prefix).
- Slug generation strips common prefixes ("Project Proposal:", "Change Proposal:"), lowercases, replaces spaces with hyphens, and removes non-alphanumeric/non-hyphen characters.

### S8: Case-insensitive section matching

**Given** `mixed-case.md` with headings `## VISION`, `## modules`, `## key Requirements`, `## design Decisions`.
**When** `Register(ctx, "input/mixed-case.md", "spec")` is called.
**Then:**
- All four required sections are matched case-insensitively.
- Registration succeeds.
- Function returns nil error.

### S9: Source file does not exist

**Given** no file at `input/nonexistent.md`.
**When** `Register(ctx, "input/nonexistent.md", "spec")` is called.
**Then:**
- Function returns an error wrapping the underlying filesystem error.
- No file is written to `spec/proposals/`.

### S10: Target proposals directory does not exist

**Given** a valid proposal file but `spec/proposals/` directory has not been created.
**When** `Register(ctx, "input/valid-project.md", "spec")` is called.
**Then:**
- The registrar creates `spec/proposals/` (with permissions 0755) before copying.
- Registration succeeds.

## Edge Cases

### E1: Empty file

**Given** `empty.md` is a zero-byte file.
**When** `Register(ctx, "input/empty.md", "spec")` is called.
**Then:**
- Function returns an error indicating the proposal type cannot be detected (no H2 headings found).
- Error message is descriptive: "proposal: cannot detect type from headings".

### E2: File with H2 headings but no recognizable type

**Given** `no-headings.md` containing `## Introduction` and `## Conclusion` (neither "Vision" nor "Proposed change").
**When** `Register(ctx, "input/no-headings.md", "spec")` is called.
**Then:**
- Function returns an error: "proposal: cannot detect type from headings".
- No file is written.

### E3: Duplicate registration

**Given** `valid-change.md` is a valid change proposal. `spec/proposals/` already contains a file with the same target name (e.g., `2026-03-10-some-change.md`).
**When** `Register` is called twice for the same file on the same day.
**Then:**
- Second call returns an error indicating the target file already exists.
- The existing file is not overwritten.

### E4: Proposal containing both project and change markers

**Given** a file with both `## Vision` and `## Proposed change` headings.
**When** `Register` is called.
**Then:**
- Proposal type is detected as "project" (the `## Vision` check takes precedence as defined in `detectType`).
- Validation checks project-required sections.

### E5: Very large proposal file

**Given** a valid project proposal that is 10 MB in size (large embedded diagrams or data tables).
**When** `Register` is called.
**Then:**
- File is copied successfully. Section detection operates only on lines starting with `## `, so performance is proportional to line count, not file size.
- No artificial file size limit is imposed.

### E6: Proposal with extra sections beyond required ones

**Given** a project proposal with all four required sections plus additional sections like `## Timeline`, `## Open questions`, `## Appendix`.
**When** `Register` is called.
**Then:**
- Registration succeeds. Validation checks only that required sections are present; extra sections are ignored.
