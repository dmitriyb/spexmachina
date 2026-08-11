# BeadReader

[[5179e5a95653|Reading bead metadata]] happens here and nowhere else: a caller hands over the bytes of a tracker listing, and BeadReader hands back one entry per bead. It starts no process and contacts no tracker — and it parses no labels: the label is an adapter-facing idempotency key spex reads nothing from. The input matches the shape of `br list --json`, and of any tracker whose listing conforms to it: an array of bead objects carrying `id` and `status`.

## Responsibilities

- Decode the input JSON array into one entry per bead.
- Carry each bead's `id` and live `status` through untouched, so that [[3dcf3c279ac5|the cleanup gate for a removed spec node]] downstream can tell a closed bead from an open one.
- Return an empty result, not an error, if the input is a valid JSON array with no beads.
- Return the entries in the order the input array gave them. A caller wanting any other order sorts after the call.

Which beads are spec-managed is not this component's question: pairings come from the journal fold carrying task ids, live status joins onto them by task id, and a bead the fold never names simply never joins.

## Interface

One call, taking either a stream to read from or the bytes already in hand. Each entry it returns carries two things: the tracker's own bead id (`spexmachina-abc` and the like) and the bead's live status exactly as the input reported it. A failure is returned to the caller rather than printed, and every message begins `impact: read beads:`.

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

## No Label Parsing

Earlier versions read the spec node identity hash out of the bead's `spex:<spec_node_id>` label, with a grammar distinguishing live forms from legacy integers, `cleanup-` prefixes and marker labels — a parser whose whole job was reversing a key the journal already holds forward. That parser is retired. Labels are now `spex:<eid>` idempotency keys, meaningful to the adapter's exact-match probe and to nothing inside spex; whatever shape a bead's labels take — current eids, legacy hashes, integers, markers — BeadReader carries none of them and branches on none of them.

## Error Handling

- Malformed JSON → `"impact: read beads: parse: <json err>"`.
- A bead object missing `id` → `"impact: read beads: missing bead id at index N"`.

## No Subprocess

This is the structural fix for this component. The old implementation shelled out from inside the binary and ran the tracker's own `list --json` itself — that is retired. Callers now supply that listing as a file instead, which is the whole of [[116eb5f9906a|accepting bead state as a `--beads` input]], or hand over an equivalent shape from their own tracker.

BeadReader is the inbound half of the project-level rule that `spex` never invokes a bead CLI: tracker state enters the impact pipeline here, as a file the caller supplies, and leaves as changeset ops that an adapter outside the binary executes. It is not the binary's only tracker-state inlet — `spex ingest` reads `receipts.json`, whose `bead_id` and `was_existing` are tracker-assigned and load-bearing (see `spec/ingest/arch_reconciler.md`, "One write path, no tracker"). What both inlets share is the shape: tracker state always arrives as a file some other process produced, and no code path in between ever runs a tracker command. The practical consequence for this component is that it owns **no** freshness guarantee: the `status` field it carries is as current as the file it was handed, and the responsibility for that file being a live read belongs to the caller. Re-adding a subprocess here to "make sure the data is fresh" would reintroduce the tracker coupling the whole pipeline is shaped to avoid, and would make `spex impact` non-deterministic over its inputs.

## The journal carries the linkage; the bead carries a status

A bead's link back into the spec lives in the task journal, not on the bead: the fold pairs each node's identity hash with the task id the tracker minted for it, and BeadReader's entries join onto those pairings by task id. The bead contributes exactly one fact the journal cannot know — its live status — and BeadReader reads that fact and stops: whether the paired node still exists, and what kind it is, are discovered later, by NodeMatcher joining the fold against the diff, not by reading anything more off the bead.

The consequence is that nothing about the spec graph, and nothing about the label scheme, ever needs a tracker rewrite to stay joinable. Moving content between files, retiring a kind of section, re-describing a node, or changing what emit stamps into `idempotency.label` leaves every existing pairing valid, because the join key is the task id the receipt recorded at creation time. Rewriting a live tracker is precisely what the pipeline has no way to do, since `spex` never invokes a bead CLI; keying the join on the journal is what makes it never need to.

## Testing

- Unit-level: canned JSON fixtures exercise each extraction path.
- See `test_bead_matching.md` for integration tests that feed BeadReader output into NodeMatcher.
