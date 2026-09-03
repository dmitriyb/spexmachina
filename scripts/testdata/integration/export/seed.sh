#!/usr/bin/env bash
set -euo pipefail
open1=$(br create --title "open one" --type feature --json | jq -r '.id')
open2=$(br create --title "open two" --type feature --json | jq -r '.id')
claimed=$(br create --title "claimed one" --type feature --json | jq -r '.id')
br update "$claimed" --status in_progress >/dev/null
closed1=$(br create --title "closed one" --type feature --json | jq -r '.id')
br close "$closed1" --force >/dev/null
closed2=$(br create --title "closed two" --type feature --json | jq -r '.id')
br close "$closed2" --force >/dev/null

printf '%s\n' "$open1" "$open2" > open_ids.txt
printf '%s\n' "$claimed" > claimed_id.txt
printf '%s\n' "$closed1" "$closed2" > closed_ids.txt
