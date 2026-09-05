package plan

import (
	"sort"

	"github.com/dmitriyb/spexmachina/merkle"
)

// Pairing is one journal fold entry — a spec node's current task linkage, as
// mapping.MappingStore folds it from spec/.history.jsonl — enriched with the
// task's live status. Folding the journal and joining the --tasks artifact's
// live status onto each entry by task id happens upstream of NodeMatcher, in
// PlanCommand; matching reads neither TaskID's status nor alters it, only
// carrying BeadStatus through untouched for ActionClassifier's cleanup gate
// and retarget split further down (arch_node_matcher.md). A pairing whose
// task the artifact does not list arrives with BeadStatus unset.
type Pairing struct {
	SpecNodeID string // the fold entry's node key: a 12-character hex identity hash
	TaskID     string // the fold entry's current task/bead id
	NodeType   string // from the change event that gave this node its identity
	Module     string // the node's module name
	Name       string // the node's human-readable name
	BeadStatus string
	// After is the sourcing event's recorded after-hash — the spec node's
	// content hash the journal paired with this pairing's task.
	// ActionClassifier compares it against a matched change's current hash
	// to detect the already-tracked case: the change resurfaced only
	// because a partial ingest run left the snapshot unsaved, not because
	// new work exists.
	After string
}

// Match pairs a classified change with every pairing that stores its spec
// node's identity hash — not just the first (arch_node_matcher.md,
// "Responsibilities"). The fold answers with one current pairing per node,
// so in practice Records holds a single entry; matching neither relies on
// nor enforces that.
type Match struct {
	Change  merkle.ClassifiedChange
	Records []Pairing
}

// Unmatched is an added or modified spec node no pairing refers to — nothing
// tracks it yet. A removed node no pairing refers to reaches none of
// NodeMatcher's three lists: there is no bead to obsolete and nothing left
// to build.
type Unmatched struct {
	Change merkle.ClassifiedChange
}

// Orphaned is a pairing whose spec node the diff reports as removed.
// NodeType is carried alongside it because an identity hash does not embed
// one and ActionClassifier needs it downstream.
type Orphaned struct {
	Record   Pairing
	NodeType string
}

// MatchNodes correlates classified changes with the journal fold's pairings
// using direct spec node ID comparison: the key merkle puts on a changed
// leaf and the node key a pairing stores are the same identity hash, so a
// match is a lookup and nothing else — no path parsing, no module name
// resolution, no rekeying (arch_node_matcher.md, "Matching Logic"). Changes
// with Impact == merkle.Structural are filtered out before any lookup runs
// and produce no matches, no unmatched entries, and no orphans. Returns
// matched, unmatched and orphaned pairings, each ordered by bead id so that
// the same diff over the same bead state returns the same three lists in
// the same order every run (5369b5ae363c). Never fails.
func MatchNodes(changes []merkle.ClassifiedChange, pairings []Pairing) ([]Match, []Unmatched, []Orphaned) {
	index := make(map[string][]Pairing)
	for _, p := range pairings {
		index[p.SpecNodeID] = append(index[p.SpecNodeID], p)
	}

	matchedTasks := map[string]bool{}
	orphanCandidates := map[string]Orphaned{}

	var matches []Match
	var unmatched []Unmatched

	for _, c := range changes {
		if c.Impact == merkle.Structural {
			continue
		}

		found := index[c.Key]

		if len(found) > 0 {
			if c.Type == merkle.Removed {
				for _, p := range found {
					if !matchedTasks[p.TaskID] {
						orphanCandidates[p.TaskID] = Orphaned{Record: p, NodeType: c.NodeType}
					}
				}
			} else {
				records := append([]Pairing(nil), found...)
				sort.Slice(records, func(i, j int) bool {
					return records[i].TaskID < records[j].TaskID
				})
				matches = append(matches, Match{Change: c, Records: records})
				for _, p := range found {
					matchedTasks[p.TaskID] = true
					delete(orphanCandidates, p.TaskID)
				}
			}
		} else if c.Type != merkle.Removed {
			unmatched = append(unmatched, Unmatched{Change: c})
		}
	}

	var orphaned []Orphaned
	for _, o := range orphanCandidates {
		orphaned = append(orphaned, o)
	}
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].Record.TaskID < orphaned[j].Record.TaskID
	})

	return matches, unmatched, orphaned
}
