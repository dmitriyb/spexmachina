package apply

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/dmitriyb/spexmachina/mapping"
)

// ApplyOpts configures the apply command execution.
type ApplyOpts struct {
	Creates     []Action
	Obsoletes   []Action
	ProposalRef string
	DryRun      bool
	SpecDir     string
	Logger      *slog.Logger
	Stdout      io.Writer
	Stderr      io.Writer
	Now         func() time.Time
}

// RunApply orchestrates the full apply pipeline:
//  1. Label obsoletes (mark intent, keep open)
//  2. Creates in hierarchy order with topological sort
//  3. Close obsoletes (replacements exist)
//  4. Tag all affected beads with proposal
//  5. Save snapshot
func RunApply(ctx context.Context, cli BeadCLI, store mapping.Store, opts ApplyOpts) error {
	if opts.DryRun {
		return printApplyDryRun(opts)
	}

	// 1. Label obsoletes — mark beads being replaced/removed with
	// spex:obsolete + commit:<HEAD> labels, keep them open.
	if err := LabelObsoletes(ctx, cli, store, opts.Obsoletes, opts.Logger); err != nil {
		opts.Logger.ErrorContext(ctx, "some beads failed to label", "error", err)
	}

	// 2. Sort creates by hierarchy (epics->features->tasks) with
	// topological sort within each type level.
	sorted, err := SortCreateActions(opts.Creates)
	if err != nil {
		return err
	}

	// 3. Create beads — abort on failure, no snapshot saved.
	createdIDs, err := CreateBeads(ctx, cli, store, sorted)
	if err != nil {
		return err
	}

	// 4. Close obsoletes — replacements now exist.
	// Errors are logged individually by CloseBeads via slog; no summary needed here.
	if err := CloseBeads(ctx, cli, opts.Obsoletes, opts.Logger); err != nil {
		opts.Logger.ErrorContext(ctx, "some beads failed to close", "error", err)
	}

	// 5. Tag all affected beads with proposal reference.
	allIDs := collectAffectedBeadIDs(createdIDs, opts.Obsoletes)
	if opts.ProposalRef != "" {
		if err := TagWithProposal(ctx, cli, allIDs, opts.ProposalRef, opts.Logger); err != nil {
			opts.Logger.ErrorContext(ctx, "some beads failed to tag", "error", err)
		}
	}

	// 6. Save snapshot.
	now := time.Now()
	if opts.Now != nil {
		now = opts.Now()
	}
	if err := SaveSnapshot(ctx, opts.SpecDir, now); err != nil {
		return err
	}

	fmt.Fprintf(opts.Stderr, "spex apply: done (created=%d obsoleted=%d)\n",
		len(createdIDs), len(opts.Obsoletes))
	return nil
}

// collectAffectedBeadIDs gathers unique bead IDs from creates and obsoletes.
func collectAffectedBeadIDs(createdIDs []string, obsoletes []Action) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, id := range createdIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, a := range obsoletes {
		if !seen[a.BeadID] {
			seen[a.BeadID] = true
			ids = append(ids, a.BeadID)
		}
	}
	return ids
}

// typePriority returns the sort priority for a spec node type.
// Lower values are created first: modules (epics) -> components (features) -> test_sections (tasks).
func typePriority(nodeType string) int {
	switch nodeType {
	case "module":
		return 0
	case "component":
		return 1
	case "test_section":
		return 2
	default:
		return 3
	}
}

// SortCreateActions sorts create actions by hierarchy (epics -> features -> tasks)
// with topological sort within each type level based on DepBeadIDs.
func SortCreateActions(actions []Action) ([]Action, error) {
	if len(actions) == 0 {
		return actions, nil
	}

	groups := make(map[int][]Action)
	for _, a := range actions {
		p := typePriority(a.NodeType)
		groups[p] = append(groups[p], a)
	}

	var result []Action
	for _, level := range []int{0, 1, 2, 3} {
		group, ok := groups[level]
		if !ok {
			continue
		}
		sorted, err := topoSortActions(group)
		if err != nil {
			return nil, err
		}
		result = append(result, sorted...)
	}

	return result, nil
}

// topoSortActions performs a stable topological sort within a type level.
// Actions with DepBeadIDs referencing another action's OldBeadID in the batch
// are placed after that action. When no edges exist, original order is preserved.
func topoSortActions(actions []Action) ([]Action, error) {
	if len(actions) <= 1 {
		return actions, nil
	}

	n := len(actions)

	// Map OldBeadID -> action index for matching dependencies within batch.
	oldBeadIdx := make(map[string]int)
	for i, a := range actions {
		if a.OldBeadID != "" {
			oldBeadIdx[a.OldBeadID] = i
		}
	}

	// Build adjacency list: adj[j] = indices that depend on j (j must come first).
	adj := make([][]int, n)
	inDegree := make([]int, n)

	for i, a := range actions {
		for _, depID := range a.DepBeadIDs {
			if j, ok := oldBeadIdx[depID]; ok && j != i {
				adj[j] = append(adj[j], i)
				inDegree[i]++
			}
		}
	}

	// Kahn's algorithm with min-index priority for stable ordering.
	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	sorted := make([]Action, 0, n)
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		sorted = append(sorted, actions[idx])

		for _, next := range adj[idx] {
			inDegree[next]--
			if inDegree[next] == 0 {
				// Insert in sorted position to maintain stable ordering.
				pos := sort.SearchInts(queue, next)
				queue = append(queue, 0)
				copy(queue[pos+1:], queue[pos:])
				queue[pos] = next
			}
		}
	}

	if len(sorted) != n {
		return nil, fmt.Errorf("apply: circular dependency among %d actions", n-len(sorted))
	}

	return sorted, nil
}

// printApplyDryRun writes a human-readable listing of planned actions.
func printApplyDryRun(opts ApplyOpts) error {
	w := opts.Stdout
	if w == nil {
		return nil
	}

	for _, a := range opts.Obsoletes {
		fmt.Fprintf(w, "label %s (spex:obsolete, commit:<HEAD>)\n", a.BeadID)
	}

	sorted, err := SortCreateActions(opts.Creates)
	if err != nil {
		return err
	}
	for _, a := range sorted {
		bt := beadType(a.NodeType)
		if bt == "" {
			bt = a.NodeType
		}
		fmt.Fprintf(w, "create %s/%s --type %s\n", a.Module, a.Node, bt)
	}

	for _, a := range opts.Obsoletes {
		fmt.Fprintf(w, "close %s\n", a.BeadID)
	}

	total := len(opts.Creates) + len(opts.Obsoletes)
	if opts.ProposalRef != "" && total > 0 {
		fmt.Fprintf(w, "tag %d beads with proposal %s\n", total, opts.ProposalRef)
	}

	fmt.Fprintln(w, "save snapshot")

	return nil
}
