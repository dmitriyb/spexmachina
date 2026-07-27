# EmitCommand

CLI entry point for `spex emit`. Reads the impact report (stdin or `--impact`), loads the mapping store and spec graph, wires the builder components, writes `changeset.json` to stdout or `--out`.

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

## Declared surface

`spex emit` is this module's only external entry point, declared as an api node in `spec/emit/module.json` with `provided_by` naming EmitCommand. The declared name is the invocation string alone — the four flags above are not part of it.

That boundary decides what the pipeline can see. Renaming the subcommand changes the api's identity hash and so reads as a removal plus an addition, and the removal-time name check then reports every place in the spec corpus that still says `spex emit`. Changing a flag — adding one, renaming one, changing what `--out` accepts — moves no name and no hash, and the diff cannot see it. Flag-level contract changes are therefore documented here, in the table above, and are caught by review rather than by the tool.

## Pre-flight

Before running the builder:

1. Validate `--git-head` matches `^[0-9a-f]{7,40}$`.
2. Parse the impact report JSON; reject if `errors` array is non-empty (consistent with impact's own gate).
3. Load `.bead-map.json` via `mapping.Store`.
4. Load the spec graph rooted at `spec/project.json`.

## Wiring

```
EmitCommand
  ├─ impact report (stdin or file)
  ├─ mapping.Store
  ├─ spec.Graph
  └─ ChangesetBuilder{
      ├─ Resolver{specGraph, mappingStore, batch}
      ├─ TopologicalSorter{}
      └─ IdempotencyLabeler{mappingStore}
    }
```

EmitCommand composes the builder, invokes `Build(report)`, serializes the returned `Changeset` with canonical JSON encoding, writes to the configured sink.

## Exit Codes

- `0` — success; changeset written.
- `1` — input validation error (bad flags, malformed JSON, impact report carries errors). Stderr carries the structured error.
- `2` — builder error (cycle detected, unresolvable parent). Stderr carries the error with spec_node_ids implicated.

Failure modes never write a partial changeset.

## Composability

- stdin input + stdout output makes `spex impact ... | spex emit ...` pipeline-friendly.
- `--out` + a specific path lets callers capture the changeset for git review before handing to the adapter.

## Non-Responsibilities

- Does not run the adapter.
- Does not update `.bead-map.json` or save a snapshot — those belong to ingest.
- Does not invoke git — `--git-head` is caller-supplied.

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
