# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Spex Machina is a standalone CLI (`spex`) that owns the structural half of spec-driven development. It defines specs as a typed graph (JSON skeleton + markdown content leaves), tracks changes via a merkle tree, computes impact deterministically, and maps spec nodes to beads tasks. The LLM focuses on creative work and calls `spex` for everything mechanical.

## Module Hierarchy

The pipeline is `spex validate → diff → impact → emit → <adapter> → ingest`.

| Module | Purpose | Depends On |
|--------|---------|------------|
| Schema | JSON Schema for project.json + module.json + the journal line format | — |
| Validator | Spec directory validation (DAG, refs, coverage) | Schema |
| Merkle | Hash tree, snapshots, diff, impact classification | Schema |
| Impact | Map merkle diff to bead actions | Merkle |
| Emit | Compose a tool-agnostic changeset from an impact report | Impact, Mapping |
| Adapters | Contract for external adapters (`scripts/apply-br.sh` is the reference) that apply a changeset and write receipts | Emit |
| Ingest | Append journal events + save the snapshot from changeset+receipts; `--mode refresh` absorbs no-bead-work drift | Emit, Adapters, Merkle |
| Mapping | Task journal (`spec/.history.jsonl`) linking spec nodes to tasks (`spex map context`) | Schema |
| Proposal | Proposal lifecycle (register, log, templates) | — |
| Render | Generate markdown, DOT, JSON from spec | Schema |

## Technical Constraints

- **Go standard library first**: the stack is a declared contract, not folklore — project requirement `96c6c15ecc3e` "Declared stack" in `spec/project.json` names the language and every permitted third-party module. Read it there; adding a direct dependency is a spec change made there first.
- **Deterministic**: Same spec state + snapshot = same diff, impact, changeset. No LLM calls in `spex`.
- **Composable**: Every subcommand reads stdin or files, writes stdout or files, exits with documented codes. Pipeable.
- **Git-native**: Snapshots and proposals are files committed to git. No external state.

## Build & Test

- Build: `go build -o bin/ ./cmd/spex/`
- Test: `go test ./...`
- Vet: `go vet ./...`

## Git Conventions

- Default branch is `main` (never `master`).
- For "current state" of any committed artifact, read from `origin/main` (`git show origin/main:<path>`) rather than a stale branch's working tree.
- For rebase-risk, read `git log <base>..<branch> --oneline` and `git show --stat <sha>` per commit — not `git diff <base>..<branch> --stat`, which overstates conflict surface.

## Issue Tracking

This project uses `br` (beads_rust) for issue tracking. Do NOT use markdown TODOs.

Your task's spec context is provided on input, and the spec files are the source of truth — beads do not duplicate spec content. `design`, `acceptance_criteria` and `notes` are absent from bead data entirely; `description` is empty except on a legacy tail of older beads that still carries prose copied from the spec — read the spec, not the copy. To reach *another* bead's context mid-work, `br` is the entrypoint: `br show <id> --json` to inspect the bead and find its `spex:<spec_node_id>` label, then `bin/spex map context <spec_node_id>` for its full spec (`arch_file`, `test_files`, `flow_files`, `module_file`); a bead id works as the key too.

- Cleanup beads (label `spex:cleanup`) resolve through the journal: their `spex:cleanup-<hash>` hash names a removed node whose biography `spex map context` still answers.
- `br` reads a local sqlite db rebuilt from `.beads/issues.jsonl`; pass `--no-db` to any `br` command to read JSONL-only (always fresh). `br list` hides rows by default (open-only, `--limit 50`) — use `br ready`, `br show`, or `br list --all --limit 0`.

## Spec Change Doctrine

- **The spec is the truth; it changes only in the authoring loop** (interactive sessions using `/propose`, `/spec`, `/spec-review`, `/drift-fix`). Implementer boxes never write `spec/` — the portitor gate denies it structurally.
- **An implementer that finds a spec defect files a drift report**, `drifts/drift-<bead-id>.json` (schema: `schema/drift.schema.json`), never a spec edit. Non-blocking reports ride along in the bead's own PR and are triaged after the epic. A blocking report (the bead's own contract is ambiguous) travels as its own PR — the drift file plus the bead's return to `open` in the same commit — and stops the epic via the settle sentinel; the reviewer of that PR validates the drift claim itself. `/drift-fix` consumes all reports.
- **The baseline (`spec/.snapshot.json`) moves only deliberately** — a mint when work is born, a refresh when a correction owes none — always in the authoring loop, never automated, never in a box. Every refresh states its reason.
- **An epic whose drifts/ is non-empty is not closed** until `/drift-fix` has triaged the reports.

## Organizational Constraints

- **Spec traceability**: All code traces back to bead requirements.
- **Self-hosting**: Spex Machina's own spec is managed by Spex Machina.

## Where to Find Details

- **Authoring skills**: `skills/` — `/propose` (draft a proposal in plan mode), `/spec` (author spec files), `/spec-review` (audit spec internal consistency), `/drift-fix` (triage implementer drift reports).
- **Proposals**: `spec/proposals/`
- **Beads**: `.beads/`
