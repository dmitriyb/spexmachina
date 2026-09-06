# Lifecycle command tests

Acceptance coverage spanning [[f64995aaeb56|InitCommand]], [[3accd139a00d|DoctorCommand]] and, through the pre-flight both run, [[a9aa93774cc2|ProjectResolver]]: what init writes, what init refuses, what doctor reports, and the round-trip between them. All cases run in temporary directories.

## Setup

Fixtures: a fresh empty directory; a directory already initialised by `spex init`; an initialised directory subsequently damaged (snapshot deleted, or journal replaced with malformed bytes).

## Cases

- **Init seeds the empty tree, not the current spec**: run `spex init` in a directory that already contains a populated `spec/` tree. The written snapshot must encode the canonical empty tree — compare against the merkle module's empty-tree encoding, not against a snapshot of the present spec. This is the check that can actually fail in the dangerous direction: a snapshot seeded from the current spec makes the first diff clean and no work is ever born from the initial spec.
- **Init writes an empty journal, no init event**: after `spex init`, the journal file exists and contains zero lines. Any event written at birth would make "no cycle has completed" permanently false.
- **Init refuses an initialised directory**: `spex init` where `.spex/` exists → non-zero exit, and every byte under `.spex/` is unchanged — the journal survives.
- **Doctor on a healthy project**: after `spex init`, `spex doctor` reports every artifact present and readable, exit 0 — the init → doctor round-trip that spans both commands.
- **Doctor names the fix, per finding**: on the damaged fixtures, `spex doctor` lists each missing or unreadable artifact together with the command that would fix it; a missing `.spex/` names `spex init`, a damaged file inside `.spex/` does not.
- **Doctor never repairs**: run `spex doctor` against every damaged fixture; afterwards the directory is byte-identical to before. No flag, no mode, no exception mints or moves a baseline.

## Edge cases

- `spex doctor` in an uninitialised directory: reports the project as never initialised, names `spex init`, and exits with the not-a-spex-project code rather than crashing on absent files.
- `spex init` in a directory carrying state files at the retired pre-lifecycle in-spec locations but no `.spex/`: proceeds as in any uninitialised directory and writes nothing outside `.spex/`. The retired paths are not a layout and are neither read nor migrated; nothing shadows anything, because nothing ever resolves them.
