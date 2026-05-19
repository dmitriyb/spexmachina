#!/usr/bin/env bash
# test-active-skill.sh — verify active_skill() returns the marker
# value when fresh, empty when missing, malformed, or stale.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
source "$repo_root/scripts/hooks/lib/active-skill.sh"

marker="$repo_root/.claude/skill-context.json"
backup=""

# Save existing marker (if any) so we don't disrupt a real session.
if [[ -f "$marker" ]]; then
  backup="$(mktemp)"
  cp "$marker" "$backup"
fi

restore() {
  if [[ -n "$backup" ]]; then
    mv "$backup" "$marker"
  else
    rm -f "$marker"
  fi
}
trap restore EXIT

# Test 1: missing marker → empty
rm -f "$marker"
got="$(active_skill)"
if [[ -n "$got" ]]; then
  echo "FAIL test1: missing marker should yield empty, got '$got'" >&2
  exit 1
fi

# Test 2: fresh marker → skill name
printf '{"skill":"review","started_at":"%s","pid":%d}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > "$marker"
got="$(active_skill)"
if [[ "$got" != "review" ]]; then
  echo "FAIL test2: fresh marker should yield 'review', got '$got'" >&2
  exit 1
fi

# Test 3: stale marker → empty (TTL=0 forces stale)
got="$(SPEX_SKILL_TTL=0 active_skill)"
if [[ -n "$got" ]]; then
  echo "FAIL test3: TTL=0 should yield empty (stale), got '$got'" >&2
  exit 1
fi

# Test 4: marker with started_at older than default TTL → empty
printf '{"skill":"spec","started_at":"%s","pid":%d}\n' \
  "$(date -u -d '2 hours ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
     || date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > "$marker"
got="$(active_skill)"
if [[ -n "$got" ]]; then
  echo "FAIL test4: 2-hour-old marker should be stale, got '$got'" >&2
  exit 1
fi

# Test 5: malformed marker → empty (no crash)
echo "not json" > "$marker"
got="$(active_skill)"
if [[ -n "$got" ]]; then
  echo "FAIL test5: malformed marker should yield empty, got '$got'" >&2
  exit 1
fi

# Test 6: marker with empty skill field → empty
printf '{"skill":"","started_at":"%s","pid":%d}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > "$marker"
got="$(active_skill)"
if [[ -n "$got" ]]; then
  echo "FAIL test6: empty skill field should yield empty, got '$got'" >&2
  exit 1
fi

echo "ok"
