# BeadReader

Parses bead metadata from input JSON. Pure function: takes `[]byte` or `io.Reader`, returns `[]BeadSpec`. The input JSON matches the shape of `br list --json` (and compatible tracker outputs): an array of bead objects with `id`, `labels`, and `status` fields.

## Responsibilities

- Decode the input JSON array into a typed intermediate.
- For each bead, extract the `spex:<record-id>` label and parse the record-id integer.
- For each bead, carry forward the live `status` field so ActionClassifier can gate cleanup beads correctly.
- Skip beads without a `spex:` label — they are not spec-managed.
- Return an empty slice (not an error) if the input is a valid JSON array with no spec-managed beads.

## Interface

```go
type BeadSpec struct {
    ID       string // tracker bead ID, e.g., "spexmachina-abc"
    RecordID int    // integer parsed from the "spex:<n>" label
    Status   string // live status: "open" | "in_progress" | "closed"
    Labels   []string // all labels (retained for future use, e.g., spec_proposal filter)
}

// ReadBeads parses the JSON payload into typed BeadSpec entries.
// No subprocess invocation. Errors wrap with "impact: read beads: ...".
func ReadBeads(r io.Reader) ([]BeadSpec, error)

// ReadBeadsBytes is a convenience for []byte inputs.
func ReadBeadsBytes(data []byte) ([]BeadSpec, error)
```

## Input Shape

The function expects JSON that conforms to the widely-accepted tracker list shape. For br the exact shape is:

```json
{
  "issues": [
    {
      "id": "spexmachina-abc",
      "status": "open",
      "labels": ["spex:42", "commit:deadbeef"],
      ...
    }
  ]
}
```

BeadReader handles both the wrapped form (`{"issues": [...]}`) and a bare array (`[...]`) for adapter-produced JSON that may have unwrapped it.

## Label Parsing

```go
for _, lbl := range bead.Labels {
    if strings.HasPrefix(lbl, "spex:") {
        n, err := strconv.Atoi(strings.TrimPrefix(lbl, "spex:"))
        if err == nil {
            spec.RecordID = n
            break
        }
    }
}
```

If multiple `spex:<n>` labels exist on a single bead, the first one wins. Validator-level rules should prevent this; BeadReader is defensive.

## Error Handling

- Malformed JSON → `"impact: read beads: parse: <json err>"`.
- A bead object missing `id` → `"impact: read beads: missing bead id at index N"`.
- A bead object with a `spex:` label where the integer fails to parse → logged as a warning, bead skipped (still returned in the non-spec category? No — dropped since it can't be matched by RecordID).

## No Subprocess

This is the structural fix for this component. The old implementation ran `exec.CommandContext(ctx, bin, "list", "--json")` — that's retired. Callers now pass `br list --json` output as a file (via spex impact's `--beads` flag) or any equivalent shape from their tracker.

BeadReader is the inbound half of the project-level rule that `spex` never invokes a bead CLI: tracker state enters the impact pipeline here, as a file the caller supplies, and leaves as changeset ops that an adapter outside the binary executes. It is not the binary's only tracker-state inlet — `spex ingest` reads `receipts.json`, whose `bead_id` and `was_existing` are tracker-assigned and load-bearing (see `spec/ingest/arch_reconciler.md`, "Sole writer, no tracker"). What both inlets share is the shape: tracker state always arrives as a file some other process produced, and no code path in between ever runs a tracker command. The practical consequence for this component is that it owns **no** freshness guarantee: the `status` field it carries is as current as the file it was handed, and the responsibility for that file being a live read belongs to the caller. Re-adding a subprocess here to "make sure the data is fresh" would reintroduce the tracker coupling the whole pipeline is shaped to avoid, and would make `spex impact` non-deterministic over its inputs.

## The label carries a record id and nothing else

`spex:<record-id>` is the whole of a bead's link back into the spec, and it resolves to a mapping record — never to a file, a node kind, or an identity hash. That is why BeadReader parses an integer and stops: the spec-side vocabulary of node types is invisible here, and the node a bead stands for is discovered later, by NodeMatcher looking the record up, not by reading anything off the bead.

The consequence is that changes to what the spec graph is made of never reach the tracker as a relabelling. Retiring a kind of spec node, or moving content between files, leaves every `spex:<n>` label valid, because the label was never finer-grained than a record. A label that had encoded a node kind or a path would have to be rewritten on every bead each time the spec's structure changed — and rewriting a live tracker is precisely what the pipeline has no way to do, since `spex` never invokes a bead CLI.

## Testing

- Unit-level: canned JSON fixtures exercise each extraction path.
- See `test_bead_matching.md` for integration tests that feed BeadReader output into NodeMatcher.
