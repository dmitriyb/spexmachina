# Substitution table

The adapter maintains a bash associative array keyed by op_id, mapping to the bead_id produced by that op's execution.

## Declaration

```bash
declare -A SUB_TABLE
```

Requires Bash 4.0+ (associative arrays).

## Population

After each ok create op (including was_existing=true matches):

```bash
# inside process_create
SUB_TABLE["$op_id"]="$bead_id"
```

For errored/skipped create ops: NO entry is added. Subsequent ops that ref this op_id fail at resolve time.

## Lookup

```bash
lookup_sub() {
    local op_id="$1"
    if [[ -z "${SUB_TABLE[$op_id]:-}" ]]; then
        return 1
    fi
    echo "${SUB_TABLE[$op_id]}"
}
```

## Thread Safety

Not thread-safe — the adapter processes ops sequentially in a single bash process. Concurrent adapter invocations are not supported; the integration test documents this limitation.

## Persistence

The table is process-local. A crash loses it. On re-run, the adapter rebuilds it from the re-executed create ops (idempotent re-matches via `was_existing=true` still populate the table, so forward refs still resolve).

## Interaction with Ref Kinds

| Ref | Uses SUB_TABLE? | Fallback |
|-----|-----------------|----------|
| `ref:bead` | no | literal `bead_id` is used |
| `ref:op` | yes | error if entry missing |
| `ref:spec_node` | no | live lookup into `.bead-map.json` |

## Debugging

When `SPEX_ADAPTER_DEBUG=1` is set, the adapter prints the substitution table to stderr after each op:

```bash
if [[ "${SPEX_ADAPTER_DEBUG:-0}" == "1" ]]; then
    echo "DEBUG: SUB_TABLE after $op_id:" >&2
    for k in "${!SUB_TABLE[@]}"; do
        echo "  $k → ${SUB_TABLE[$k]}" >&2
    done
fi
```

## Correctness Property

After the adapter processes the changeset, the following invariant holds:

- For every op_id `X` such that an op referenced `{"ref":"op","op_id":"X"}` somewhere AND op X's receipt was `ok`, `SUB_TABLE[X]` is set.
- For every op_id `X` that was skipped-with-was_existing-true, `SUB_TABLE[X]` is also set (to the existing bead_id).
- No entries exist for errored ops.

This ensures every same-run forward-ref can either resolve or fail explicitly.
