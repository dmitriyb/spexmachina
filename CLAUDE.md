# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Spex Machina is a standalone CLI (`spex`) that owns the structural half of spec-driven development. It defines specs as a typed graph (JSON skeleton + markdown content leaves), tracks changes via a merkle tree, computes impact deterministically, and maps spec nodes to beads tasks. The LLM focuses on creative work and calls `spex` for everything mechanical.

## Module Hierarchy

The pipeline is `spex validate → diff → impact → emit → <adapter> → ingest`.

| Module | Purpose | Depends On |
|--------|---------|------------|
| Schema | JSON Schema for project.json + module.json + bead-map | — |
| Validator | Spec directory validation (DAG, refs, orphans, coverage) | Schema |
| Merkle | Hash tree, snapshots, diff, impact classification | Schema |
| Impact | Map merkle diff to bead actions | Merkle |
| Emit | Compose a tool-agnostic changeset from an impact report | Impact, Mapping |
| Adapters | Contract for external adapters (`scripts/apply-br.sh` is the reference) that apply a changeset and write receipts | Emit |
| Ingest | Reconcile the bead-map + save the snapshot from changeset+receipts; `--mode refresh` absorbs content-only drift | Emit, Adapters, Merkle |
| Mapping | `.bead-map.json` store linking spec nodes to beads (`spex map context`) | Schema |
| Proposal | Proposal lifecycle (register, log, templates) | — |
| Render | Generate markdown, DOT, JSON from spec | Schema |

## Technical Constraints

- **Go standard library first**: Minimal external dependencies.
- **Deterministic**: Same spec state + snapshot = same diff, impact, changeset. No LLM calls in `spex`.
- **Composable**: Every subcommand reads stdin or files, writes stdout or files, exits with documented codes. Pipeable.
- **Git-native**: Snapshots and proposals are files committed to git. No external state.

## Build & Test

- Build: `go build -o bin/ ./cmd/spex/`
- Test: `go test ./...`
- Vet: `go vet ./...`

## Git Conventions

- Default branch is `main` (never `master`). `main` is protected: land changes via a branch + PR, not direct commits.
- Commits are SSH-signed. To verify a commit is actually signed, inspect the raw object for a `gpgsig` block: `git cat-file -p <sha>`. Do NOT trust `git log --show-signature` or `%G?` — both report `N`/"No signature" on correctly-signed commits when `gpg.ssh.allowedSignersFile` is not configured locally; that is a verification-side gap, not a signing failure.
- Commit messages: state what changed and, only if non-obvious, why. Keep them short (≈two sentences).
- For "current state" of any committed artifact, read from `origin/main` (`git show origin/main:<path>`) rather than a stale branch's working tree.
- For rebase-risk, read `git log <base>..<branch> --oneline` and `git show --stat <sha>` per commit — not `git diff <base>..<branch> --stat`, which overstates conflict surface.

## Issue Tracking

This project uses `br` (beads_rust) for issue tracking and `bv` (beads_viewer) for task selection. Do NOT use markdown TODOs.

- Find work: `bv --robot-next` or `br ready`
- Claim work: `br update <id> --status in_progress`
- Link PR: `br update <id> --external-ref "PR#<number>"`
- Close work: only after review approval — `br close <id> --reason "..."`. In the automated pipeline this is the reviewer role's job and is gated at the git boundary (see Enforcement); do not close beads you implemented.
- `br list --json` is NOT canonical: it filters to open-only and paginates at `--limit 50`, both silently. For full tracker state, read `.beads/issues.jsonl`: `jq -s '{issues: .}' .beads/issues.jsonl`. If you must use `br list`, pass `--all --limit 0`.

## Bead Context Resolution

Beads carry empty `description`/`design`/`acceptance_criteria`/`notes` by design. The bead's title plus its `spex:<record_id>` label is the entry point — the spec files are the source of truth.

1. `br show <bead-id> --json` — find the `spex:<record_id>` label.
2. `bin/spex map context <record_id>` — returns the record plus `arch_file`, `impl_files`, `test_files`, `flow_files`, `module_file`.
3. Read those spec files. Do not hunt elsewhere for a bead's "real" description — there isn't one.

Hard rules:
- Never read `.beads/beads.db` directly (it is a reflection of `.beads/issues.jsonl`, rebuilt from it). Use `br` commands or read `issues.jsonl`.
- Cleanup beads (label `spex:cleanup`) have no map record by design.

### Recovery: undoing a `br` mutation

The db is the source of truth; `issues.jsonl` is its reflection. To unwind a `br` mutation cleanly: `git checkout -- .beads/issues.jsonl`, then `rm .beads/beads.db`, then re-import from the restored jsonl to rebuild the db. `git checkout` alone leaves the db holding the mutated state.

## Organizational Constraints

- **Module dependency order**: Schema first, then Validator/Merkle, then Impact, then Emit → Adapters → Ingest.
- **Spec traceability**: All code traces back to bead requirements.
- **Self-hosting**: Spex Machina's own spec is managed by Spex Machina.

## Enforcement

Enforcement is **external** and lives outside this repo — there are no in-repo Claude Code or git hooks.

- **portitor** (git gateway) is the hard result-gate: agents push over SSH to a gated mirror, and a `pre-receive` gate verifies signature → signer role → branch/ancestry/content rules before anything reaches GitHub. portitor holds the only GitHub credential. The bead-close rule (only the reviewer/owner role may add `"status":"closed"` to `.beads/issues.jsonl`) is a portitor content rule.
- **faber** orchestrates the implement/review/fix boxes; each box runs one skill headlessly in an isolated container and lands its branch through portitor.

Interactive (human) sessions push to GitHub directly and are the design/authoring entrypoint; the gate constrains the autonomous agents. What used to be in-repo hooks/skills and where each rule moved is recorded in `docs/enforcement-migration.md`.

## Where to Find Details

- **Authoring skills**: `skills/` — `/propose` (draft a proposal in plan mode), `/spec` (author spec files), `/spec-review` (audit spec internal consistency). The implement/review/fix/cleanup lifecycle runs through faber boxes, not slash skills.
- **Proposals**: `spec/proposals/`
- **Beads**: `.beads/`
- **Pipeline runner**: `scripts/run-pipeline.sh`; reference adapter `scripts/apply-br.sh`.
- **Enforcement migration**: `docs/enforcement-migration.md`
