# Spex Machina

*Spec ex machina — no deus required.*

A state machine and a simple CLI tool for your specifications. Define your project as a typed DAG (JSON skeleton + markdown content), track changes with a merkle tree, and let the tool figure out which tasks need updating. No LLM in the loop — just deterministic graph operations.

## Why

AI agents mix two kinds of work: **structural** (parsing specs, diffing text, computing dependencies, creating tasks) and **creative** (writing content, generating code, reviewing PRs). The structural half doesn't need an LLM — it needs a program.

Spex Machina owns the structural half. The LLM focuses on what it's good at.

## How it works

```
spec change → spex validate → spex hash → spex diff → spex impact → spex apply
                  │                                         │              │
                  │                                         │              └─ creates/closes/updates beads
                  │                                         └─ finds affected tasks
                  └─ confirms spec is a valid DAG
```

Every change starts with a **proposal** — a traceable document committed to git that captures *why* the change is being made.

## Agent skills

Before `spex` CLI exists, two Claude Code skills drive the creative work:

- `/propose` — turns a free-form conversation into a structured proposal (project or change)
- `/spec` — reads a proposal and authors the spec (JSON + markdown), creating or modifying modules

These skills handle the LLM-side of spec authoring. Once the CLI is built, `/propose` and `/spec` will call `spex` subcommands for validation and registration, keeping the creative and structural halves cleanly separated.

## Spec format

Specs are JSON skeleton + markdown leaves:

- `project.json` — requirements, module declarations, milestones
- `<module>/module.json` — module requirements, architecture components, implementation sections
- `<module>/*.md` — rich content (diagrams, algorithms, narratives) linked from JSON

The JSON is machine-readable for graph operations. The markdown is human-readable for content. The merkle tree hashes both.

## Modules

| Module | What it does |
|--------|-------------|
| **Schema** | JSON Schema definitions for project.json and module.json |
| **Validator** | Validates spec structure: DAG acyclicity, cross-references, orphan detection |
| **Merkle** | Hash tree over the spec, snapshots, diff, impact classification |
| **Impact** | Maps changed spec nodes to affected beads tasks |
| **Apply** | Executes bead actions (create/close/update) from impact reports |
| **Proposal** | Proposal lifecycle: registration, validation, history |
| **Render** | Generates markdown, graphviz DOT, or JSON from the spec |

## Task tracking

Uses [beads](https://github.com/steveyegge/beads) via `br` (beads_rust) for issue tracking. Tasks are derived from the spec — each maps to a requirement + component with full traceability.

## Status

Bootstrap phase. Schema complete, working through Validator and Merkle. Once Apply is built, it will generate its own tasks from the spec (self-hosting).

## License

See [LICENSE](LICENSE).
