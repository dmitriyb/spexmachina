# EmitCommand

CLI entry point for `spex emit`. Reads the impact report (stdin or `--impact`), folds the task journal and loads the spec graph, wires the builder components, writes `changeset.json` to stdout or `--out`.

## Usage

```
spex emit --proposal <ref> --git-head <sha> [--impact <file>] [--out <file>]
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--proposal` | yes | Proposal ref (filename stem, e.g., `2026-04-18-decouple-spex-from-br`). Embedded in the changeset and used for the proposal epic's title. |
| `--git-head` | yes | Git HEAD SHA. Threaded into changeset.json as `git_head`; used by the adapter for `commit:<HEAD>` labels on obsoleted beads. |
| `--impact`   | no (stdin default) | Path to impact report JSON. If omitted, reads from stdin. |
| `--out`      | no (stdout default) | Path to write changeset.json. If omitted, writes to stdout. |

The root's persistent `--spec-dir` is read too: it is what the spec graph is loaded from and where the task journal (`<spec-dir>/.history.jsonl`) lives. The retired `--map` flag went with the file it pointed at.

## Declared surface

`spex emit` is this module's only external entry point, declared as [[7cccc4a96101|an api node]] in `spec/emit/module.json` with `provided_by` naming EmitCommand. The declared name is the invocation string alone — the flags above are not part of it.

That boundary decides what the pipeline can see. Renaming the subcommand changes the api's identity hash and so reads as a removal plus an addition, and the removal-time name check then reports every place in the spec corpus that still says `spex emit`. Changing a flag — adding one, renaming one, changing what `--out` accepts — moves no name and no hash, and the diff cannot see it. Flag-level contract changes are therefore documented here, in the table above, and are caught by review rather than by the tool.

## Pre-flight

Before running the builder:

1. Require `--proposal`, and require `--git-head` to match `^[0-9a-f]{7,40}$`.
2. Parse the impact report JSON; reject if `errors` array is non-empty (consistent with impact's own gate). The gate is re-applied here rather than trusted upstream, so a stale report piped in from an earlier run cannot slip past.
3. Parse and fold the task journal at `<spec-dir>/.history.jsonl` — an absent journal folds empty.
4. Load the spec graph rooted at the spec directory's `project.json`.

## Wiring

EmitCommand assembles the run and then gets out of the way. It gathers five things — the impact report from stdin or `--impact`, the journal fold, the spec graph rooted at the spec directory, the `--git-head` SHA and the `--proposal` ref — and hands all five to [[7f06f7d80e94|ChangesetBuilder]], which owns everything from there.

The three subordinate components — Resolver, TopologicalSorter and IdempotencyLabeler — are reached only through the builder. EmitCommand neither builds nor calls them, which is why this module's `uses` graph runs command → builder → the three rather than command → all four: there is exactly one place a change to the composition has to be made.

Once the builder answers, the command has one job left: serialize the changeset in canonical form and write it to the configured sink.

## Exit Codes

- `0` — success; changeset written.
- `1` — input validation error (bad flags, malformed JSON, impact report carries errors). Stderr names the flag or the input that failed.
- `2` — builder error (cycle detected, unresolvable parent). Stderr carries the error with spec_node_ids implicated.

Failure modes never write a partial changeset.

## Composability

- stdin input + stdout output makes `spex impact ... | spex emit ...` pipeline-friendly.
- `--out` + a specific path lets callers capture the changeset for git review before handing to the adapter.
- `--out` writes atomically: the changeset lands beside the target and is moved into place only once it is complete. A failure part-way through leaves no half-written file for the adapter to pick up, and the target path holds either the previous run's changeset or the new one, never a splice of the two.

## Non-Responsibilities

- Does not run the adapter.
- Does not append to the journal or save a snapshot — those belong to ingest.
- Does not invoke git — `--git-head` is caller-supplied.

Those three absences are one property: [[aa2375420738|emit is a pure function of the files and flags it is handed]]. It starts no subprocess, opens no connection and asks no tracker anything, so the same impact report, journal, spec directory, proposal ref and SHA produce the same bytes on every machine and at every hour.

## Test surface

EmitCommand's CLI-level tests (flag validation, stdin/stdout wiring, exit
codes, impact-report-with-errors rejection) live in the `Emit command
tests` test_section and ship with this component's implementation bead.
The four-component composition that produces the changeset itself
(ChangesetBuilder + Resolver + TopologicalSorter + IdempotencyLabeler) is
covered by the consolidated `test_changeset_builder` test_section, which
names all four in its `describes` array — EmitCommand wires that
composition but the cross-component integration assertions live with the
builder tests, not here.
