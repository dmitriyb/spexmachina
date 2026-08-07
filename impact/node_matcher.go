package impact

import (
	"sort"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Pairing is one journal fold entry — a spec node's current task linkage, as
// mapping.MappingStore folds it from spec/.history.jsonl — enriched with the
// bead's live status. Folding the journal and joining BeadReader's live
// status onto each entry by identity hash happens upstream, in ImpactCommand;
// matching reads neither, carrying BeadStatus through untouched for
// ActionClassifier's cleanup gate further down. A pairing for which no bead
// was supplied arrives with BeadStatus unset.
type Pairing struct {
	SpecNodeID string // the fold entry's node key: a 12-character hex identity hash
	TaskID     string // the fold entry's current task/bead id
	NodeType   string // from the change event that gave this node its identity
	Module     string // the node's module name
	Name       string // the node's human-readable name
	BeadStatus string
}

// Match pairs a classified change with every pairing that stores its spec node's identity hash.
type Match struct {
	Change merkle.ClassifiedChange
	// Records is shaped as mapping.Record, the form ActionClassifier
	// (spexmachina-y0wc.24) still consumes until its own migration onto the
	// journal lands; recordsFromPairings bridges Pairing to it below.
	Records []mapping.Record
}

// Unmatched represents a changed spec node with no corresponding pairing.
type Unmatched struct {
	Change merkle.ClassifiedChange
}

// Orphaned represents a pairing whose referenced spec node was removed.
// NodeType is preserved from the originating removed change because identity
// hashes do not embed the node type and ActionClassifier needs it downstream.
type Orphaned struct {
	Record   mapping.Record
	NodeType string
}

// MatchNodes correlates classified changes with the journal fold's pairings
// using direct spec node ID comparison: the key merkle puts on a changed leaf
// and the node key a pairing stores are the same identity hash, so a match is
// a lookup and nothing else — no path parsing, no rekeying. Returns results
// sorted deterministically (NFR5).
func MatchNodes(changes []merkle.ClassifiedChange, pairings []Pairing) ([]Match, []Unmatched, []Orphaned) {
	// Index pairings by SpecNodeID.
	index := make(map[string][]Pairing)
	for _, p := range pairings {
		index[p.SpecNodeID] = append(index[p.SpecNodeID], p)
	}

	matched := map[string]bool{}
	orphanCandidates := map[string]Orphaned{}

	var matches []Match
	var unmatched []Unmatched

	for _, c := range changes {
		// Structural changes do not produce bead actions.
		if c.Impact == merkle.Structural {
			continue
		}

		// Direct ID lookup: change.Key == pairing.SpecNodeID.
		found := index[c.Key]

		if len(found) > 0 {
			records := recordsFromPairings(found)

			if c.Type == merkle.Removed {
				for i, p := range found {
					if !matched[p.TaskID] {
						orphanCandidates[p.TaskID] = Orphaned{Record: records[i], NodeType: c.NodeType}
					}
				}
			} else {
				sort.Slice(records, func(i, j int) bool {
					return records[i].BeadID < records[j].BeadID
				})
				matches = append(matches, Match{Change: c, Records: records})
				for _, p := range found {
					matched[p.TaskID] = true
					delete(orphanCandidates, p.TaskID)
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

// recordsFromPairings bridges Pairing to the mapping.Record shape
// ActionClassifier still consumes. Every field is a straight rename — no
// rekeying of the identity hash itself.
func recordsFromPairings(pairings []Pairing) []mapping.Record {
	out := make([]mapping.Record, len(pairings))
	for i, p := range pairings {
		out[i] = mapping.Record{
			SpecNodeID: p.SpecNodeID,
			BeadID:     p.TaskID,
			NodeType:   p.NodeType,
			Module:     p.Module,
			Component:  p.Name,
			BeadStatus: p.BeadStatus,
		}
	}
	return out
}
