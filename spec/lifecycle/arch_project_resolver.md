# ProjectResolver

The single pre-flight every subcommand calls before touching project state. It answers one question — *where does this project's derived state live, and is it usable?* — and it answers it in exactly one place, so no reader of the snapshot or the journal re-implements the rule. Before this component existed, "there is no snapshot" was read as an empty baseline by one consumer and as "no cycle has completed" by another, and nothing kept the two aligned.

## Responsibilities

- Resolve the snapshot and journal locations for the project, per [[44b8c5de0d37|Project state directory]]: the `.spex/` state directory is the home of everything the tool writes, and the only layout there is — authored content under `spec/` is never resolved here, because a person, not the tool, owns it.
- Distinguish the two absences, per [[055444aee4c7|Uninitialized project is an error]], and refuse rather than guess. The failure mode is asymmetric and the answers must be too:
  - no `.spex/` — never initialised; the error names `spex init`;
  - `.spex/` present but the snapshot or journal missing or unparseable — broken; the error names `spex doctor`. A user with a bad merge who is told "run init" re-initialises and destroys the journal, which is why these two messages may never collapse into one.

There is no fallback to the pre-lifecycle in-spec state paths and no layout reporting: those paths are retired vocabulary, dropped before any release carried them, so the resolver has exactly one place to look and absence has exactly one meaning per branch above.

## Observable behaviour

- On success, the caller receives a project context: the resolved snapshot location and the resolved journal location. Callers thread these locations through; no other component computes a state path.
- On refusal, the process exits with the stable, documented not-a-spex-project code — distinct from the input-error and invariant exit codes — so CI and scripts branch on the code rather than grepping the message.
- Resolution is read-only. No invocation of the resolver creates or repairs anything; the commands that write state are `spex init` (creation) and the pipeline's own writers (updates), and repair is a human decision informed by `spex doctor`.

## Rationale

The resolver, not the snapshot loader, is where absence is interpreted. The merkle module's `SnapshotStore.Load` errors on an absent file and offers no fallback; the canonical empty tree it once returned survives only as the seed `InitCommand` writes. Keeping the interpretation here means a future subcommand gets the whole contract — both absences and the exit code — by calling the pre-flight, instead of remembering to handle a missing file correctly.
