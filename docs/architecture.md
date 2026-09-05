# Architecture

How a spec change becomes a set of task actions, and why every step is reproducible.

The authoritative source is `spec/**`. This document is the human-readable
tour of it; where the two disagree, the spec wins.

## The shape of the problem

An agent working from a specification mixes two kinds of work. **Creative**
work — writing content, generating code, reviewing changes — needs a model.
**Structural** work — parsing the spec, diffing it, computing what a change
invalidates, opening and closing tasks — does not. It needs a program, because
the only useful answer is the same answer every time.

`spex` is that program. It never calls a model, never guesses, and never
reaches the network. Given the same spec directory and the same snapshot it
produces byte-identical output, which is what makes it safe to put inside an
automated loop.

## The spec as a typed graph

A spec directory is a JSON skeleton with markdown content leaves:

```
spec/
├── project.json          project requirements + module declarations
├── .snapshot.json        the merkle baseline (what was last ingested)
├── .history.jsonl        the task journal (append-only)
├── proposals/            why each change was made
└── <module>/
    ├── module.json       module requirements, components, flows, tests, apis
    └── *.md              the content leaves
```

The JSON carries structure and identity; the markdown carries prose. Six node
types live in that graph:

| Node type | Declared in | Carries content? |
|---|---|---|
| `requirement` | `project.json`, `module.json` | no — title and description inline |
| `module` | `project.json` | no — a container |
| `component` | `module.json` | yes — `content` points at a markdown leaf |
| `data_flow` | `module.json` | yes |
| `test_section` | `module.json` | yes |
| `api` | `module.json` | no — names an external entry point |

They are connected by typed edges, all expressed as arrays of identity hashes:

- `implements` — component → requirement
- `depends_on` — requirement → requirement
- `uses` — component → component, data_flow → component
- `describes` — test_section → component
- `provided_by` — api → component
- `described_in` — implicit, from any node's `content` path to its markdown leaf

The whole thing must be an acyclic directed graph; `spex validate` refuses
anything else.

### Identity is derived, not assigned

A node's ID is not a counter and not a UUID. It is the first 6 bytes of
`SHA256(type/name)` — or `SHA256(type/module/name)` for module-scoped types —
rendered as 12 lowercase hex characters (`schema.IdentityHash`). `spex hash-id`
computes one directly:

```sh
spex hash-id --type component --module merkle --name "Hasher"
```

This has a consequence worth stating plainly: **renaming a node changes its
identity**. The old ID disappears and a new one appears, which the pipeline
reads as one node removed and another added — not as a rename. That is
deliberate: a name is part of what a node *is*, so changing it is a spec
change with task consequences, not a cosmetic edit.

## The merkle tree

`spex` hashes the spec bottom-up (`merkle/hasher.go`):

- **Leaves** — each markdown file is hashed by streaming its bytes
  (`HashFile`); each JSON-resident node is hashed over its canonical content
  (`HashBytes`).
- **Interior nodes** — child hashes are sorted lexicographically, then
  concatenated and hashed (`HashChildren`). Sorting is what makes the tree
  independent of array order in the JSON, so reformatting `module.json` does
  not manufacture a change.

There is no `spex hash` subcommand. The tree is built on demand inside
`spex diff` (read side) and persisted by `spex ingest` (write side). The only
durable artifact is `spec/.snapshot.json`.

## The pipeline

```
validate → diff → plan → <adapter> → ingest
```

### validate

Structural gate, run before anything else. It checks JSON Schema conformance,
that every `content` path resolves to a file that exists, that cross-file
links resolve, that IDs are unique and correctly derived from their names,
that the graph is acyclic, that declared names agree across files, that every
requirement is covered by at least one component and every component by a test
section, and that coupled sections are well formed. Each check is its own file
under `validator/`.

### diff

Rebuilds the merkle tree and compares it against `spec/.snapshot.json`,
producing a flat list of changes, each one of three types
(`merkle/diff_engine.go`):

- **Added** — present now, absent in the snapshot (no old hash)
- **Removed** — present in the snapshot, absent now (no new hash)
- **Modified** — present in both, different hash

A missing snapshot is treated as the empty tree, so the first diff on a fresh
project reports the whole spec as added. That is the bootstrap cycle: it
produces the initial journal and the initial snapshot together.

`diff` also runs the removed-name check (`validator.CheckRemovedNames`), which
belongs here rather than in `validate` because it needs the classified changes
to work with: it recovers the name of a node that just disappeared and looks
for prose that still refers to it. Findings are *completeness errors*, and
they set exit status 2 — the diff is still written to stdout, but the non-zero
status says do not pipe this onward.

### plan

Decides the whole bead-action changeset in one pass: it matches changed nodes
against the task journal's pairings, classifies each into an action,
topologically orders them, assigns idempotency labels, resolves references,
and composes the changeset. Three action types
(`plan/action_classifier.go`):

- **create** — this node needs work that does not exist yet. When it replaces
  a task being obsoleted, the action carries `OldTaskID`, which is what
  preserves the chain across a rename.
- **obsolete** — the task must go: its node left the spec, or the node
  changed and its task is closed (or its status unknown), or a test section
  folded back to a single component. It reaches the adapter as a `close` op,
  never an op named "obsolete".
- **retarget** — the node changed and its task is still open, so the task's
  target moves rather than being closed and recreated. A node whose task is
  `in_progress` refuses the run instead: a claimed task's target never moves
  under the implementer holding it. A closed task is not retargeted either —
  it takes obsolete+create, like an unknown status.

Pass `--tasks <file>` (the version-1 task-state artifact the adapter's
export half derives from the tracker, listing in-flight tasks only) so live
task status participates in the classification. The flag is required: a run
without a task-state artifact is exit 1, not a run with an empty one, since
an absent artifact would read every task as finished and re-create
in-flight work. An empty artifact is the explicit nothing-in-flight case:
no pairing is known-open and the cleanup gate defaults closed — nothing is
retargeted, no cleanup task is minted for a removed node, and a matched
modified node takes the obsolete+create path unless the journal already
records that new hash or it is a test section folding back.

The output is `changeset.json` (v4) — an ordered, tool-agnostic list of
operations drawn from a `create` / `close` / `retarget` vocabulary, with
forward references encoded so an adapter can apply them in order without
resolving IDs itself, plus a top-level `absorbed` array carrying the nodes
`--absorb` marked as cosmetic, which yield no operation at all. `plan` is a
pure function of the diff, the task-state artifact, the task journal, the
spec graph, the absorb list, a proposal ref and a caller-supplied git HEAD
SHA. It is the last step that knows anything about specs.

### adapter

Everything past `plan` is deliberately outside the binary. An adapter reads a
changeset, applies it to whatever tracker you actually use, and writes
`receipts.json` — one receipt per operation, recording what really happened.
`scripts/apply-br.sh` is the reference implementation, targeting `br`
(beads_rust). The contract, not the script, is what `spex` depends on.

### ingest

Reads the changeset and the receipts together, reconciles them (op IDs must
line up), appends one event per operation to `spec/.history.jsonl`, and writes
the new `spec/.snapshot.json`. Ingest is the only writer of the baseline.

`--mode refresh` handles the other case: spec drift that owes no task work —
a wording correction, a clarification. It takes an empty changeset and empty
receipts, appends a change event per drifted leaf, and rewrites the snapshot
with no task lifecycle at all. Added and removed leaves are refused unless the
node type is in the absorbable set (`requirement` and `api` in both
directions, `component` in the removed direction only), and a removed node
whose task is still open is refused regardless of type.

## The journal

`spec/.history.jsonl` is append-only, one JSON object per line, schema in
`schema/bead-map.schema.json`. It is the link between spec nodes and tracker
tasks, and it is deliberately a log rather than a table: folding it forward
gives the current mapping, while reading it whole gives the biography of a
node that no longer exists.

That biography is why `spex map context <id>` still answers for a removed
node — you can hand it a spec hash or a task ID and get back the full context
(`arch_file`, `test_files`, `flow_files`, `module_file`) that the node had
when it was alive.

## Modules

| Module | Purpose | Depends on |
|---|---|---|
| **Schema** | JSON Schema for `project.json`, `module.json`, the journal line format; identity hashing | — |
| **Validator** | Spec directory validation: schema, DAG, refs, coverage, name consistency | Schema |
| **Merkle** | Hash tree, snapshots, diff, change classification | Schema |
| **Plan** | Decides the bead-action changeset from a merkle diff in one pass | Merkle, Mapping |
| **Adapters** | The contract external adapters implement, and the receipts they return | Plan |
| **Ingest** | Appends journal events and saves the snapshot from changeset + receipts | Plan, Mapping, Adapters, Merkle |
| **Mapping** | The task journal and its queries (`spex map`) | Schema |
| **Proposal** | Proposal lifecycle: registration, templates, history | — |
| **Render** | Generates markdown, Graphviz DOT and JSON from the spec | Schema |
| **CLI** | Root command, version, subcommand registration | — |
| **Delivery** | Release pipeline, manifest, embedded installer, `spex upgrade` | — |

## Design constraints

These are contracts, not preferences — each is a requirement in
`spec/project.json`.

- **Go standard library first.** The permitted third-party modules are named
  in the "Declared stack" requirement. Adding a direct dependency is a spec
  change made there first.
- **Deterministic.** Same spec state and snapshot ⇒ same diff, actions and
  changeset. No model calls, no clocks in the hot path, no network.
- **Composable.** Every subcommand reads stdin or files, writes stdout or
  files, and exits with documented codes.
- **Git-native.** Snapshots, proposals and the journal are files committed to
  git. There is no database and no server.

## Where the spec changes

The spec is the truth, and it changes only in the authoring loop — the skills
described in [`skills.md`](skills.md). Automated implementer contexts never
write `spec/`; one that finds a spec defect files a drift report under
`drifts/` instead, which `/drift` later triages. The baseline in
`spec/.snapshot.json` moves only deliberately, and every refresh states its
reason.
