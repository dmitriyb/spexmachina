# Node Matching Algorithm

## Approach

Build an index of mapping records by their spec node ID, then look up each changed spec node directly. Skip structural changes before entering the matching loop.

## Algorithm

1. Filter out changes with `impact == Structural` — these produce no matches
2. Index mapping records by `spec_node_id` (e.g., `"module/3/component/2"`)
3. Index mapping records by module name (for orphan detection on removed changes)
4. For each non-structural changed spec node:
   - Use the change's key (spec ID) to look up matching mapping records directly
   - No path parsing, no name resolution, no case conversion needed
5. Collect results into matched, unmatched, and orphaned lists

## Direct ID Matching

The change key from the ID-based merkle diff (e.g., `"module/3/component/2"`) directly matches the `spec_node_id` field in mapping records. This is a simple map lookup:

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

## Multiple Beads per Node

A single spec node may have multiple beads (e.g., an implementation bead and a review bead). All matching records are returned.
