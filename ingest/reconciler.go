package ingest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
)

// SpecGraph supplies the spec-side metadata the Reconciler needs to build a
// fresh or modified node's change event: its name, kind and module.
// Cleanup and proposal-epic creates never reach it — see
// spec/ingest/arch_event_builder.md "Proposal-Epic Ops" and "Cleanup-Create
// Ops".
type SpecGraph interface {
	NodeMetadata(specNodeID string) (NodeMetadata, error)
}

// NodeMetadata is the subset of spec-graph data a change event needs.
// ContentFile is the node's content-leaf path (the event's "path" field);
// SpecHash is the merkle leaf hash (the event's "after" field on add/modify).
type NodeMetadata struct {
	Module      string
	Component   string
	ContentFile string
	SpecHash    string
	NodeType    string
}

// ReconcileSummary is the per-op tally Reconciler.Apply returns. The CLI
// folds OkCreates+OkCloses+OkRetargets into Summary.Ok before serialising
// to stdout. EventsAppended and ReceiptsAppended count only lines that
// actually landed on this call — an idempotent re-run reports zero of
// each even though every op still counts as ok.
type ReconcileSummary struct {
	OkCreates        int
	OkCloses         int
	OkRetargets      int
	Skipped          int
	Errors           int
	EventsAppended   int
	ReceiptsAppended int
}

// Reconciler consumes a paired changeset+receipts and appends the journal
// lines each op implies to spec/.history.jsonl: change events (added,
// modified, removed) and task receipts (task_created, task_closed).
// Event ids derive from (git_head, op_id), so a line whose eid the journal
// already carries is dropped rather than re-appended — the batch is
// idempotent by construction. Invariants 1, 2 and 5 are asserted against
// the whole batch before anything reaches disk; invariant 4 (snapshot
// saved iff complete) is SnapshotSaver's gate, not this component's.
type Reconciler struct {
	// SpecDir is the spec root.
	SpecDir string
	// JournalPath is the task journal's resolved location. Defaults to
	// <SpecDir>/.history.jsonl when empty; the shipped command sets it to
	// the location inside .spex/ the lifecycle pre-flight resolved, so
	// this component computes no location of its own.
	JournalPath string
	// SpecGraph resolves a fresh or modified node's current metadata.
	SpecGraph SpecGraph
}

// Apply pairs each changeset op with its receipt, assembles the per-run
// state and hands it to one EventBuilder, dispatches every op (and the
// changeset's absorbed entries) to it in changeset order, then runs
// InvariantChecker and JournalEncoder over the constructed batch before
// committing it atomically through MappingStore. On any error the on-disk
// journal is untouched. See arch_reconciler.md, "Interface" and
// "Transaction Semantics".
func (r *Reconciler) Apply(cs plan.Changeset, rc adapters.Receipts) (ReconcileSummary, error) {
	receiptsByOp, err := pairReceipts(cs, rc)
	if err != nil {
		return ReconcileSummary{}, err
	}

	// The resolved profile gates JournalEncoder's node_type membership
	// check (invariant 5) below — a profile.json's declared types, not a
	// compiled-in default, decide which change events this run may write.
	profile, err := schema.ResolveProfile(r.SpecDir)
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: %w", err)
	}

	journalPath := r.JournalPath
	if journalPath == "" {
		journalPath = filepath.Join(r.SpecDir, ".history.jsonl")
	}
	store := mapping.NewMappingStore(journalPath)
	existing, err := store.Parse()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}
	fold, err := store.List()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}

	existingEIDs := make(map[string]bool, len(existing))
	registeredByStem := map[string]string{}
	for _, ev := range existing {
		if ev.EID != "" {
			existingEIDs[ev.EID] = true
		}
		if ev.Event == "registered" {
			registeredByStem[ev.Proposal] = ev.EID
		}
	}
	batchEIDs := map[string]bool{}
	hasEID := func(eid string) bool { return existingEIDs[eid] || batchEIDs[eid] }

	builder := NewEventBuilder(EventBuilderState{
		SpecGraph:         r.SpecGraph,
		Fold:              fold,
		SameBatchRemovals: sameBatchRemovals(cs, receiptsByOp, fold),
		RegisteredByStem:  registeredByStem,
		HasEID:            hasEID,
	})

	var (
		sum   ReconcileSummary
		batch []mapping.Event
	)
	appendBatch := func(lines []mapping.Event) {
		for _, ev := range lines {
			batch = append(batch, ev)
			if ev.EID != "" {
				batchEIDs[ev.EID] = true
			}
		}
	}

	for _, op := range cs.Ops {
		opRC := receiptsByOp[op.OpID]

		switch op.Type {
		case plan.OpClose:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkCloses++
				// BuildClose constructs its own pair unconditionally for a
				// "Spec node modified" reason — no other op in the batch
				// can claim it, since node-bearing creates carry no
				// lineage dep. See "Fold-Back Closes" in
				// arch_event_builder.md.
				lines, err := builder.BuildClose(cs, op, opRC)
				if err != nil {
					return ReconcileSummary{}, err
				}
				appendBatch(lines)
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		case plan.OpCreate:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkCreates++
				lines, err := builder.BuildCreate(cs, op, opRC)
				if err != nil {
					return ReconcileSummary{}, err
				}
				appendBatch(lines)
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		case plan.OpRetarget:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkRetargets++
				lines, err := builder.BuildRetarget(cs, op, opRC)
				if err != nil {
					return ReconcileSummary{}, err
				}
				appendBatch(lines)
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		default:
			return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown type %q", op.OpID, op.Type)
		}
	}

	// The changeset's top-level absorbed array is not receipt-gated — it
	// describes spec state rather than tracker work, so it is processed
	// regardless of what the ops loop above found. See
	// arch_event_builder.md "Absorbed Entries".
	absorbedLines, err := builder.BuildAbsorbed(cs)
	if err != nil {
		return ReconcileSummary{}, err
	}
	appendBatch(absorbedLines)

	if len(batch) > 0 {
		if err := NewInvariantChecker().Check(existing, batch); err != nil {
			return ReconcileSummary{}, err
		}
		if err := checkInvariant5(batch, profile); err != nil {
			return ReconcileSummary{}, err
		}
		if err := store.Append(batch); err != nil {
			return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: commit: %w", err)
		}
	}

	for _, ev := range batch {
		switch ev.Event {
		case "added", "modified", "removed":
			sum.EventsAppended++
		case "task_created", "task_closed", "task_retargeted", "refresh":
			sum.ReceiptsAppended++
		}
	}
	return sum, nil
}

// sameBatchRemovals maps every node identity hash this batch's ok
// "Spec node removed" closes will retire to the eid their removed event
// will carry — computed once, before any op is processed, so a cleanup
// create for that same hash resolves its referent regardless of whether
// plan placed the cleanup's create before or after its own removal close
// (real changesets always put it before — see arch_reconciler.md
// "Ordering"). Node hash comes from the fold's live entry for the close's
// target bead, exactly as buildRemoved resolves it.
//
// TODO(bead:spexmachina-swvx.24): current arch_reconciler.md ("No
// op's lines depend on another op in the batch") retires this batch-wide
// pre-pass along with the modified-node pair: a cleanup create no longer
// needs a same-batch removal close to exist at all, since it now mints
// its own removal when the fold's latest event for the node isn't
// already one (landed in event_builder.go's buildCleanupCreate). Drop
// this helper, the EventBuilderState.SameBatchRemovals field and this
// call site — the result of this function has no remaining reader.
func sameBatchRemovals(cs plan.Changeset, receiptsByOp map[string]adapters.OpReceipt, fold mapping.Fold) map[string]string {
	out := map[string]string{}
	for _, op := range cs.Ops {
		if op.Type != plan.OpClose || !strings.HasPrefix(op.Reason, ReasonRemovedPrefix) {
			continue
		}
		if receiptsByOp[op.OpID].Status != adapters.OpStatusOk {
			continue
		}
		if op.Target == nil || op.Target.Kind != plan.RefTask || op.Target.TaskID == "" {
			continue
		}
		for _, e := range fold.Entries {
			if e.TaskID == op.Target.TaskID {
				out[e.Key] = deriveEID(cs.GitHead, op.OpID)
				break
			}
		}
	}
	return out
}

// pairReceipts builds an op_id → OpReceipt index after asserting that the
// changeset and receipts cover exactly the same op_id set. An imbalance is
// a contract violation by plan or the adapter and is treated as input
// error, not invariant failure.
func pairReceipts(cs plan.Changeset, rc adapters.Receipts) (map[string]adapters.OpReceipt, error) {
	byOp := make(map[string]adapters.OpReceipt, len(rc.Ops))
	for _, or := range rc.Ops {
		if _, dup := byOp[or.OpID]; dup {
			return nil, fmt.Errorf("ingest: reconcile: duplicate receipt op_id %s", or.OpID)
		}
		byOp[or.OpID] = or
	}
	seen := make(map[string]bool, len(cs.Ops))
	for _, op := range cs.Ops {
		if _, ok := byOp[op.OpID]; !ok {
			return nil, fmt.Errorf("ingest: reconcile: no receipt for op %s", op.OpID)
		}
		seen[op.OpID] = true
	}
	for opID := range byOp {
		if !seen[opID] {
			return nil, fmt.Errorf("ingest: reconcile: receipt op_id %s not in changeset", opID)
		}
	}
	return byOp, nil
}

// deriveEID derives a change event's id from the batch's git_head and the
// constructing op's own id. Re-deriving the same pair on a later run
// yields the same eid — the mechanism that makes the whole batch
// idempotent by construction. It delegates to plan.DeriveEID, the same
// formula IdempotencyLabeler mints the op's spex:<eid> label from, so the
// label and the event it names can never drift apart (see
// spec/map/flow_task_mapping.md, "Data Flow").
func deriveEID(gitHead, opID string) string {
	return plan.DeriveEID(gitHead, opID)
}

// Invariants 1 and 2 (double-pairing and dangling referents) are
// InvariantChecker's contract now — see invariant_checker.go.
// checkInvariant5, the wire-shape types (changeEventLine and friends) and
// encodeLine moved to journal_encoder.go as JournalEncoder's own contract
// (spexmachina-ugrs.4) — checkInvariant5 delegates line-by-line to
// JournalEncoder.Validate instead of carrying its own schema-compile and
// encode logic, so this file and refresh.go both inherit the gate rather
// than re-implementing it.

func strPtr(s string) *string { return &s }

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
