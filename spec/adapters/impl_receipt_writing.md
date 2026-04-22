# Receipt writing

How the adapter emits receipts.json v1 after processing all ops.

## In-Memory Accumulation

```bash
RECEIPTS=()

append_receipt() {
    local op_id="$1" status="$2" bead_id="$3" was_existing="$4" detail="$5"
    local entry
    entry=$(jq -cn \
        --arg op "$op_id" --arg st "$status" --arg bid "$bead_id" \
        --argjson we "$was_existing" --arg dt "$detail" \
        '{op_id: $op, status: $st, bead_id: $bid, was_existing: $we} + ($dt | if . == "" then {} else {reason: .} end)')
    RECEIPTS+=("$entry")
}
```

For `status=error` entries, `detail` is placed in an `error` field instead of `reason`:

```bash
append_error_receipt() {
    local op_id="$1" bead_id="$2" err="$3"
    RECEIPTS+=("$(jq -cn --arg op "$op_id" --arg bid "$bead_id" --arg e "$err" \
        '{op_id: $op, status: "error", bead_id: $bid, was_existing: false, error: $e}')")
}
```

## Final Assembly

```bash
emit_receipts_json() {
    # Determine top-level status
    local top_status="complete"
    for r in "${RECEIPTS[@]}"; do
        local s=$(jq -r '.status' <<< "$r")
        if [[ "$s" == "error" ]]; then
            top_status="partial"
            break
        fi
    done

    # Assemble v1 wrapper
    local out
    out=$(jq -n --argjson ops "[$(IFS=,; echo "${RECEIPTS[*]}")]" --arg st "$top_status" \
        '{version: 1, status: $st, ops: $ops}')

    # Write atomically
    if [[ -n "${RECEIPTS_OUT:-}" ]]; then
        local tmp="${RECEIPTS_OUT}.tmp"
        echo "$out" | jq . > "$tmp"
        mv "$tmp" "$RECEIPTS_OUT"
    else
        echo "$out" | jq .
    fi
}
```

## Status Determination

- If ANY op's status is `error` → top-level `partial`.
- If no errors but some ops `skipped` (is that still complete?): YES — skipped means "adapter intentionally did nothing, this is not a failure." Top-level stays `complete`.
- If the adapter itself crashes before finishing, no receipts file is written — the caller must treat the absence of receipts.json as equivalent to `partial` with unknown progress.

## Receipt Shape

v1 schema (documented here since there is no separate `.schema.json` for it yet — that can be a follow-up):

```json
{
  "version": 1,
  "status": "complete" | "partial",
  "ops": [
    {
      "op_id": "op-0001",
      "status": "ok" | "skipped" | "error",
      "bead_id": "<id or empty>",
      "was_existing": true | false,
      "reason": "<optional; for skipped>",
      "error": "<optional; for error>"
    }
  ]
}
```

Every op in the input changeset has exactly one receipt entry.

## Atomic Write

Same pattern as `spex emit --out`: write to `${RECEIPTS_OUT}.tmp` then `mv` to the target. Either the file has the full v1 JSON or it doesn't exist (or it has the pre-run content if `mv` never happened).

## Testing

See `test_idempotency.md` and `test_br_integration.md` for end-to-end receipt shape assertions. See `test_substitution_table.md` for op-ordering correctness.
