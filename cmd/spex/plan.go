package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

// hexNodeIDRe validates a --absorb entry's node field: the 12-character
// lowercase-hex identity hash shape every spec node key takes
// (spec/plan/arch_plan_command.md, pre-flight step 6).
var hexNodeIDRe = regexp.MustCompile(`^[a-f0-9]{12}$`)

func newPlanCmd() *cobra.Command {
	var proposal, gitHead, diffPath, tasksPath, absorbPath, outPath string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute the bead-action changeset from a merkle diff",
		Long: `spex plan reads a merkle diff, refuses it if spex diff flagged it as
incomplete, enriches the journal fold's pairings with live task status from
a caller-supplied --tasks file, maps changed spec nodes to actions, and
composes changeset.json — one invocation, no intermediate document.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlanE(cmd, proposal, gitHead, diffPath, tasksPath, absorbPath, outPath)
		},
	}

	cmd.Flags().StringVar(&proposal, "proposal", "", "proposal ref (filename stem)")
	cmd.Flags().StringVar(&gitHead, "git-head", "", "git HEAD SHA (7-40 hex chars)")
	cmd.Flags().StringVar(&diffPath, "diff", "", "path to the diff document spex diff --json writes (default: stdin; '-' selects stdin explicitly)")
	cmd.Flags().StringVar(&tasksPath, "tasks", "", "task-state artifact: the version-1 document listing in-flight tasks (schema/task-state.schema.json)")
	cmd.Flags().StringVar(&absorbPath, "absorb", "", "git-committed JSON list of {node, reason} cosmetic-modification marks")
	cmd.Flags().StringVar(&outPath, "out", "", "changeset output path (default: stdout)")
	_ = cmd.MarkFlagRequired("proposal")
	_ = cmd.MarkFlagRequired("git-head")
	_ = cmd.MarkFlagRequired("tasks")
	return cmd
}

func runPlanE(cmd *cobra.Command, proposal, gitHead, diffPath, tasksPath, absorbPath, outPath string) error {
	if proposal == "" {
		return planInputErr(fmt.Errorf("plan: --proposal is required"))
	}
	if !gitHeadRe.MatchString(gitHead) {
		return planInputErr(fmt.Errorf("plan: --git-head must be a hex SHA (7-40 chars), got %q", gitHead))
	}
	if tasksPath == "" {
		return planInputErr(fmt.Errorf("plan: --tasks is required"))
	}

	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return planInputErr(err)
	}

	changes, diffErrors, err := readPlanDiff(cmd, diffPath)
	if err != nil {
		return planInputErr(err)
	}
	if len(diffErrors) > 0 {
		for _, de := range diffErrors {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: [%s] %s\n", de.Type, de.Message)
		}
		return planInputErr(fmt.Errorf("plan: diff contains %d error(s), refusing to proceed", len(diffErrors)))
	}

	ctx, err := lifecycle.Resolve(resolveProjectRoot(specDir))
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	store := mapping.NewMappingStore(ctx.JournalPath)
	events, err := store.Parse()
	if err != nil {
		return planInputErr(fmt.Errorf("plan: read journal: map: %w", err))
	}
	fold, err := store.List()
	if err != nil {
		return planInputErr(fmt.Errorf("plan: read journal: map: %w", err))
	}
	registration := resolvePlanRegistration(events, proposal)

	graph, err := loadPlanSpecGraph(specDir)
	if err != nil {
		return planInputErr(fmt.Errorf("plan: load spec graph: %w", err))
	}

	// TaskReader validates and parses the required --tasks artifact; each
	// listed task's live status joins onto the fold's pairing whose task id
	// matches (spec/plan/arch_plan_command.md, pre-flight step 5).
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return planInputErr(fmt.Errorf("plan: read tasks: %w", err))
	}
	tasks, err := plan.ReadTasksBytes(data)
	if err != nil {
		return planInputErr(err)
	}
	taskStatus := make(map[string]string, len(tasks))
	for _, tsk := range tasks {
		taskStatus[tsk.ID] = tsk.Status
	}

	absorbed := []plan.AbsorbedEntry{}
	if absorbPath != "" {
		data, err := os.ReadFile(absorbPath)
		if err != nil {
			return planInputErr(fmt.Errorf("plan: read absorb: %w", err))
		}
		var entries []absorbInput
		if err := json.Unmarshal(data, &entries); err != nil {
			return planInputErr(fmt.Errorf("plan: read absorb: parse %s: %w", absorbPath, err))
		}
		absorbed, changes, err = applyAbsorb(entries, changes)
		if err != nil {
			return planRefusalErr(err)
		}
	}

	// An empty plan-relevant list is not a refusal: it warns and the run
	// proceeds, exit 0 (spec/plan/arch_plan_command.md, "Exit Codes": "One
	// condition warns and does not fail"). What survives in the changeset
	// regardless — the proposal epic, any cleanup create — is left unstated
	// here: arch_plan_command.md and flow_plan.md disagree on that point
	// (drifts/drift-spexmachina-swvx.21-empty-plan-relevant-scope.json).
	if len(graph.PlanRelevant()) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: plan: no node type in the resolved profile's plan-relevant list produces tasks")
	}

	pairings := planPairingsForMatching(fold, taskStatus)
	matches, unmatched, orphaned := plan.MatchNodes(changes, pairings)

	actions, err := plan.ClassifyActions(matches, unmatched, orphaned, graph)
	if err != nil {
		return planRefusalErr(err)
	}

	builder := &plan.Builder{
		SpecGraph:    graph,
		Fold:         newPlanFold(fold, taskStatus),
		Registration: registration,
		GitHead:      gitHead,
		Proposal:     proposal,
		Absorbed:     absorbed,
	}
	cs, err := builder.Build(actions)
	if err != nil {
		return planRefusalErr(err)
	}

	return writePlanChangeset(cs, outPath, cmd.OutOrStdout())
}

// readPlanDiff reads the diff document from --diff (a path, or "-" for
// explicit stdin) or, when the flag is omitted, from stdin — the pipeline
// composability arch_plan_command.md's "Composability" section describes.
func readPlanDiff(cmd *cobra.Command, diffPath string) ([]merkle.ClassifiedChange, []merkle.DiffError, error) {
	var data []byte
	var err error
	if diffPath != "" && diffPath != "-" {
		data, err = os.ReadFile(diffPath)
		if err != nil {
			return nil, nil, fmt.Errorf("plan: read diff: %w", err)
		}
	} else {
		data, err = io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, nil, fmt.Errorf("plan: read stdin: %w", err)
		}
	}
	changes, diffErrors, err := parseDiffJSON(data)
	if err != nil {
		return nil, nil, fmt.Errorf("plan: %w", err)
	}
	return changes, diffErrors, nil
}

// absorbInput mirrors one --absorb file entry's on-disk shape: a marked
// node's identity hash and the authored reason it is cosmetic.
type absorbInput struct {
	Node   string `json:"node"`
	Reason string `json:"reason"`
}

// applyAbsorb validates every --absorb entry against the diff's changes and
// withholds each valid mark's change from the stream handed to matching
// (spec/plan/arch_plan_command.md, pre-flight step 6). A mark whose node is
// not a 12-hex identity hash, or that the diff does not report as modified,
// is a plan error naming the node. Absorbed entries are returned in input
// order; the remaining changes preserve the diff's original order.
func applyAbsorb(entries []absorbInput, changes []merkle.ClassifiedChange) ([]plan.AbsorbedEntry, []merkle.ClassifiedChange, error) {
	byKey := make(map[string]merkle.ClassifiedChange, len(changes))
	for _, c := range changes {
		byKey[c.Key] = c
	}

	withheld := make(map[string]bool, len(entries))
	absorbed := make([]plan.AbsorbedEntry, 0, len(entries))
	for _, e := range entries {
		if !hexNodeIDRe.MatchString(e.Node) {
			return nil, nil, fmt.Errorf("plan: absorb: entry %q is not a 12-character hex node identity hash", e.Node)
		}
		c, ok := byKey[e.Node]
		if !ok || c.Type != merkle.Modified {
			return nil, nil, fmt.Errorf("plan: absorb: node %s is not reported as modified in the diff", e.Node)
		}
		absorbed = append(absorbed, plan.AbsorbedEntry{Node: e.Node, Before: c.OldHash, After: c.NewHash, Reason: e.Reason})
		withheld[e.Node] = true
	}

	remaining := make([]merkle.ClassifiedChange, 0, len(changes))
	for _, c := range changes {
		if !withheld[c.Key] {
			remaining = append(remaining, c)
		}
	}
	return absorbed, remaining, nil
}

// loadPlanSpecGraph reads project.json and every declared module's
// module.json into a plan.SpecGraph, keyed by module identity hash exactly
// as plan.NewSpecGraph expects (spec/plan/arch_plan_command.md, pre-flight
// step 4). A module.json that does not exist on disk is skipped rather than
// treated as fatal, inherited from the spec graph loader this command replaced.
// It also resolves the project's profile (profile.json beside project.json,
// or the built-in default) and attaches it, so ActionClassifier's node-type
// gate reads this project's own plan-relevant declaration
// (spec/plan/arch_action_classifier.md, "Node-Type Gate").
func loadPlanSpecGraph(specDir string) (plan.SpecGraph, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return plan.SpecGraph{}, fmt.Errorf("read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return plan.SpecGraph{}, fmt.Errorf("parse project.json: %w", err)
	}

	specs := make(map[string]schema.ModuleSpec, len(proj.Modules))
	for _, mod := range proj.Modules {
		modPath := filepath.Join(specDir, mod.Path, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return plan.SpecGraph{}, fmt.Errorf("read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return plan.SpecGraph{}, fmt.Errorf("parse %s: %w", modPath, err)
		}
		specs[mod.ID] = ms
	}

	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return plan.SpecGraph{}, fmt.Errorf("resolve profile: %w", err)
	}

	return plan.NewSpecGraph(proj, specs).WithProfile(profile), nil
}

// resolvePlanRegistration finds the run's registration: the eid of the
// latest `registered` event in the journal whose proposal matches proposal,
// or plan.Registration{} if the journal holds none — read from the
// journal's parsed events rather than
// the fold, since a `registered` event with no task_created yet has no
// task-bearing pairing to appear in the fold as.
func resolvePlanRegistration(events []mapping.Event, proposal string) plan.Registration {
	var reg plan.Registration
	for _, ev := range events {
		if ev.Event == "registered" && ev.Proposal == proposal {
			reg = plan.Registration{EID: ev.EID}
		}
	}
	return reg
}

// planPairingFromEntry adapts one journal fold entry into plan.Pairing,
// joining live task status by task id when taskStatus is non-nil
// (spec/plan/arch_plan_command.md, pre-flight step 5).
func planPairingFromEntry(e mapping.FoldEntry, taskStatus map[string]string) plan.Pairing {
	var after string
	if e.Source.After != nil {
		after = *e.Source.After
	}
	p := plan.Pairing{
		SpecNodeID: e.Key,
		TaskID:     e.TaskID,
		NodeType:   e.Source.NodeType,
		Module:     e.Source.Module,
		Name:       e.Source.Name,
		After:      after,
	}
	if status, ok := taskStatus[e.TaskID]; ok {
		p.BeadStatus = status
	}
	return p
}

// planPairingsForMatching lists the journal fold's task-bearing spec-node
// pairings NodeMatcher correlates against the diff's changes: proposal-epic
// entries (no Source.Node) and removed-node tombstones are excluded, since
// neither ever matches a changed spec node by identity hash.
func planPairingsForMatching(fold mapping.Fold, taskStatus map[string]string) []plan.Pairing {
	var out []plan.Pairing
	for _, e := range fold.Entries {
		if e.Removed || e.Source.Node == "" {
			continue
		}
		out = append(out, planPairingFromEntry(e, taskStatus))
	}
	return out
}

// planFold adapts mapping.Fold onto plan.Fold (Resolver's FoldLookup plus
// IdempotencyLabeler's RemovalLookup), indexed once at construction for O(1)
// reads. Unlike planPairingsForMatching, Lookup covers every task-bearing
// entry including proposal-epic ones — Resolver's epic and parent
// resolution key off the proposal slug, not a spec node identity hash.
type planFold struct {
	lookup  map[string]plan.Pairing
	removal map[string]plan.RemovalEntry
}

func newPlanFold(fold mapping.Fold, taskStatus map[string]string) planFold {
	lookup := make(map[string]plan.Pairing, len(fold.Entries))
	removal := make(map[string]plan.RemovalEntry, len(fold.Entries))
	for _, e := range fold.Entries {
		if e.Removed {
			removal[e.Key] = plan.RemovalEntry{Removed: true, EID: e.Source.EID}
			continue
		}
		lookup[e.Key] = planPairingFromEntry(e, taskStatus)
	}
	return planFold{lookup: lookup, removal: removal}
}

func (f planFold) Lookup(key string) (plan.Pairing, bool) {
	p, ok := f.lookup[key]
	return p, ok
}

func (f planFold) Removal(specNodeID string) (plan.RemovalEntry, bool) {
	e, ok := f.removal[specNodeID]
	return e, ok
}

// writePlanChangeset serializes cs as canonical 2-space-indented JSON. With
// an empty outPath the encoded form goes to stdout. Otherwise a temp file +
// rename ensures the target path holds either the previous run's changeset
// or the new one, never a splice of the two (spec/plan/arch_plan_command.md,
// "Composability").
func writePlanChangeset(cs plan.Changeset, outPath string, stdout io.Writer) error {
	enc := func(w io.Writer) error {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		e.SetEscapeHTML(false)
		return e.Encode(cs)
	}
	if outPath == "" {
		return enc(stdout)
	}
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("plan: create out: %w", err)
	}
	if err := enc(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("plan: encode changeset: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("plan: close out: %w", err)
	}
	return os.Rename(tmp, outPath)
}

// planError carries a process exit code alongside the wrapped error. main
// inspects the ExitCode interface to honor the codes documented in
// arch_plan_command.md's "Exit Codes": 1 for input validation, 2 for a
// contract refusal.
type planError struct {
	code int
	err  error
}

func (e *planError) Error() string { return e.err.Error() }
func (e *planError) Unwrap() error { return e.err }
func (e *planError) ExitCode() int { return e.code }

func planInputErr(err error) error   { return &planError{code: 1, err: err} }
func planRefusalErr(err error) error { return &planError{code: 2, err: err} }
