# Command reference

Every subcommand reads stdin or files, writes stdout or files, and exits with
a documented code. That is what makes the pipeline in
[`architecture.md`](architecture.md) composable rather than merely sequential.

## Global

```
spex [command] [flags]
```

| Flag | Default | Purpose |
|---|---|---|
| `-s`, `--spec-dir <path>` | `spec/` | Path to the spec directory. Accepted by every subcommand |
| `-h`, `--help` | | Help for any command |

Unless a command documents otherwise, **0** means success and **1** means
failure. `spex diff`, `spex emit` and `spex ingest` add a distinct **2**; each
is described below.

---

## The pipeline

### `spex validate`

Validates the spec directory: schema conformance, content path resolution,
link resolution, ID uniqueness, ID derivation, DAG acyclicity, name
consistency, test coverage, requirement coverage, coupled sections. The
removed-name check is not here — it runs in `spex diff`, which has the
classified changes it needs.

Writes a JSON report to stdout (`{"valid":…, "error_count":…, "warning_count":…, "errors":[…]}`),
colorized when stdout is a terminal. The exit status is read off the report
that was just serialized, so it can never disagree with the `valid` field the
caller sees.

```sh
spex validate
```

| Exit | Meaning |
|---|---|
| 0 | `valid: true` |
| 1 | validation failed, or the spec directory could not be read |

### `spex diff`

Rebuilds the merkle tree and compares it against the snapshot.

| Flag | Default | Purpose |
|---|---|---|
| `--json` | off | Emit JSON instead of the human summary |
| `--snapshot <path>` | `<spec-dir>/.snapshot.json` | Snapshot to compare against |

A missing snapshot is treated as the empty tree — the first diff on a fresh
project reports the whole spec as added.

```sh
spex diff                 # human summary
spex diff --json          # machine-readable, the form `spex impact` consumes
```

| Exit | Meaning |
|---|---|
| 0 | diff produced, no completeness errors |
| 1 | input error |
| 2 | completeness errors found — chiefly the removed-name check, which flags prose still referring to a node that just disappeared. The full diff is still on stdout; the non-zero status tells you **not** to pipe it into `spex impact` |

### `spex impact`

Maps a diff onto task actions.

| Flag | Default | Purpose |
|---|---|---|
| `--diff <path>` | stdin | Diff JSON to read |
| `--beads <path>` | — | Tracker list JSON (e.g. `br list --json` output), supplying live task status for cleanup classification |
| `--json` | on | Emit JSON. Currently the only supported format |
| `--bead-cli <name>` | `br` | Deprecated; use `--beads` |

Without `--beads`, classification can only reason from the task journal.

```sh
br list --all --json > beads.json
spex diff --json | spex impact --beads beads.json > impact.json
```

### `spex emit`

Composes `changeset.json` (v2) from an impact report: an ordered,
tool-agnostic list of `create` / `close` / `label` / `tag` operations with
forward references encoded, ready for an adapter.

| Flag | Default | Purpose |
|---|---|---|
| `--impact <path>` | stdin | Impact report JSON |
| `--proposal <stem>` | — | Proposal filename stem, e.g. `2026-04-18-decouple-spex-from-br` |
| `--git-head <sha>` | — | Caller-supplied git HEAD SHA, 7–40 hex characters |
| `--out <path>` | stdout | Changeset output path |

```sh
spex emit --impact impact.json \
          --proposal 2026-04-18-decouple-spex-from-br \
          --git-head "$(git rev-parse HEAD)" \
          --out changeset.json
```

| Exit | Meaning |
|---|---|
| 0 | changeset written |
| 1 | input or validation error (bad flags, malformed JSON, bad SHA) |
| 2 | changeset could not be built from otherwise valid input |

### `spex ingest`

Reconciles a changeset with the receipts an adapter wrote, appends the
resulting events to `spec/.history.jsonl`, and writes `spec/.snapshot.json`.
Ingest is the only writer of the baseline.

| Flag | Default | Purpose |
|---|---|---|
| `--changeset <path>` | — | Changeset JSON (required) |
| `--receipts <path>` | — | Receipts JSON (required) |
| `--mode <mode>` | `normal` | `normal` or `refresh` |
| `--git-head <sha>` | — | Refresh mode only: commit stamped on the refresh receipt. Normal mode ignores it, since the changeset carries its own |

**`--mode refresh`** absorbs spec drift that owes no task work. It takes an
empty changeset and empty receipts, appends one change event per drifted or
absorbable added/removed leaf, closes them with a refresh receipt, and
rewrites the snapshot — atomically, with no task lifecycle. Added and removed
leaves are refused unless the node type is absorbable (`requirement` and `api`
in both directions, `component` in the removed direction only), and a removed
node with a still-open task is refused regardless of type.

```sh
spex ingest --changeset changeset.json --receipts receipts.json
spex ingest --mode refresh --changeset empty.json --receipts empty.json \
            --git-head "$(git rev-parse HEAD)"
```

| Exit | Meaning |
|---|---|
| 0 | success — complete, or partial with no reconciler errors |
| 1 | input error: bad flags, malformed JSON, op ID mismatch, IO failure, missing pre-refresh snapshot, non-empty refresh artifacts |
| 2 | invariant failure (journal unchanged on disk) or refresh refusal |

---

## Querying

### `spex map`

Reads the task journal.

| Subcommand | Purpose |
|---|---|
| `spex map list` | List the folded node-to-task linkage |
| `spex map get <key>` | One node's journal linkage, by identity hash or task ID |
| `spex map context <key>` | Full spec context for a node — live or removed — by identity hash or task ID |

`map context` is the everyday entry point: it returns `arch_file`,
`test_files`, `flow_files` and `module_file` for the node, and it answers for
removed nodes too, because the journal holds their biography.

```sh
spex map context 96c6c15ecc3e
spex map context spexmachina-ow43.5     # a task ID works as the key
```

### `spex render`

Renders the spec.

| Flag | Default | Purpose |
|---|---|---|
| `-f`, `--format <fmt>` | `markdown` | `markdown`, `dot`, or `json` |
| `--slim` | off | JSON only: nodes only, each `{id, type, name, module}` |

`--slim` drops inlined content, descriptions and edges, leaving a compact
name→hash lookup table. Read edges from `module.json`.

```sh
spex render --format markdown
spex render --format dot | dot -Tpng > spec.png
spex render --format json | jq '.nodes[] | select(.type == "component")'
spex render --format json --slim | jq -r '.nodes[] | "\(.name)\t\(.id)"'
```

### `spex hash-id`

Computes the identity hash for a node without touching the spec.

| Flag | Purpose |
|---|---|
| `--type <type>` | `requirement`, `component`, `data_flow`, `test_section`, `api`, `module` |
| `--name <name>` | Node name or title |
| `--module <module>` | Required for module-scoped node types |

```sh
spex hash-id --type requirement --name "Declared stack"
spex hash-id --type component --module merkle --name "Hasher"
```

---

## Proposals

### `spex template <project|change>`

Writes a proposal template to stdout.

### `spex register <proposal-path>`

Registers a proposal into `spec/proposals/`.

### `spex log`

Shows proposal history and the task actions linked to it. Reads task JSON on
stdin.

| Flag | Purpose |
|---|---|
| `--proposal <stem>` | Filter to a single proposal stem |
| `--json` | JSON output |

```sh
br list --all --json | spex log --proposal 2026-04-18-decouple-spex-from-br
```

---

## Binary

### `spex version`

Prints version and build information — the version stamp, commit and build
date compiled in at release time. A source build reports `dev`.

### `spex upgrade`

Self-updates the installed binary using the embedded, signed installer. See
the Upgrading section of the [README](../README.md) for the trust model.

| Flag | Purpose |
|---|---|
| `--version <vX.Y.Z>` | Install this exact release, in any direction. The deliberate path to an older release |
| `--check` | Report the comparison and change nothing |
| `--dry-run` | Alias for `--check` |
| `--rollback` | Restore the previous binary from its `.bak` backup |

With no flags, upgrade is **forward-only**: it resolves the latest release and
hard-refuses — non-overridably — a latest that is older than what is
installed. `--check` exits 0 even when the outcome is such an anomaly; the
refusal applies to the real upgrade path.

The command's exit status is the installer script's own.

---

## Adapters

Everything past `spex emit` is outside the binary. An adapter reads a
changeset, applies it to a tracker, and writes receipts.
`scripts/apply-br.sh` is the reference implementation for `br` (beads_rust):

```
apply-br.sh [<changeset.json>] [<receipts.json>]

  no args   changeset on stdin, receipts on stdout
  one arg   that file is the changeset; receipts on stdout
  two args  receipts written atomically to <receipts.json>
```

`spex` depends on the receipts contract, not on this script.
