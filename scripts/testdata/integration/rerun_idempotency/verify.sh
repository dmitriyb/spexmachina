#!/usr/bin/env bash
set -euo pipefail
# The second run is fully idempotent: every create matches its label, the
# close hits the already-closed skip branch, and nothing in the tracker
# moves — the snapshot seed.sh took right after the first run must be
# byte-identical to the tracker's state now.
br list --json --all --limit 0 | jq -S . > state_after_run2.json
diff state_after_run1.json state_after_run2.json || { echo "tracker state changed on re-run"; exit 1; }
