package impact

import (
	"sort"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Match pairs a classified change with the mapping records that reference its spec node.
type Match struct {
	Change  merkle.ClassifiedChange
	Records []mapping.Record
}

// Unmatched represents a changed spec node with no corresponding mapping record.
type Unmatched struct {
	Change merkle.ClassifiedChange
}

// Orphaned represents a mapping record whose referenced spec node was removed.
// NodeType is preserved from the originating removed change because identity
// hashes do not embed the node type and ActionClassifier needs it downstream.
type Orphaned struct {
	Record   mapping.Record
	NodeType string
}

// MatchNodes correlates classified changes with mapping records using direct
// spec node ID comparison. Returns results sorted deterministically (NFR5).
func MatchNodes(changes []merkle.ClassifiedChange, records []mapping.Record) ([]Match, []Unmatched, []Orphaned) {
	// Index records by SpecNodeID.
	index := make(map[string][]mapping.Record)
	for _, r := range records {
		index[r.SpecNodeID] = append(index[r.SpecNodeID], r)
	}

	matched := map[int]bool{}
	orphanCandidates := map[int]Orphaned{}

	var matches []Match
	var unmatched []Unmatched

	for _, c := range changes {
		// Structural changes do not produce bead actions.
		if c.Impact == merkle.Structural {
			continue
		}

		// Direct ID lookup: change.Path == record.SpecNodeID.
		found := copyRecords(index[c.Path])

		if len(found) > 0 {
			sort.Slice(found, func(i, j int) bool {
				return found[i].BeadID < found[j].BeadID
			})

			if c.Type == merkle.Removed {
				for _, r := range found {
					if !matched[r.ID] {
						orphanCandidates[r.ID] = Orphaned{Record: r, NodeType: c.NodeType}
					}
				}
			} else {
				matches = append(matches, Match{Change: c, Records: found})
				for _, r := range found {
					matched[r.ID] = true
					delete(orphanCandidates, r.ID)
				}
			}
		} else if c.Type != merkle.Removed {
			unmatched = append(unmatched, Unmatched{Change: c})
		}
	}

	// Collect orphaned records sorted by bead ID.
	var orphaned []Orphaned
	for _, o := range orphanCandidates {
		orphaned = append(orphaned, o)
	}
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].Record.BeadID < orphaned[j].Record.BeadID
	})

	return matches, unmatched, orphaned
}

// copyRecords returns a shallow copy of the record slice to avoid aliasing index internals.
func copyRecords(records []mapping.Record) []mapping.Record {
	if len(records) == 0 {
		return nil
	}
	out := make([]mapping.Record, len(records))
	copy(out, records)
	return out
}

