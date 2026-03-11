# Node Matching Algorithm

## Approach

Build an index of mapping records by their spec node ID, then look up each changed spec node directly.

## Algorithm

1. Index mapping records by `spec_node_id` (e.g., `"module/3/component/2"`)
2. For each changed spec node:
   - Use the change's key (spec ID) to look up matching mapping records directly
   - No path parsing, no name resolution, no case conversion needed
3. Collect results into matched, unmatched, and orphaned lists

## Direct ID Matching

The change key from the ID-based merkle diff (e.g., `"module/3/component/2"`) directly matches the `spec_node_id` field in mapping records. This is a simple map lookup:

```go
func MatchNodes(changes []ClassifiedChange, records []Record) (matched, unmatched, orphaned) {
    index := make(map[string][]Record)
    for _, r := range records {
        index[r.SpecNodeID] = append(index[r.SpecNodeID], r)
    }

    for _, change := range changes {
        if recs, ok := index[change.Key]; ok {
            matched = append(matched, Match{Change: change, Records: recs})
            delete(index, change.Key)
        } else {
            unmatched = append(unmatched, Unmatched{Change: change})
        }
    }

    // Remaining records in index are orphaned
    for _, recs := range index {
        for _, r := range recs {
            orphaned = append(orphaned, Orphaned{Record: r})
        }
    }
}
```

## Multiple Beads per Node

A single spec node may have multiple beads (e.g., an implementation bead and a review bead). All matching records are returned.
