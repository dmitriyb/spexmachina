# Changeset consumption

How the adapter reads and validates changeset.json v1 before processing ops.

## Version Check

```bash
version=$(jq -r '.version' "$CHANGESET")
if [[ "$version" != "1" ]]; then
    echo "error: unsupported changeset version: $version (expected 1)" >&2
    exit 1
fi
```

## Required Top-Level Fields

- `version` (int, must be 1)
- `git_head` (string, hex SHA) — threaded into commit:<HEAD> labels on close ops
- `proposal` (string) — threaded into proposal labels
- `ops` (array) — the ordered op list

Missing any of these → exit 1 with a clear error naming the missing field.

## Per-Op Parsing

For each op, pre-parse the required shape:

```bash
op_id=$(jq -r '.op_id // empty' <<< "$op")
op_type=$(jq -r '.type // empty' <<< "$op")
if [[ -z "$op_id" || -z "$op_type" ]]; then
    fail_receipt "$op_id" "malformed op: missing op_id or type"
    continue
fi
```

Schema-dependent fields are extracted inside each op type's handler:

- `create`: `spec_node_id`, `spec_node_kind`, `idempotency.label`, `parent` (opt), `deps` (opt), `priority`, `title`, `body`.
- `close`: `target`, `labels`, `reason`.
- `label`: `target`, `labels`.
- `tag`: `target`, `labels`.

## Ref Parsing

A ref is a JSON object with a `ref` discriminator. The adapter has one helper:

```bash
resolve_ref() {
    local ref_json="$1"
    local kind=$(jq -r '.ref' <<< "$ref_json")
    case "$kind" in
        bead)
            jq -r '.bead_id' <<< "$ref_json"
            ;;
        op)
            local op_id=$(jq -r '.op_id' <<< "$ref_json")
            if [[ -z "${SUB_TABLE[$op_id]:-}" ]]; then
                echo "__UNRESOLVED__$op_id"
            else
                echo "${SUB_TABLE[$op_id]}"
            fi
            ;;
        spec_node)
            local snid=$(jq -r '.spec_node_id' <<< "$ref_json")
            # Look up in .bead-map.json for latest bead for this spec_node_id.
            jq -r --arg snid "$snid" '.records[] | select(.spec_node_id == $snid) | .bead_id' "$SPEX_MAPPING_FILE" | head -1
            ;;
        *)
            echo "__UNKNOWN_REF__$kind"
            ;;
    esac
}
```

An unresolved `op` ref (`__UNRESOLVED__...`) causes the consuming op to record an error receipt with reason `"dependency op-XXXX not yet resolved"` — this should never happen for a well-formed changeset since emit guarantees topological order.

An unresolved `spec_node` ref (jq returns empty) causes the consuming op to record error reason `"spec_node_id X has no mapping record"`. This is possible if the mapping store is stale; the user should re-run emit.

## Encoding Expectations

- JSON arrays preserve order.
- Unicode is UTF-8.
- Numeric values (priority) are integers.

## Error Handling

Any parse-level error (malformed JSON, missing required field) exits the adapter with status 1 BEFORE writing any receipts — the failure is at the changeset level, not at an op level. Receipts are only written for attempted ops.
