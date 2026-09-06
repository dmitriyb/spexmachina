#!/usr/bin/env bash
# lens-requirement-links.sh — spec-review mechanical lens: every module requirement is carried by
# an arch leaf.
#
# A requirement's behaviour is stated in the arch leaf of a component that implements it; a test
# leaf only asserts it. The 2026-09-06 class: unit-shaped scenarios had become the place a
# requirement's cases were enumerated, so deleting them looked like losing the requirement. This
# lens fails a module requirement that no implementing component's arch leaf links by id
# (`[[<id>|...]]`) — the structural half of /spec-review lens 8. Whether the leaf actually
# honours the claim is the judgment half and stays with the review.
#
# Requirements with no implementing component are the validator's business
# (RequirementCoverageChecker) and are not reported here.
#
# Exit 1 on any unlinked requirement; 0 otherwise. Output: one line per unlinked requirement.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0; checked=0
for mod_json in spec/*/module.json; do
    jq empty "$mod_json" || { echo "lens-requirement-links: unparseable $mod_json" >&2; exit 2; }
    mod_dir=$(dirname "$mod_json")
    while IFS=$'\t' read -r rid rname leaves; do
        [ -z "$leaves" ] && continue
        checked=$((checked+1))
        linked=0
        for leaf in $leaves; do
            [ -f "$mod_dir/$leaf" ] || continue
            grep -q -F "[[$rid|" "$mod_dir/$leaf" && { linked=1; break; }
        done
        if [ "$linked" -eq 0 ]; then
            echo "UNLINKED $mod_dir/module.json $rid \"$rname\" — implementing arch leaves: $leaves"
            fail=1
        fi
    done < <(jq -r '
        .components as $c
        | .requirements[]
        | .id as $r
        | [$c[] | select(.implements | index($r)) | .content] as $leaves
        | "\($r)\t\(.name)\t\($leaves | join(" "))"' "$mod_json")
done
[ "$fail" -eq 0 ] && echo "lens-requirement-links: $checked module requirements checked, every one linked from an implementing arch leaf"
exit "$fail"
