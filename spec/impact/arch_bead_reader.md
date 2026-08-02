# BeadReader

[[5179e5a95653|Reading bead metadata]] happens here and nowhere else: a caller hands over the bytes of a tracker listing, and BeadReader hands back one entry per spec-managed bead. It starts no process and contacts no tracker. The input matches the shape of `br list --json`, and of any tracker whose listing conforms to it: an array of bead objects carrying `id`, `labels` and `status`.

## Responsibilities

- Decode the input JSON array into one entry per bead.
- For each bead, extract the `spex:<spec_node_id>` label and read the identity hash.
- Carry each bead's live `status` through untouched, so that [[3dcf3c279ac5|the cleanup gate for a removed spec node]] downstream can tell a closed bead from an open one.
- Skip beads without a `spex:` label — they are not spec-managed.
- Return an empty result, not an error, if the input is a valid JSON array with no spec-managed beads.
- Return the entries in the order the input array gave them. A caller wanting any other order sorts after the call.

## Interface

One call, taking either a stream to read from or the bytes already in hand. Each entry it returns carries four things: the tracker's own bead id (`spexmachina-abc` and the like), the spec node identity hash read out of the `spex:<spec_node_id>` label, the bead's live status exactly as the input reported it, and the bead's full label list, kept for downstream filters. A failure is returned to the caller rather than printed, and every message begins `impact: read beads:`.

## Input Shape

BeadReader expects JSON conforming to the widely-accepted tracker list shape. For br the exact shape is:

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

A bead's labels are read in order and the first `spex:<...>` whose suffix reads as a 12-hex identity hash (`^[a-f0-9]{12}$`) or a proposal slug (`^\d{4}-\d{2}-\d{2}-`, the dated-stem convention) wins; legacy integer suffixes, the `spex:cleanup-` prefix, and marker labels like `spex:obsolete` and bare `spex:cleanup` match neither grammar and are skipped as inert. If a bead carries more than one live-form label, the rest are ignored. Validator-level rules should prevent that from happening; BeadReader is defensive about it rather than reliant on them.

## Error Handling

- Malformed JSON → `"impact: read beads: parse: <json err>"`.
- A bead object missing `id` → `"impact: read beads: missing bead id at index N"`.
- A bead object with a `spex:` label whose suffix matches neither the hash nor the slug grammar → that label is passed over; if no other label on the bead parses, the bead is dropped from the result, silently and without an error, exactly as a bead carrying no `spex:` label is.

## No Subprocess

This is the structural fix for this component. The old implementation shelled out from inside the binary and ran the tracker's own `list --json` itself — that is retired. Callers now supply that listing as a file instead, which is the whole of [[116eb5f9906a|accepting bead state as a `--beads` input]], or hand over an equivalent shape from their own tracker.

BeadReader is the inbound half of the project-level rule that `spex` never invokes a bead CLI: tracker state enters the impact pipeline here, as a file the caller supplies, and leaves as changeset ops that an adapter outside the binary executes. It is not the binary's only tracker-state inlet — `spex ingest` reads `receipts.json`, whose `bead_id` and `was_existing` are tracker-assigned and load-bearing (see `spec/ingest/arch_reconciler.md`, "Sole writer, no tracker"). What both inlets share is the shape: tracker state always arrives as a file some other process produced, and no code path in between ever runs a tracker command. The practical consequence for this component is that it owns **no** freshness guarantee: the `status` field it carries is as current as the file it was handed, and the responsibility for that file being a live read belongs to the caller. Re-adding a subprocess here to "make sure the data is fresh" would reintroduce the tracker coupling the whole pipeline is shaped to avoid, and would make `spex impact` non-deterministic over its inputs.

## The label carries a node identity and nothing else

`spex:<spec_node_id>` is the whole of a bead's link back into the spec, and it is the node's own identity hash — the same key the merkle tree and the task journal use, so no lookup stands between the label and the node. BeadReader reads the hash and stops: whether that node still exists, and what kind it is, are discovered later, by NodeMatcher joining against the diff and the journal fold, not by reading anything more off the bead.

The consequence is that changes to what the spec graph is made of never reach the tracker as a relabelling. Moving content between files, retiring a kind of section, or re-describing a node leaves every `spex:<spec_node_id>` label valid, because identity is `hash(module, type, name)` and none of those operations move it. The one thing that does move it — a rename — is delete-plus-create by definition, so the old label closes with its task and the new node mints its own. Rewriting a live tracker is precisely what the pipeline has no way to do, since `spex` never invokes a bead CLI; the label scheme is built so it never needs to.

## Testing

- Unit-level: canned JSON fixtures exercise each extraction path.
- See `test_bead_matching.md` for integration tests that feed BeadReader output into NodeMatcher.
