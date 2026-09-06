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
    partial-project.md  # project proposal missing "Design decisions"
```

`valid-project.md` contains all four required project sections (Vision, Modules, Key requirements, Design decisions) with substantive placeholder content under each heading.

Every `Register` call carries the fixture git head `cafe1234` — the caller-supplied head that
feeds the registered event's eid; scenario prose omits the argument for brevity. The temp project
starts freshly initialised: an empty journal, nothing registered.

## Scenarios

### S1: Register a valid project proposal

**Given** `valid-project.md` with all four required H2 sections.
**When** `Register("input/valid-project.md", "spec")` is called.
**Then:**
- File is copied to `spec/proposals/YYYY-MM-DD-<slug>.md` where YYYY-MM-DD is today's date.
- The slug is derived from the H1 heading of the proposal (lowercased, spaces replaced with hyphens, non-alphanumeric characters stripped).
- The copied file's content is byte-for-byte identical to the source.
- File permissions on the copy are 0644 (`copyFile` opens with that mode; no test asserts it).
- The journal gains exactly one line: a `registered` event with
  `eid: "cafe1234:<slug>"`, `proposal: "<slug>"` (the copied file's stem), `git_head: "cafe1234"`.
- Function returns nil error.

### S3: Reject project proposal with missing sections

**Given** `partial-project.md` containing `## Vision`, `## Modules`, `## Key requirements` but missing `## Design decisions`.
**When** `Register("input/partial-project.md", "spec")` is called.
**Then:**
- Function returns an error.
- Error message includes the name of every missing section ("Design decisions").
- No file is written to `spec/proposals/`, and no journal line is appended — a refusal leaves
  neither mark.

## Edge Cases

### E3b: Crash recovery — event appended, file not copied

**Given** a journal already holding the `registered` event a prior interrupted run appended
(`eid: "cafe1234:<slug>"`), and no file in `spec/proposals/`.
**When** `Register` is called for the same proposal with the same git head.
**Then:**
- The append finds its eid already present and adds nothing — the journal still holds exactly
  one `registered` line for the slug.
- The copy lands normally and the function returns nil error. No ordering leaves a registered
  file without its event.

