#!/usr/bin/env bash
#
# run-pipeline.sh — Run the spex deterministic pipeline for a proposal.
#
# Stages: validate → diff → impact → emit → adapter → ingest
#
# Usage:
#   run-pipeline.sh --phase emit  --proposal <stem>  [--run-dir <dir>]
#   run-pipeline.sh --phase apply --run-dir <dir>
#   run-pipeline.sh --phase full  --proposal <stem>  [--run-dir <dir>]
#
# Phases:
#   emit  — validate, diff, impact, emit. Writes changeset.json into run-dir.
#   apply — adapter, ingest. Consumes changeset.json from --run-dir.
#   full  — emit + apply in one invocation. Use only when no review pause is needed.
#
# Proposal stem:
#   The filename of the proposal markdown without the .md suffix, e.g.
#   2026-04-27-pipeline-cleanup-and-refresh-mode for
#   spec/proposals/2026-04-27-pipeline-cleanup-and-refresh-mode.md
#
# Exit codes:
#   0   success
#   1   pre-flight failure (bad args, on main, missing binary, missing proposal)
#   10  validate or diff failure (errors array non-empty, IO failure)
#   11  impact failure
#   12  emit failure
#   13  adapter failure
#   14  ingest failure (1 = input error, 2 = invariant — both surface as 14 here)
#
# Final stdout line is always: "RUN_DIR=<absolute path>"
# All stage artifacts go under that directory:
#   validate.json, diff.json, impact.json, changeset.json, receipts.json,
#   ingest_summary.json, log
#
# This script never commits and never prompts. Pause-and-review is the
# caller's responsibility (typically /converge between phase emit and phase apply).

set -euo pipefail

PHASE=full
PROPOSAL=
RUN_DIR=

usage() {
    sed -n '2,40p' "$0" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --phase)    PHASE="$2"; shift 2 ;;
        --proposal) PROPOSAL="$2"; shift 2 ;;
        --run-dir)  RUN_DIR="$2"; shift 2 ;;
        --help|-h)  usage ;;
        *) echo "error: unknown arg: $1" >&2; usage ;;
    esac
done

case "$PHASE" in
    emit|apply|full) ;;
    *) echo "error: --phase must be emit|apply|full, got: $PHASE" >&2; exit 1 ;;
esac

# ----- Pre-flight ----------------------------------------------------------

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" \
    || { echo "error: not a git repository" >&2; exit 1; }
cd "$REPO_ROOT"

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$BRANCH" == "main" ]]; then
    echo "error: refusing to run on main; switch to a feature branch" >&2
    exit 1
fi

SPEX="$REPO_ROOT/bin/spex"
if [[ ! -x "$SPEX" ]]; then
    echo "info: bin/spex not found, building..." >&2
    if ! go build -o bin/spex ./cmd/spex/ >&2; then
        echo "error: build failed" >&2
        exit 1
    fi
fi

if ! command -v br >/dev/null 2>&1; then
    echo "error: br not on PATH" >&2
    exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq not on PATH" >&2
    exit 1
fi

if [[ "$PHASE" == "emit" || "$PHASE" == "full" ]]; then
    if [[ -z "$PROPOSAL" ]]; then
        echo "error: --proposal required for phase '$PHASE'" >&2
        exit 1
    fi
    PROPOSAL_FILE="spec/proposals/${PROPOSAL}.md"
    if [[ ! -f "$PROPOSAL_FILE" ]]; then
        echo "error: proposal file not found: $PROPOSAL_FILE" >&2
        exit 1
    fi
fi

if [[ -z "$RUN_DIR" ]]; then
    if [[ "$PHASE" == "apply" ]]; then
        echo "error: --run-dir required for phase 'apply'" >&2
        exit 1
    fi
    RUN_DIR="$REPO_ROOT/.spex/run-$(date -u +%Y%m%dT%H%M%SZ)"
fi

mkdir -p "$RUN_DIR"
RUN_DIR="$(cd "$RUN_DIR" && pwd)"

if [[ "$PHASE" == "apply" ]]; then
    if [[ ! -f "$RUN_DIR/changeset.json" ]]; then
        echo "error: $RUN_DIR/changeset.json not found; emit phase did not produce it" >&2
        exit 1
    fi
fi

LOG="$RUN_DIR/log"
: > "$LOG"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" >> "$LOG"; }

# ----- Stage helpers -------------------------------------------------------

stage_validate() {
    log "validate: start"
    if ! "$SPEX" validate > "$RUN_DIR/validate.json" 2>>"$LOG"; then
        log "validate: failed"
        echo "error: spex validate failed; see $RUN_DIR/log" >&2
        exit 10
    fi
    if ! jq -e '.valid == true' "$RUN_DIR/validate.json" >/dev/null; then
        local n
        n=$(jq -r '.error_count' "$RUN_DIR/validate.json")
        log "validate: spec invalid ($n errors)"
        echo "error: spex validate reported $n error(s); see $RUN_DIR/validate.json" >&2
        exit 10
    fi
    log "validate: ok"
}

stage_diff() {
    log "diff: start"
    if ! "$SPEX" diff --json > "$RUN_DIR/diff.json" 2>>"$LOG"; then
        log "diff: failed"
        echo "error: spex diff failed; see $RUN_DIR/log" >&2
        exit 10
    fi
    local err_count
    err_count=$(jq -r '.errors | length' "$RUN_DIR/diff.json")
    if [[ "$err_count" -gt 0 ]]; then
        log "diff: $err_count completeness error(s)"
        echo "error: spex diff has $err_count completeness error(s); see $RUN_DIR/diff.json" >&2
        echo "       run /spec to address — /converge does not auto-fix completeness findings" >&2
        exit 10
    fi
    log "diff: ok ($(jq -r '.changes | length' "$RUN_DIR/diff.json") changes)"
}

stage_impact() {
    log "impact: start"
    # Use .beads/issues.jsonl, NOT `br list --json`. Reasons:
    #   1. br list defaults to open-only (`--all` flag needed to include closed),
    #      and impact needs closed beads to classify removed+closed → cleanup.
    #   2. br list defaults to --limit 50 with pagination; impact would silently
    #      truncate for any project with > 50 beads.
    #   3. issues.jsonl is the source-of-truth reflection (the db is rebuilt
    #      from it on recovery — see CLAUDE.md and feedback memory).
    # The shape impact expects is `{"issues": [...]}` per impact/bead_reader.go;
    # jq -s wraps the line-delimited records into that envelope.
    local beads
    beads="$RUN_DIR/beads.json"
    if ! jq -s '{issues: .}' .beads/issues.jsonl > "$beads" 2>>"$LOG"; then
        log "impact: jq jsonl wrap failed"
        echo "error: failed to read .beads/issues.jsonl; see $RUN_DIR/log" >&2
        exit 11
    fi
    if ! "$SPEX" impact \
            --diff "$RUN_DIR/diff.json" \
            --beads "$beads" \
            --json > "$RUN_DIR/impact.json" 2>>"$LOG"; then
        log "impact: failed"
        echo "error: spex impact failed; see $RUN_DIR/log" >&2
        exit 11
    fi
    log "impact: ok ($(jq -r '.issues | length' "$beads") beads in tracker view)"
}

stage_emit() {
    log "emit: start"
    local git_head
    git_head="$(git rev-parse HEAD)"
    if ! "$SPEX" emit \
            --impact "$RUN_DIR/impact.json" \
            --proposal "$PROPOSAL" \
            --git-head "$git_head" \
            --out "$RUN_DIR/changeset.json" 2>>"$LOG"; then
        log "emit: failed"
        echo "error: spex emit failed; see $RUN_DIR/log" >&2
        exit 12
    fi
    log "emit: ok ($(jq -r '.ops | length' "$RUN_DIR/changeset.json") ops, git_head=$git_head)"
}

stage_adapter() {
    log "adapter: start"
    if ! "$REPO_ROOT/scripts/apply-br.sh" "$RUN_DIR/changeset.json" "$RUN_DIR/receipts.json" 2>>"$LOG"; then
        log "adapter: failed"
        echo "error: adapter failed; see $RUN_DIR/log and $RUN_DIR/receipts.json" >&2
        exit 13
    fi
    log "adapter: ok (status=$(jq -r '.status' "$RUN_DIR/receipts.json"))"
}

stage_ingest() {
    log "ingest: start"
    local rc=0
    "$SPEX" ingest \
        --changeset "$RUN_DIR/changeset.json" \
        --receipts "$RUN_DIR/receipts.json" > "$RUN_DIR/ingest_summary.json" 2>>"$LOG" || rc=$?
    if [[ "$rc" -ne 0 ]]; then
        log "ingest: failed (exit $rc)"
        echo "error: spex ingest failed (exit $rc); see $RUN_DIR/log" >&2
        exit 14
    fi
    log "ingest: ok"
}

# ----- Dispatch ------------------------------------------------------------

log "phase=$PHASE proposal=${PROPOSAL:-<n/a>} branch=$BRANCH run_dir=$RUN_DIR"

case "$PHASE" in
    emit|full)
        stage_validate
        stage_diff
        stage_impact
        stage_emit
        ;;
esac

case "$PHASE" in
    apply|full)
        stage_adapter
        stage_ingest
        ;;
esac

log "phase=$PHASE complete"
echo "RUN_DIR=$RUN_DIR"
