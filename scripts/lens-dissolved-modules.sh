#!/usr/bin/env bash
# lens-dissolved-modules.sh — spec-review mechanical lens: dissolved module names.
#
# A module that was merged away or renamed leaves no trace any gate can see.
# The removal-time name sweep recovers removed *component* and *api* names by
# hashing corpus phrases, but a module is not one of those — when a whole
# module is retired `spex diff` says so itself, reporting `unverifiable_module`
# and leaving the prose sweep to a human. lens-lexicon.sh cannot cover the gap
# either: it matches terms declared under a proposal's "## Retired vocabulary"
# heading, and declaring a bare module name there would match every ordinary
# use of the word (`impact` alone hits 99 live sites in this corpus — the
# `Impact` field on ClassifiedChange, "impact classification", "Emit the ops").
#
# So this lens derives its own terms and searches only for forms in which a
# module name is being used as an ACTOR or a LOCATION, never as a word:
#
#   possessive     impact's bead-producing types
#   arrow chain    spec change → impact → bead actions  (also the \u2192
#                  escape, which is how an arrow reaches a module.json string)
#   qualifier      requirement '...' (impact) description changed
#   path fragment  spec/impact/arch_action_classifier.md
#   json field     "module": "impact"
#   noun phrase    the impact module   (this direction only: "per-module
#                  impact views" is ordinary English, not a reference)
#
# Terms are DERIVED, never declared: a module named by a `removed` event in
# spec/.history.jsonl that spec/project.json no longer lists has dissolved.
# Nothing to maintain — retiring a module registers it here automatically.
#
# Precision over recall, deliberately. A hit is always worth a look; a clean
# run does NOT mean the corpus is free of stale module references, because
# three forms match nothing and cannot be told from ordinary English: a module
# named as a bare subject ("so impact never sees it"), an adjectival compound
# ("impact analysis"), and a `<module>: <Component>` bead title ("emit:
# ChangesetBuilder"). The last two are often correct as written anyway.
#
# Names are read from the journal's `module` field verbatim and matched
# case-sensitively, as project.json declares them today (all lowercase).
#
# A hit that is CORRECT stays correct: a leaf documenting the journal has to
# quote a removal record, and a removal record names the module the removed node
# lived in — forever. Rewriting such an example to name a live module would make
# the documentation lie. Those lines are excused by name in the allow file, one
# entry per line as `<path-relative-to-spec-dir><TAB><substring>`, `#` for the
# reason. Both halves
# must match, so a NEW stale reference in an already-excused file still fires.
# Suppressions are counted in the output, never silent, and an entry that
# matches nothing is reported as dead so the list cannot quietly rot.
#
# Usage: lens-dissolved-modules.sh [<spec-dir>] [<allow-file>]
#        (defaults: spec, scripts/lens-dissolved-modules.allow)
#
# Exit 1 if any hit is found; 0 otherwise.
set -euo pipefail
cd "$(dirname "$0")/.."

SPEC_DIR="${1:-spec}"
SPEC_DIR="${SPEC_DIR%/}"
ALLOW="${2:-scripts/lens-dissolved-modules.allow}"
JOURNAL="$SPEC_DIR/.history.jsonl"
PROJECT="$SPEC_DIR/project.json"

if [ ! -f "$JOURNAL" ] || [ ! -f "$PROJECT" ]; then
    echo "lens-dissolved-modules: no journal or project.json; nothing to derive"
    exit 0
fi

# Modules named by a removed event that project.json no longer declares.
dissolved=$(python3 - "$JOURNAL" "$PROJECT" <<'PY'
import json, sys
journal, project = sys.argv[1], sys.argv[2]
live = {m.get("name") for m in json.load(open(project)).get("modules", [])}
seen = set()
with open(journal) as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
        except ValueError:
            continue                      # a malformed line costs one event, not the run
        if ev.get("event") == "removed" and ev.get("module"):
            seen.add(ev["module"])
for name in sorted(seen - live):
    print(name)
PY
)

if [ -z "$dissolved" ]; then
    echo "lens-dissolved-modules: no dissolved modules in the journal"
    exit 0
fi

# Allow entries: parallel arrays of path and substring, plus a used-flag so a
# dead entry can be reported.
allow_paths=(); allow_texts=(); allow_used=()
if [ -f "$ALLOW" ]; then
    while IFS=$'\t' read -r apath atext; do
        case "$apath" in ''|'#'*) continue ;; esac
        [ -z "${atext:-}" ] && continue
        allow_paths+=("$apath"); allow_texts+=("$atext"); allow_used+=(0)
    done < "$ALLOW"
fi

# excused <hit-line> — true if some entry matches this hit's path AND text.
# Paths are compared relative to the spec dir, so an entry reads
# `map/arch_mapping_store.md` and holds wherever the tree is rooted.
excused() {
    local line="$1" path="${1%%:*}" i
    path="${path#$SPEC_DIR/}"
    for i in "${!allow_paths[@]}"; do
        [ "${allow_paths[$i]}" = "$path" ] || continue
        case "$line" in *"${allow_texts[$i]}"*) allow_used[$i]=1; return 0 ;; esac
    done
    return 1
}

fail=0
suppressed=0
while IFS= read -r name; do
    [ -z "$name" ] && continue
    # A derived name is interpolated raw into an ERE. Validate its shape first:
    # spec module names are lowercase (the validator's name_consistency rule),
    # and a name carrying a regex metacharacter would otherwise either break the
    # pattern or — as `++` does under ugrep, which reads it as a possessive
    # quantifier — silently match the wrong thing and report a clean corpus.
    if ! [[ "$name" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
        echo "lens-dissolved-modules: refusing to sweep for '$name': not a plain lowercase module name" >&2
        exit 2
    fi

    # Actor/location forms only — see the header. \b keeps `map` from matching
    # `mapping`, and the proposals directory is the historical record, exempt
    # by the same rule lens-lexicon.sh applies.
    arrow='(→|->|\\u2192)'
    # `?<name>`? — spec prose backticks identifiers routinely, and a closing
    # backtick would otherwise block every whitespace- and apostrophe-anchored
    # form below.
    n="\`?${name}\`?"
    pattern="\\b${name}\`?('|’)s\\b|${arrow}[[:space:]]*${n}\\b|\\b${n}[[:space:]]*${arrow}|\\(${n}\\)|\\b${name}/|\"module\"[[:space:]]*:[[:space:]]*\"${name}\"|\\b${n}[[:space:]]+module\\b"
    # A derived name is interpolated into an ERE, so a name carrying a regex
    # metacharacter would make the pattern invalid. grep says so on exit >= 2;
    # swallowing that would turn a broken sweep into a clean report.
    set +e
    raw=$(grep -rnE --include='*.md' --include='*.json' -- "$pattern" "$SPEC_DIR/" 2>&1)
    rc=$?
    set -e
    if [ "$rc" -ge 2 ]; then
        echo "lens-dissolved-modules: grep failed for '$name' (exit $rc): $raw" >&2
        exit 2
    fi
    hits=$(grep -v "^$SPEC_DIR/proposals/" <<<"$raw" || true)
    kept=""
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        if excused "$line"; then
            suppressed=$((suppressed+1))
        else
            kept+="$line"$'\n'
        fi
    done <<<"$hits"
    if [ -n "$kept" ]; then
        echo "DISSOLVED MODULE '$name' named as a live actor or location:"
        printf '%s' "$kept" | sed 's/^/  /'
        fail=1
    fi
done <<<"$dissolved"

for i in "${!allow_paths[@]}"; do
    if [ "${allow_used[$i]}" -eq 0 ]; then
        echo "lens-dissolved-modules: dead allow entry — ${allow_paths[$i]} no longer contains '${allow_texts[$i]}'" >&2
    fi
done

note=""
[ "$suppressed" -gt 0 ] && note=" ($suppressed excused by $ALLOW)"
[ "$fail" -eq 0 ] && echo "lens-dissolved-modules: corpus clean of dissolved module references$note"
[ "$fail" -eq 1 ] && [ "$suppressed" -gt 0 ] && echo "lens-dissolved-modules: $suppressed further hit(s) excused by $ALLOW"
exit "$fail"
