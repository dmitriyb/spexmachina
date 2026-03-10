# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Spex Machina is a standalone CLI (`spex`) that owns the structural half of spec-driven development. It defines specs as a typed graph (JSON skeleton + markdown content leaves), tracks changes via a merkle tree, computes impact deterministically, and maps spec nodes to beads tasks. The LLM focuses on creative work and calls `spex` for everything mechanical.

## Module Hierarchy

| Module | Purpose | Depends On |
|--------|---------|------------|
| Schema | JSON Schema for project.json + module.json | — |
| Validator | Spec directory validation (DAG, refs, orphans) | Schema |
| Merkle | Hash tree, snapshots, diff, impact classification | Schema |
| Impact | Map merkle diff to affected beads | Merkle |
| Apply | Execute bead actions via `br` CLI | Impact |
| Proposal | Proposal lifecycle (register, log, templates) | — |
| Render | Generate markdown, DOT, JSON from spec | Schema |

## Technical Constraints

- **Go standard library first**: Minimal external dependencies
- **Deterministic**: Same spec state + snapshot = same diff, impact, actions. No LLM calls in `spex`.
- **Composable**: Every subcommand reads stdin or files, writes stdout or files, exits 0/1. Pipeable.
- **Git-native**: Snapshots and proposals are files committed to git. No external state.

## Build & Test

- Build: `go build -o bin/ ./cmd/spex/`
- Test: `go test ./...`
- Vet: `go vet ./...`

## Git Conventions

- Default branch is `main` (never `master`)
- Always `git fetch origin` before creating a new branch
- Always branch from `origin/main`, not from the current branch

## Issue Tracking

This project uses `br` (beads_rust) for issue tracking and `bv` (beads_viewer) for task selection. Do NOT use markdown TODOs or other tracking methods.

- Find work: `bv --robot-next` or `br ready`
- Claim work: `br update <id> --status in_progress`
- Complete work: `br close <id>`
- Link PR: `br update <id> --external-ref "PR#<number>"`

## Organizational Constraints

- **Module dependency order**: Schema first, then Validator/Merkle, then Impact, then Apply
- **Spec traceability**: All code must trace back to bead requirements
- **Self-hosting**: Spex Machina's own spec is managed by Spex Machina (after bootstrap)

## Where to Find Details

- **Skills**: `skills/` — all skill definitions (`/propose`, `/spec`, `/implement`, `/review`, `/fix`)
- **Discovery**: `.claude/skills/` — symlinks for Claude Code slash commands
- **Proposal**: `spec/proposals/` — project and change proposals
- **Beads**: `.beads/` — task tracking database
