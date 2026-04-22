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
//  2. Create proposal epic, then creates in hierarchy order with topological sort
//  3. Close obsoletes (replacements exist)
//  4. Save snapshot
//
// Bead grouping by proposal wave is structural: every created bead is a child
// of that run's proposal epic. No per-bead proposal tagging is applied.
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

	// 3. Create proposal epic (if any creates), then create all beads under
	// that epic as --parent. Abort on failure, no snapshot saved.
	createdIDs, err := CreateBeads(ctx, cli, store, opts.ProposalRef, sorted)
	if err != nil {
		return err
	}

	// 4. Close obsoletes — replacements now exist.
	if err := CloseBeads(ctx, cli, opts.Obsoletes, opts.Logger); err != nil {
		opts.Logger.ErrorContext(ctx, "some beads failed to close", "error", err)
	}

	// 5. Save snapshot.
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

// typePriority returns the sort priority for a spec node type.
// Lower values are created first: features (components) and data_flow tasks
// share a level so topological sort can interleave them, then test_section
// tasks. The proposal epic is created separately outside this ordering.
func typePriority(nodeType string) int {
	switch nodeType {
	case "component", "data_flow":
		return 0
	case "test_section":
		return 1
	default:
		return 2
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
	for _, level := range []int{0, 1, 2} {
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
	if len(sorted) > 0 && opts.ProposalRef != "" {
		fmt.Fprintf(w, "create proposal-epic %q --type epic\n", opts.ProposalRef)
	}
	for _, a := range sorted {
		if isCleanup(a) {
			fmt.Fprintf(w, "create cleanup/%s --type task\n", a.Node)
			continue
		}
		bt := beadType(a.NodeType)
		if bt == "" {
			bt = a.NodeType
		}
		fmt.Fprintf(w, "create %s/%s --type %s\n", a.Module, a.Node, bt)
	}

	for _, a := range opts.Obsoletes {
		fmt.Fprintf(w, "close %s\n", a.BeadID)
	}

	fmt.Fprintln(w, "save snapshot")

	return nil
}
