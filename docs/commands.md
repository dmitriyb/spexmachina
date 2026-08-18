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
failure. `spex diff`, `spex plan` and `spex ingest` add a distinct **2**; each
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
spex diff --json          # machine-readable, the form `spex plan` consumes
```

| Exit | Meaning |
|---|---|
| 0 | diff produced, no completeness errors |
| 1 | input error |
| 2 | completeness errors found — chiefly the removed-name check, which flags prose still referring to a node that just disappeared. The full diff is still on stdout; the non-zero status tells you **not** to pipe it into `spex plan`, which refuses such a diff anyway |

### `spex plan`

Decides the whole bead-action changeset from a diff in one pass — match,
classify, order, label, resolve, compose — and writes `changeset.json` (v3):
an ordered, tool-agnostic list of `create` / `close` / `retarget` operations
with forward references encoded, plus a top-level `absorbed` array for the
nodes marked cosmetic, ready for an adapter.

| Flag | Default | Purpose |
|---|---|---|
| `--proposal <stem>` | — | Proposal filename stem, e.g. `2026-08-13-plan-module`. Required |
| `--git-head <sha>` | — | Caller-supplied git HEAD SHA, 7–40 hex characters. Required |
| `--diff <path>` | stdin | Diff JSON to read; `-` selects stdin explicitly |
| `--beads <path>` | — | Tracker list JSON (e.g. `br list --json` output), supplying live task status |
| `--absorb <path>` | — | Git-committed JSON list of `{node, reason}` marks; a marked node's change yields no op and rides in `absorbed` instead |
| `--out <path>` | stdout | Changeset output path |

Without `--beads` no pairing is known-open and the cleanup gate defaults
closed: nothing is retargeted, and no cleanup task is minted for a removed
node.

Prefer a short `--git-head`: a node-bearing create's idempotency label is
`spex:<git-head>:op-NN`, and `br` rejects a label over 50 characters. (The
proposal epic's label is fixed earlier, at `spex register`.)

```sh
br list --all --json > beads.json
spex diff --json | spex plan --proposal 2026-08-13-plan-module \
                             --git-head "$(git rev-parse --short HEAD)" \
                             --beads beads.json --out changeset.json
```

| Exit | Meaning |
|---|---|
| 0 | changeset written |
| 1 | input error: bad flags, malformed JSON, bad SHA, unreadable `--beads` or journal, or a diff that still carries completeness errors |
| 2 | contract refusal: a claimed (`in_progress`) task's node changed, an invalid absorb entry, a dep cycle, an unresolvable dep or parent |

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

Everything past `spex plan` is outside the binary. An adapter reads a
changeset, applies it to a tracker, and writes receipts.
`scripts/apply-br.sh` is the reference implementation for `br` (beads_rust):

```
apply-br.sh [<changeset.json>] [<receipts.json>]

  no args   changeset on stdin, receipts on stdout
  one arg   that file is the changeset; receipts on stdout
  two args  receipts written atomically to <receipts.json>
```

`spex` depends on the receipts contract, not on this script.
