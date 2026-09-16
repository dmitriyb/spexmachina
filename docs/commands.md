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

Every command that reads project state — `diff`, `plan`, `ingest`, `map`,
`register` and `doctor` — runs the lifecycle pre-flight first and exits **3**,
*not a spex project*, when it refuses: either `.spex/` is absent (stderr names
`spex init`) or it is present but its snapshot or journal is missing or
unparseable (stderr names `spex doctor`). The two messages never collapse into
one, because running `init` on a broken project destroys the journal.
`validate`, `render`, `hash-id`, `template`, `log` and `init` do not read
project state and never exit 3.

---

## Project state

### `spex init`

Creates `.spex/` with exactly two files: a snapshot seeded with the canonical
empty tree — never a snapshot of the spec that already exists, so the first
`spex diff` reports the whole spec as added — and an empty journal, with no
init event. The directory is committed to git, not ignored.

It refuses a directory that already has `.spex/`, whatever its condition, and
leaves it untouched: it is the one command that can destroy a journal, so it
never overwrites. A broken state directory is `spex doctor`'s to diagnose.

```sh
spex init
```

| Exit | Meaning |
|---|---|
| 0 | `.spex/` created |
| 1 | `.spex/` already exists, or the directory could not be written |

### `spex doctor`

Reports the health of the project state and never repairs it: after any run
the project directory is byte-identical. The report is JSON on stdout —
`{"healthy":…, "findings":[{"artifact":…, "status":…}]}` — with `status` one
of `present`, `missing` or `unreadable`, a `detail` on unreadable artifacts,
and a `fix` naming the command that resolves the finding. Only the
never-initialised case carries a fix (`spex init`); damage inside an existing
`.spex/` names no command, because re-initialising is how a journal dies.

```sh
spex doctor
```

| Exit | Meaning |
|---|---|
| 0 | healthy |
| 1 | `.spex/` exists but its snapshot or journal is missing or unreadable |
| 3 | never initialised — the single finding is `.spex` missing, fix `spex init` |

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
| `--snapshot <path>` | `.spex/snapshot.json`, as resolved by the pre-flight | Snapshot to compare against |

`spex init` seeds the snapshot with the empty tree, so the first diff on a
fresh project reports the whole spec as added. A missing snapshot is exit 3,
not an empty tree.

```sh
spex diff                 # human summary
spex diff --json          # machine-readable, the form `spex plan` consumes
```

| Exit | Meaning |
|---|---|
| 0 | diff produced, no completeness errors |
| 1 | input error |
| 2 | completeness errors found — chiefly the removed-name check, which flags prose still referring to a node that just disappeared. The full diff is still on stdout; the non-zero status tells you **not** to pipe it into `spex plan`, which refuses such a diff anyway |
| 3 | not a spex project |

### `spex plan`

Decides the whole bead-action changeset from a diff in one pass — match,
classify, order, label, resolve, compose — and writes `changeset.json` (v4):
an ordered, tool-agnostic list of `create` / `close` / `retarget` operations
with forward references encoded, plus a top-level `absorbed` array for the
nodes marked cosmetic, ready for an adapter.

| Flag | Default | Purpose |
|---|---|---|
| `--proposal <stem>` | — | Proposal filename stem, e.g. `2026-08-13-plan-module`. Required |
| `--git-head <sha>` | — | Caller-supplied git HEAD SHA, 7–40 hex characters. Required |
| `--tasks <path>` | — | Version-1 task-state artifact (`schema/task-state.schema.json`) the adapter's export half derived from the tracker, listing in-flight tasks only. Required |
| `--diff <path>` | stdin | Diff JSON to read; `-` selects stdin explicitly |
| `--absorb <path>` | — | Git-committed JSON list of `{node, reason}` marks; a marked node's change yields no op and rides in `absorbed` instead |
| `--out <path>` | stdout | Changeset output path |

`--tasks` is required: a run without a task-state artifact is exit 1, not
a run with an empty one, since an absent artifact would read every task as
finished and re-create in-flight work.

Prefer a short `--git-head`: a node-bearing create's idempotency label is
`spex:<git-head>:op-NN`, and `br` rejects a label over 50 characters. (The
proposal epic's label is fixed earlier, at `spex register`.)

```sh
scripts/export-br.sh tasks.json
spex diff --json | spex plan --proposal 2026-08-13-plan-module \
                             --git-head "$(git rev-parse --short HEAD)" \
                             --tasks tasks.json --out changeset.json
```

| Exit | Meaning |
|---|---|
| 0 | changeset written |
| 1 | input error: bad or missing flags (`--tasks` included), malformed JSON, bad SHA, unreadable `--tasks` or journal, or a diff that still carries completeness errors |
| 2 | contract refusal: a claimed (`in_progress`) task's node changed, an invalid absorb entry, a dep cycle, an unresolvable dep or parent |
| 3 | not a spex project |

### `spex ingest`

Reconciles a changeset with the receipts an adapter wrote, appends the
resulting events to `.spex/history.jsonl`, and writes `.spex/snapshot.json`.
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
| 3 | not a spex project |

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

Computes the identity hash for a node. It reads `spec/profile.json` under `--spec-dir` when that file is present and uses the built-in default profile otherwise; that resolution is the only spec access it makes. Given the resolved profile the output is a pure function of the three flags.

| Flag | Purpose |
|---|---|
| `--type <type>` | A node type the resolved profile declares, plus the fixed `module` type. Under the default profile: `requirement`, `component`, `data_flow`, `test_section`, `api`, `module` |
| `--name <name>` | Node name |
| `--module <module>` | Required for module-scoped node types |

```sh
spex hash-id --type requirement --name "Declared stack"
spex hash-id --type component --module merkle --name "Hasher"
```

---

## Authoring

The write path over `spec/`. Every command reads the resolved profile, applies
the change to an in-memory copy, runs the validator's own checkers over it, and
either refuses — nothing written, the error document on stdout with a `fix` per
entry — or writes and prints a report: the files `written`, the `obligations`
the change incurred (the completeness checker's entries for this write against
the tree as it was), and where relevant a `retired_name` or `replaced_target`.
None of them reads or writes `.spex/`, none needs an initialised project, and
all honour `--spec-dir`. Output is compact when piped, pretty-printed on a
terminal.

| Exit | Meaning |
|---|---|
| 0 | written; the report is on stdout |
| 1 | input error: missing or malformed flag, no `project.json` under `--spec-dir`, malformed or out-of-range profile |
| 2 | refused: the validator's entries, each with a `fix`, on stdout; the tree is untouched |

### `spex profile show`

Prints the resolved profile as one JSON document: `spec/profile.json` when
present, the built-in default otherwise, after validation. A version 1 profile
prints with the version 2 conventions filled in. Read it for the declared
types, their plural keys, fields, reference kinds and targets, coverage chains,
and per content-bearing type the `content_prefix` and `leaf_sections`.

```sh
spex profile show | jq '.node_types[] | {name, scope, fields: [.fields[].name]}'
```

### `spex node add <name>`

Declares a node. File, array, id, required fields and content path are decided
from the profile; a content-bearing node gets its leaf skeleton at the same
time.

| Flag | Purpose |
|---|---|
| `--type <type>` | A type the resolved profile declares, or `module`. Required |
| `--module <module>` | The owning module, for a module-scoped type |
| `--field <name>=<value>` | One declared field value; repeatable. Reference fields take an id, or a comma-separated list for many |

`--type module` appends the module to `project.json` and writes a
`module.json` skeleton declaring its name, so the module is visible to every
gate from its first moment.

```sh
spex node add widgets --type module
spex node add "List widgets" --type requirement --module widgets \
  --field type=functional --field preq_id=2836ae8c6551 --field description="Lists them."
spex node add WidgetLister --type component --module widgets \
  --field description="Lists widgets." --field implements=3207191d7b70
```

### `spex node remove <id>`

Removes a node and its leaf. While anything still references the id — a
reference field in any JSON file, a typed link in any leaf — the removal is
refused and each reference is listed with the `spex edge remove` that
retargets it.

| Flag | Purpose |
|---|---|
| `--force` | Remove anyway; the same list is printed as what was left dangling |

A module id is refused, forced or not. The report carries the `retired_name`
for the vocabulary sweep.

### `spex node rename <id> <new-name>`

One transaction: the new id is derived, the entry rewritten, every reference
field and every typed link naming the old id repointed, and the content file
moved. A refusal — undeclarable name, collision, module id — writes nothing.
The report carries the `retired_name`; the pipeline still sees a removal plus
an addition.

### `spex edge add <source-id> <field> <target-id>`

Adds one entry to one reference field, after checking that the target exists,
that the source type declares the field, that the field permits the target's
type, that a module-local field stays module-local, and that every
cycle-checked field stays acyclic. Adding an entry already held changes nothing
and says so. On a cardinality-one field such as `preq_id`, a different target
replaces the held one and the report carries it under `replaced_target`.

### `spex edge remove <source-id> <field> <target-id>`

The inverse. Clearing a required cardinality-one field is refused with the
validator's own entries; retarget it with one `spex edge add` instead.

### `spex leaf scaffold <id>`

Writes a content leaf's skeleton — `# <name>`, the type's `leaf_sections` as
`##` headings, and one placeholder line with a typed link per edge the leaf
owes — for a node whose leaf is absent or empty. A non-empty leaf is never
overwritten; the refusal names the file. A leaf holding exactly the skeleton is
reported as already scaffolded.

### `spex migrate`

Brings a spec from an earlier format version to the current one: the
`title`-to-`name` rename on every requirement, removal of every array the
profile does not declare (each removed entry reported, its content file
reported as orphaned and left on disk), and `spec_version` stamped in
`project.json`. A current tree is reported as current and left byte-identical,
so running it is the check.

---

## Proposals

### `spex template <project|change>`

Writes a proposal template to stdout.

### `spex register <proposal-path>`

Copies a proposal into `spec/proposals/` and appends a `registered` event to
the journal, keyed `<git-head>:<stem>` — the eid that becomes the proposal
epic's idempotency label, `spex:<git-head>:<stem>`. The proposal must be a
`project` or `change` template with its H2 sections intact; a malformed one is
refused before anything is written. The source file is read from
`<proposal-path>` and copied to its dated stem; a destination that already
exists is refused as already registered, so the source is a draft outside
`spec/proposals/`, not the destination itself.

| Flag | Default | Purpose |
|---|---|---|
| `--git-head <sha>` | — | Caller-supplied git HEAD SHA, 7–40 hex characters. Required; validated before the proposal is read |

```sh
spex register --git-head "$(git rev-parse --short HEAD)" drafts/2026-08-13-plan-module.md
```

| Exit | Meaning |
|---|---|
| 0 | registered; stdout prints the destination path |
| 1 | missing or malformed `--git-head`, unreadable or malformed proposal, or a destination already present |
| 3 | not a spex project |

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
