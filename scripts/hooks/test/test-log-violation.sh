#!/usr/bin/env bash
# test-log-violation.sh — verify log-violation appends a JSON line
# with a ts field to .claude/hook-violations.log.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log_file="$repo_root/.claude/hook-violations.log"

# Snapshot existing line count (file may not exist yet).
before=0
if [[ -f "$log_file" ]]; then
  before="$(wc -l < "$log_file")"
fi

# Feed a known inner payload.
printf '%s\n' '{"protocol":"spex-halt/v1","rule":"test","invariant":"x","source":"y"}' \
  | "$repo_root/scripts/hooks/log-violation"

after="$(wc -l < "$log_file")"
if (( after != before + 1 )); then
  echo "log line count did not increase by 1 (before=$before after=$after)" >&2
  exit 1
fi

# Last line must parse as JSON with ts and the original fields.
tail -1 "$log_file" | jq -e '
  .ts | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
' >/dev/null

tail -1 "$log_file" | jq -e '
  .protocol == "spex-halt/v1" and .rule == "test"
' >/dev/null

echo "ok"
