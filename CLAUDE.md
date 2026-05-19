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
- `main` is protected: never commit directly to it. All changes land via a dedicated branch + PR merge, even one-line data fixes.
- Always `git fetch origin` before creating a new branch
- Always branch from `origin/main`, not from the current branch
- Commits must be SSH-signed. Never bypass signing (`--no-gpg-sign`, `-c commit.gpgsign=false`). To verify a commit is actually signed, inspect the raw object for a `gpgsig` block: `git cat-file -p <sha>`. Do NOT trust `git log --show-signature` or `%G?` — both report `N`/"No signature" on correctly-signed commits when `gpg.ssh.allowedSignersFile` is not configured locally. That is a verification-side gap, not a signing failure.

## Issue Tracking

This project uses `br` (beads_rust) for issue tracking and `bv` (beads_viewer) for task selection. Do NOT use markdown TODOs or other tracking methods.

- Find work: `bv --robot-next` or `br ready`
- Claim work: `br update <id> --status in_progress`
- Link PR: `br update <id> --external-ref "PR#<number>"`
- Close work: performed by `/review` after LGTM, never by `/implement`. The command is `br close <id> --reason "Reviewed and approved in PR#<number>. All review feedback addressed."` — do not run it from any other skill or context.

## Bead Context Resolution

Beads in this repo intentionally carry empty `description`, `design`, `acceptance_criteria`, and `notes` fields. The bead's title plus its `spex:<record_id>` label is the entry point — the spec files are the source of truth.

To resolve the spec context for any bead (review, fix, implement, ad-hoc):

1. `br show <bead-id> --json` — read the bead, find the `spex:<record_id>` label in `labels`.
2. `bin/spex map context <record_id>` — returns the JSON record plus the `arch_file`, `impl_files`, `test_files`, `flow_files`, and `module_file` paths.
3. Read those spec files. Do not hunt elsewhere for the bead's "real" description — there isn't one.

Hard rules:
- Never read `.beads/beads.db` directly (sqlite3, python sqlite3, raw file reads, etc.). Use `br` commands or `.beads/issues.jsonl`.
- Never bypass the documented `br` / `spex` interfaces to dig into their storage. If `br` says a field is empty, treat it as empty.
- If you need bead/spec data and the documented tools don't expose it, ask the user — do not improvise with general-purpose tools.

Cleanup beads (carrying the `spex:cleanup` label) have no map record by design — see `/cleanup` for that workflow.

## Organizational Constraints

- **Module dependency order**: Schema first, then Validator/Merkle, then Impact, then Apply
- **Spec traceability**: All code must trace back to bead requirements
- **Self-hosting**: Spex Machina's own spec is managed by Spex Machina (after bootstrap)

## Enforcement

The repo ships machine-enforced rules via Claude Code hooks
(`.claude/settings.json` + `scripts/hooks/`) and git hooks
(`scripts/git-hooks/`). Full design in `docs/enforcement-rfc.md`.

**One-time per clone:** `make setup-hooks`. This wires
`core.hooksPath` and makes the scripts executable.

**Halt protocol — when a hook denies a tool call.** Claude Code
surfaces a `permissionDecisionReason` string. If the string parses as
JSON with `"protocol": "spex-halt/v1"`, the agent MUST:

1. Halt the current tool sequence. No further tools on the same
   goal-path until the user has responded.
2. Render the payload to the user verbatim, including: the `rule`
   slug, the `invariant`, the `source` (file:line), and every entry
   in `recovery`.
3. Propose a recovery path drawn from `recovery`, but do not execute.
4. Destructive recovery steps (`destructive: true` entries, plus the
   syntactic backstop list: `rm`, `rm -rf`, `git reset --hard`,
   `git checkout -- <file>`, `git clean -f`, `git branch -D`,
   force-push, `br delete`, any DB wipe, any operation that
   overwrites unstaged work) require explicit per-step user
   confirmation. Never bundle destructive steps.
5. Non-destructive recovery (file edits, `git add`, `git stash`,
   `git fetch`, `br update`, additional `Read`s) may proceed once the
   user acknowledges the block and names the path.

**Skill identity.** Hooks consult `.claude/skill-context.json` to know
which skill is active. Each skill MUST write this marker as its first
action:

```bash
printf '{"skill":"<name>","started_at":"%s","pid":%d}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > .claude/skill-context.json
```

Hooks treat a missing/stale (>60 min) marker as "skill unknown" and
apply their per-rule fail-mode (R6/R12 block; R9 allows on the
assumption the commit is user-driven). See `docs/enforcement-rfc.md`
§9.1 for the asymmetry.

**Override env vars.** Two narrow overrides exist, both user-set:
- `SPEX_REBASELINE=1` permits `spex hash` (R12 carve-out).
- `SPEX_OFFLINE=1` skips R3's fetch-recency check.
The agent must not set either; if the agent believes an override is
warranted, it asks the user.

**Before opening a PR** that touches `.claude/settings.json`,
`scripts/hooks/`, or `scripts/git-hooks/`: run `make verify-enforcement`.

## Where to Find Details

- **Skills**: `skills/` — all skill definitions (`/propose`, `/spec`, `/implement`, `/review`, `/fix`)
- **Discovery**: `.claude/skills/` — symlinks for Claude Code slash commands
- **Proposal**: `spec/proposals/` — project and change proposals
- **Beads**: `.beads/` — task tracking database
- **Enforcement**: `docs/enforcement-rfc.md` — hook design, catalog, halt protocol
