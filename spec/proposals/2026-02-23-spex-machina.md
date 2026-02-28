# Project Proposal: Spex Machina

*The spec state machine.*

## Vision

Software projects driven by AI agents need a deterministic backbone. Today, skills like `/spec`, `/backlog`, `/sync`, `/implement`, `/review`, `/fix` mix two kinds of work: structural operations (parsing specs, diffing text, traversing dependencies, creating tasks) and creative operations (writing spec content, generating code, reviewing PRs). The structural half is done by the LLM, which makes it fragile, irreproducible, and expensive.

Spex Machina is a standalone CLI (`spex`) that owns the structural half. It defines specs as a typed graph (JSON skeleton + markdown content leaves), tracks changes over time via a merkle tree, computes impact deterministically, and maps spec nodes to beads tasks. The LLM focuses on what it's good at — creative work — and calls `spex` for everything mechanical.

Every spec change — from initial project creation to incremental edits — is driven by a proposal: a traceable document that captures WHY the change is being made, committed to git alongside the spec change itself.

## Modules

### 1. Schema

The JSON Schema definitions for `project.json` and `module.json`. This is the foundation — it defines what a valid spec looks like. Includes:

- Project-level schema: requirements, architecture reference, module declarations with inter-module dependencies
- Module-level schema: requirements (functional + non-functional, linked to project requirements via `preq_id`), architecture components (with markdown content links), implementation sections (with markdown content links), data flows
- All IDs are numeric within their type
- All cross-references are validated (requirements referenced by components, components referenced by impl sections, etc.)

### 2. Validator

Reads a spec directory, checks:

- JSON schema conformance (project.json, module.json)
- All `content` paths resolve to existing markdown files
- DAG is acyclic (module dependencies, component→requirement refs)
- No orphan requirements (every requirement referenced by at least one component)
- No orphan components (every component referenced by at least one impl section)
- All numeric IDs unique within their type and container

Exits 0 if valid, exits 1 with structured JSON errors listing every violation.

### 3. Merkle

Computes the merkle tree over the spec:

- Leaf nodes: hash of each markdown file + hash of each JSON file
- Interior nodes: hash of children's hashes
- Tree structure mirrors the spec: `project_hash → module_hash → {arch_hash, impl_hash} → leaf hashes`
- Snapshot storage: a hash file committed to git alongside the spec
- Diff: compare current hashes against stored snapshot, report changed leaves and changed paths
- Impact classification: impl-only / arch+impl / structural change

### 4. Impact

Given a merkle diff, computes affected beads:

- Reads bead metadata (`metadata.component`, `metadata.impl_section`, `metadata.module`, `metadata.spec_hash`)
- Matches changed spec nodes to referencing beads
- Classifies actions: create (new spec nodes without beads), close (removed spec nodes with existing beads), review (modified spec nodes with existing beads)
- Outputs structured impact report (JSON)

### 5. Apply

Executes the impact report:

- Creates beads via `bd create` with full spec metadata
- Closes obsolete beads via `bd close` with reason
- Updates bead metadata for modified spec nodes
- Tags all affected beads with proposal reference
- Saves new merkle snapshot

### 6. Proposal

Manages the proposal lifecycle:

- Register: copy/link proposal to `proposals/` directory, validate structure (required sections present)
- Log: show proposal history — which proposals led to which spec changes, linked to which bead actions
- Two templates: project proposal (vision, modules, requirements, design decisions) and change proposal (context, proposed change, impact expectation)

### 7. Render

Generates human-readable output from spec JSON + markdown:

- Markdown: collated spec document for reading (requirements → architecture → implementation, with content inlined from md files)
- DOT: graphviz graph of spec nodes and edges
- JSON: machine-readable full graph for piping to other tools

## Key requirements

### Functional

1. **Validate spec structure** — given a spec directory, confirm it's a valid DAG with no orphans, no cycles, no broken references. Structured error output.
2. **Compute merkle tree** — hash every node, store snapshots, diff against previous snapshots, classify change impact paths.
3. **Map spec nodes to beads** — each plan-relevant spec node maps to a bead with `spec_id`, typed dependencies, and full metadata (requirements, component, impl_section, module, spec_hash, proposal).
4. **Compute impact** — given changed spec nodes, find all affected beads and classify actions (create/close/review).
5. **Apply changes** — execute bead actions via `bd` CLI, tag with proposal, save snapshot.
6. **Manage proposals** — register, validate structure, link to spec changes and bead actions, show history.
7. **Render spec** — generate markdown, DOT, or JSON from spec structure.

### Non-functional

1. **Self-hosting** — Spex Machina's own spec is managed by Spex Machina (after bootstrap).
2. **Deterministic** — given the same spec state and snapshot, always produce the same diff, impact, and actions. No LLM calls.
3. **Composable** — every subcommand reads stdin or files, writes stdout or files, exits 0/1. Pipeable.
4. **Fast** — validating a spec with 100 modules and 1000 nodes should take <1s.
5. **Git-native** — snapshots are files committed to git. Proposals are files committed to git. No external state.

## Design decisions

### Spec format: JSON skeleton + markdown leaves

The spec graph structure (nodes, edges, IDs, cross-references) lives in JSON where every entity is a field, not a regex target. Rich content (ASCII diagrams, code snippets, algorithm descriptions, data flow narratives) lives in markdown files linked from JSON via `content` paths. This keeps the graph machine-readable while allowing rich human-authored content.

The merkle tree hashes both: JSON structure changes are detected at the interior nodes, markdown content changes are detected at the leaves. The impact path (impl-only vs arch+impl vs structural) is determined by which level of the tree changed.

### Requirement decomposition: project → module

Project-level requirements define high-level goals. Module-level requirements are decompositions of project requirements, linked via `preq_id` (project requirement ID). This creates a traceable hierarchy: a project requirement like "Validate spec structure" decomposes into module requirements like "JSON schema conformance", "Content path resolution", and "DAG acyclicity". The validator can check that every module requirement traces back to a project requirement.

### Task backend: beads, not GitHub Issues

Beads provides native `spec_id`, typed dependency graph (`blocks`, `parent-child`, `conditional-blocks`), arbitrary JSON metadata, and deterministic CLI (`bd --json`). GitHub Issues lacks all of these — dependencies are text in body, spec references are label conventions, metadata is parsed from prose by the LLM. `external_ref` on beads bridges the `Closes #N` gap for PR linking. The beads viewer (`bv`) TUI provides kanban, dependency graph visualization, PageRank, bottleneck detection, and critical path analysis — far beyond GitHub's flat issue list.

### Proposals as first-class artifacts

Every spec transition has a traceable rationale. Proposals are committed to git, referenced in bead metadata, and queryable via `spex log`. This creates a full audit trail: conversation → proposal → spec change → merkle diff → bead actions. Six months later, "why did we add this component?" has a concrete answer.

### Supervised spec changes, agentic task execution

Spec changes cascade — one node change can affect many tasks. The cost of supervision is low (review a structured diff), the cost of unsupervised changes is high (wrong tasks created, correct tasks closed). So spec changes always go through: propose → validate → impact → approve → apply.

Task execution (`/implement`, `/review`, `/fix`) runs autonomously. Once tasks exist in beads with clear spec context (requirements, component, impl_section), agents work independently. `bd ready --json` picks the next task, `bd update --claim` locks it, `external_ref` tracks the PR.

### No steps in the spec

The current plan.md conflates spec content with project management. In Spex Machina, the spec defines WHAT to build (requirements, architecture, implementation details). Tasks define the WORK and live in beads. The agent proposes task breakdowns from the spec following the heuristic: ~1 requirement + ~1 component per task, ≤500 LOC per PR. Task granularity is a project management decision, not a spec decision.

### Standalone CLI, not embedded in skills

`spex` is a binary that skills call, not a library embedded in the skill framework. This keeps the tool testable independently, usable outside of Claude Code, and version-controllable separately from the skills. Skills are rewritten to call `spex` subcommands instead of doing structural work themselves.

### Go implementation

Go for the CLI: single binary, fast compilation, excellent JSON handling, `os/exec` for calling `bd`, strong stdlib for hashing and file operations. No runtime dependencies.
