# Node Matching Algorithm

## Approach

Build an index of mapping records by identity hash, then look up each changed spec node directly. Skip structural changes before entering the matching loop.

## Algorithm

1. Filter out changes with `impact == Structural` — these produce no matches
2. Index mapping records by `spec_node_id` (a 12-char hex identity hash)
3. Index mapping records by module identity hash (for orphan detection on removed changes)
4. For each non-structural changed spec node:
   - Use the change's key (the same identity hash) to look up matching mapping records directly
   - No path parsing, no name resolution, no case conversion, no rekeying
5. Collect results into matched, unmatched, and orphaned lists

## Direct Identity-Hash Matching

The change key from the merkle diff is the identity hash of the spec node — the same string stored in `spec_node_id` on every mapping record. Matching is a single map lookup:

```go
func MatchNodes(changes []ClassifiedChange, records []Record) (matched, unmatched, orphaned) {
    index := make(map[string][]Record)
    for _, r := range records {
        index[r.SpecNodeID] = append(index[r.SpecNodeID], r)
    }

    for _, change := range changes {
        // Structural changes do not produce bead actions
        if change.Impact == Structural {
            continue
        }

        if recs, ok := index[change.Key]; ok {
            matched = append(matched, Match{Change: change, Records: recs})
        } else if change.Type != Removed {
            unmatched = append(unmatched, Unmatched{Change: change})
        }
    }

    // Orphan detection for removed changes...
}
```

A worked example: a component's `id` field is `abc123def456`. The merkle tree builds a leaf with key `"abc123def456"`. The bead-map record for that component has `spec_node_id: "abc123def456"`. When the component's content file changes, the diff emits `change.Key = "abc123def456"`, and `index["abc123def456"]` returns the record directly. There is no intermediate format.

## Multiple Beads per Node

A single spec node may have multiple beads (e.g., an implementation bead and a follow-up bead). All matching records are returned.

## What's no longer here

Earlier versions of this file documented two key formats — `module/N/component/M` (merkle) and `<module-name>/component/<id>` (bead-map) — and described a `buildMerkleIndex` helper that translated one to the other. Both formats are gone. The merkle tree key is the identity hash, the bead-map `spec_node_id` is the identity hash, and `buildMerkleIndex` is deleted.
