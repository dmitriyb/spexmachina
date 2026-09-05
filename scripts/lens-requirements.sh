#!/usr/bin/env bash
# lens-requirements.sh — spec-review mechanical lens: requirements vs the
# leaves that implement them.
#
# For every component in scope, pairs each requirement it implements (and
# the project requirement behind it) with the component's arch leaf and the
# test leaves that describe it. The reviewer reads each pair and judges
# whether the leaves still honour every claim the requirement makes — a
# requirement's promise that no leaf keeps any more is a critical finding
# (the "with their dates" class: an arch leaf rewritten for another purpose
# dropped a field the requirement still asks for, and nothing re-read the
# requirement). Report-only: always exits 0; the output is a worksheet, not
# a verdict. Arguments name modules; none means every module project.json
# lists.
set -euo pipefail
cd "$(dirname "$0")/.."

PROJECT=spec/project.json
if [[ $# -gt 0 ]]; then
    modules=("$@")
else
    mapfile -t modules < <(jq -r '.modules[].name' "$PROJECT")
fi

pairs=0
for mod in "${modules[@]}"; do
    path=$(jq -r --arg m "$mod" '.modules[] | select(.name==$m) | .path' "$PROJECT")
    if [[ -z "$path" ]]; then
        echo "lens-requirements: no module named '$mod' in $PROJECT" >&2
        continue
    fi
    MJ="spec/$path/module.json"
    echo "== module $mod =="
    while IFS=$'\t' read -r cid cname content; do
        echo "component $cname ($cid)"
        echo "  arch:  spec/$path/$content"
        jq -r --arg c "$cid" \
            '.test_sections[]? | select(.describes | index($c)) | "  test:  spec/'"$path"'/\(.content)"' "$MJ"
        while IFS=$'\t' read -r rid rname preq rdesc; do
            [[ -z "$rid" ]] && continue
            pairs=$((pairs + 1))
            echo "  req    $rid  $rname"
            echo "         $rdesc"
            if [[ -n "$preq" && "$preq" != "null" ]]; then
                jq -r --arg p "$preq" \
                    '.requirements[] | select(.id==$p) | "    preq \(.id)  \(.name)\n         \(.description)"' "$PROJECT"
            fi
        done < <(jq -r --arg c "$cid" '
            (.components[] | select(.id==$c) | .implements // [])[] as $r
            | .requirements[] | select(.id==$r)
            | [.id, .name, (.preq_id // ""), .description] | @tsv' "$MJ")
        echo
    done < <(jq -r '.components[] | [.id, .name, .content] | @tsv' "$MJ")
done
echo "pairs: $pairs (component x requirement)"
exit 0
