#!/usr/bin/env bash
# block-signing-bypass.sh — R5b (signing-flag-denied)
#
# Blocks Bash commands that bypass SSH signing:
#   git commit --no-gpg-sign
#   git -c commit.gpgsign=false commit
#   git -c commit.gpgsign=0 commit (rare but possible)
#
# CLAUDE.md:40 — Never bypass signing.

set -uo pipefail
source "$(dirname "$0")/lib/emit-halt.sh"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<<"$input")"
[[ "$tool" != "Bash" ]] && exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
[[ -z "$command" ]] && exit 0

# Strip heredoc bodies — documentation mentioning these flags inside a
# `git commit -m "$(cat <<EOF ... EOF)"` body must not trip the hook.
stripped="$(strip_heredoc_bodies "$command")"

# Match either: `--no-gpg-sign` flag, or `-c commit.gpgsign=` set to
# anything other than `true` (matches false/0/no/off).
if [[ "$stripped" =~ --no-gpg-sign ]] \
   || [[ "$stripped" =~ -c[[:space:]]+commit\.gpgsign=(false|0|no|off|FALSE) ]] \
   || [[ "$stripped" =~ -c[[:space:]]+commit\.gpgsign[[:space:]]*=[[:space:]]*(false|0|no|off|FALSE) ]]; then
  emit_halt \
    "signing-flag-denied" \
    "$command" \
    "Commits must be SSH-signed. Never bypass signing via --no-gpg-sign or -c commit.gpgsign=false." \
    "CLAUDE.md:40" \
    "Re-run the command without the bypass flag" false \
    "If signing is genuinely broken, fix the config (git config --global commit.gpgsign true) and the signing key (user.signingkey)" false
fi
exit 0
