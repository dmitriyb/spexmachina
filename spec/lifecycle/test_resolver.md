# Resolver tests

Integration coverage for [[a9aa93774cc2|ProjectResolver]]: resolution of the `.spex/` state directory and the two typed absences. All cases run against temporary project directories built per case; no test touches this repository's own state.

## Setup

Each case constructs a directory in one of three states:

- **initialised**: `.spex/` present, holding a parseable snapshot and journal
- **uninitialised**: no `.spex/`
- **broken**: `.spex/` present, with one of its files missing or unparseable

## Cases

- **Initialised resolves**: state *initialised* → a project context whose snapshot and journal locations are under `.spex/`.
- **Only `.spex/` is consulted**: state *uninitialised* with state files planted at the retired pre-lifecycle in-spec locations → still the uninitialised refusal. No fallback resolves them and nothing reads them: the retired paths are not a layout, and a resolver that honoured them would give absence two meanings again.
- **Uninitialised is init's error**: state *uninitialised* → typed error naming `spex init`, carrying the stable not-a-spex-project exit code — distinct from the input-error and invariant codes.
- **Broken is doctor's error**: state *broken* with the snapshot deleted → typed error naming `spex doctor`, never `spex init`. Repeat with the journal truncated to malformed bytes: same class of error. The asymmetry is the point under test — a resolver that answers "run init" here invites a user to destroy a journal.

## Edge cases

- `.spex/` exists but is empty → broken, not uninitialised: the directory's presence is the initialisation marker.
- `.spex/` is a file, not a directory → broken, error names `spex doctor`.
- Resolution is read-only: after any case above, the directory's contents are byte-identical to its setup state.
