#!/usr/bin/env bash
# lens-usage-strings.sh — spec-review mechanical lens: CLI usage strings in
# module.json descriptions vs the owning arch leaf's flag vocabulary.
#
# For every component and api description in every module.json that mentions a
# `--flag`, assert every such flag also appears in the owning arch leaf
# (JSON -> leaf direction only). A flag present in the JSON description but
# absent from the leaf is exactly the class of defect PR #224 fixed in-flight:
# `--map` alive in a description while the leaf's flag table had already
# retired it.
#
# Exit 1 if any mismatch is found; 0 otherwise. Output: one line per mismatch.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0
checked=0
for mod_json in spec/*/module.json; do
    jq empty "$mod_json" || { echo "lens-usage-strings: unparseable $mod_json" >&2; exit 2; }
    mod_dir=$(dirname "$mod_json")
    # components: description + content leaf
    while IFS=$'\t' read -r name content desc; do
        [ -z "$desc" ] && continue
        json_flags=$(grep -oE -- '--[a-z][a-z0-9-]+' <<<"$desc" | sort -u || true)
        [ -z "$json_flags" ] && continue
        leaf="$mod_dir/$content"
        [ -f "$leaf" ] || continue
        leaf_flags=$(grep -oE -- '--[a-z][a-z0-9-]+' "$leaf" | sort -u || true)
        while read -r f; do
            [ -z "$f" ] && continue
            if ! grep -qxF -- "$f" <<<"$leaf_flags"; then
                echo "MISMATCH: $mod_json component '$name' description mentions $f; $leaf never mentions it"
                fail=1
            fi
        done <<<"$json_flags"
        checked=$((checked+1))
    done < <(jq -r '.components[]? | [.name, .content, .description // ""] | @tsv' "$mod_json")
    # apis: description vs provided_by components' leaves (union)
    while IFS=$'\t' read -r name provided desc; do
        [ -z "$desc" ] && continue
        json_flags=$(grep -oE -- '--[a-z][a-z0-9-]+' <<<"$desc" | sort -u || true)
        [ -z "$json_flags" ] && continue
        leaves=$(jq -r --arg p "$provided" '.components[]? | select(.id == ($p | split(",")[])) | .content' "$mod_json" 2>/dev/null || true)
        union=""
        while read -r c; do
            [ -f "$mod_dir/$c" ] && union+=$'\n'"$(grep -oE -- '--[a-z][a-z0-9-]+' "$mod_dir/$c" || true)"
        done <<<"$leaves"
        union=$(sort -u <<<"$union")
        while read -r f; do
            [ -z "$f" ] && continue
            if ! grep -qxF -- "$f" <<<"$union"; then
                echo "MISMATCH: $mod_json api '$name' description mentions $f; no provided_by leaf mentions it"
                fail=1
            fi
        done <<<"$json_flags"
        checked=$((checked+1))
    done < <(jq -r '.apis[]? | [.name, ((.provided_by // []) | join(",") | if . == "" then "-" else . end), .description // ""] | @tsv' "$mod_json")
done
[ "$fail" -eq 0 ] && echo "lens-usage-strings: no mismatches ($checked descriptions checked)"
exit "$fail"
