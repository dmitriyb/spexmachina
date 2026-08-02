# Spex Machina

*Spec ex machina — no deus required.*

A state machine and a simple CLI tool for your specifications. Define your project as a typed DAG (JSON skeleton + markdown content), track changes with a merkle tree, and let the tool figure out which tasks need updating. No LLM in the loop — just deterministic graph operations.

## Why

AI agents mix two kinds of work: **structural** (parsing specs, diffing text, computing dependencies, creating tasks) and **creative** (writing content, generating code, reviewing PRs). The structural half doesn't need an LLM — it needs a program.

Spex Machina owns the structural half. The LLM focuses on what it's good at.

## How it works

```
spec change → spex validate → spex diff → spex impact → spex emit → adapter → spex ingest
                  │                            │            │           │           │
                  │                            │            │           │           └─ appends journal,
                  │                            │            │           │              writes snapshot
                  │                            │            │           └─ executes bead actions
                  │                            │            │              against the tracker
                  │                            │            └─ composes the next changeset
                  │                            └─ finds affected beads
                  └─ confirms spec is a valid DAG
```

The merkle tree is built on demand inside `spex diff` (read) and persisted by
`spex ingest`'s SnapshotSaver (write); there is no separate `spex hash` step.
The first diff on a fresh project treats the missing snapshot as the empty
tree, so the bootstrap cycle produces the initial journal and snapshot
together.

Every change starts with a **proposal** — a traceable document committed to git that captures *why* the change is being made.

## Agent skills

Three Claude Code skills drive the creative work alongside the CLI:

- `/propose` — turns a free-form conversation into a structured proposal (project or change)
- `/spec` — reads a proposal and authors the spec (JSON + markdown), creating or modifying modules
- `/spec-review` — audits the spec for internal inconsistencies and drafts a correction proposal

These skills handle the LLM-side of spec authoring. They call `spex` subcommands for validation, registration and impact, keeping the creative and structural halves cleanly separated.

## Spec format

Specs are JSON skeleton + markdown leaves:

- `project.json` — project requirements and module declarations
- `<module>/module.json` — module requirements, architecture components, implementation sections
- `<module>/*.md` — rich content (diagrams, algorithms, narratives) linked from JSON

The JSON is machine-readable for graph operations. The markdown is human-readable for content. The merkle tree hashes both.

## Modules

| Module | What it does |
|--------|-------------|
| **Schema** | JSON Schema definitions for project.json, module.json and the journal line format |
| **Validator** | Validates spec structure: JSON schema conformance, content path resolution, DAG acyclicity, ID uniqueness |
| **Merkle** | Hash tree over the spec, snapshots, diff, impact classification |
| **Impact** | Maps changed spec nodes to affected beads tasks |
| **Emit** | Composes `changeset.json` — an ordered, tool-agnostic list of bead operations — from the impact report |
| **Adapters** | Reference adapter scripts outside the binary; each executes a changeset against a tracker and writes `receipts.json` |
| **Ingest** | Appends journal events from changeset + receipts and saves the new snapshot |
| **Map** | Reads the task journal (`spec/.history.jsonl`), linking spec node IDs to task IDs; folds, context resolution, CLI queries |
| **Proposal** | Proposal lifecycle: registration, validation, history |
| **Render** | Generates markdown, graphviz DOT, or JSON from the spec |
| **CLI** | Root command, version subcommand, and the subcommand registration framework |

## Task tracking

Uses [beads](https://github.com/steveyegge/beads) via `br` (beads_rust) for issue tracking. Tasks are derived from the spec — each maps to a requirement + component with full traceability.

## Status

Self-hosting. The full `validate → diff → impact → emit → adapter → ingest` pipeline ships, and Spex Machina's own spec is managed by Spex Machina — the tasks in `.beads/` are generated from the spec in `spec/`.

## License

See [LICENSE](LICENSE).
