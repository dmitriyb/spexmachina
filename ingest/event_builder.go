package ingest

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
)

// EventBuilderState is the per-run state every EventBuilder construction
// path shares — the eid predicate over journal and in-flight batch, the
// journal fold, the same-batch removals, the registered-by-stem map and
// the modified-handled set. Reconciler assembles it once per run and
// hands it to EventBuilder at construction, instead of threading it
// through every call. See spec/ingest/arch_event_builder.md, "Per-Run
// State".
type EventBuilderState struct {
	// SpecGraph resolves a fresh or modified node's current name, kind and
	// module — the identity a change event must carry, because the event
	// is the only record of it once the node is gone. Cleanup and
	// proposal-epic creates never reach it.
	SpecGraph SpecGraph
	// Fold is the journal's live pairings, epic slugs and legacy lines, as
	// of the start of this run.
	Fold mapping.Fold
	// SameBatchRemovals maps a node identity hash this batch's ok
	// "Spec node removed" closes will retire to the eid their removed
	// event will carry — resolved before any op is processed, so a
	// cleanup create resolves its referent regardless of op order.
	SameBatchRemovals map[string]string
	// RegisteredByStem maps a proposal stem to its `registered` event's
	// eid, for proposal-epic creates.
	RegisteredByStem map[string]string
	// ModifiedHandled records which beads' modified pairs an earlier
	// create in this batch has already built, so the paired close
	// constructs nothing twice.
	ModifiedHandled map[string]bool
	// HasEID answers whether an eid is already present, in the journal or
	// in the batch constructed so far. It is mutated as the batch grows,
	// so a mid-batch collision is caught exactly as a journal-side
	// duplicate — the mechanism that makes the batch idempotent.
	HasEID func(eid string) bool
}

// EventBuilder constructs journal lines per action class from an op (or
// absorbed entry) and its receipt: the create paths (plain, cleanup,
// epic), the modified-node pair, retargets, removal closes and absorbed
// entries — every receipt references an event. Reconciler builds one
// per run and calls it once per op. See
// spec/ingest/arch_event_builder.md and requirements 539030e8c5a4,
// 7191a50f7447, 7900dcd38c4a, fd6f08ef34fa in spec/ingest/module.json.
//
// TODO(bead:spexmachina-ugrs.2): implement the construction paths per
// arch_event_builder.md's "Per-Op Construction Table". The working
// logic for these paths currently lives inline as Reconciler's
// buildCreate/buildRemoved/buildModifiedPair/buildModifiedFromClose/
// buildRetarget/buildAbsorbed helpers (ingest/reconciler.go); this bead
// extracts it into the methods below, and spexmachina-ugrs.5 rewires
// Reconciler.Apply to dispatch through them.
type EventBuilder struct {
	State EventBuilderState
}

// NewEventBuilder constructs an EventBuilder over the given per-run
// state. See EventBuilderState and arch_event_builder.md's "Per-Run
// State".
func NewEventBuilder(state EventBuilderState) *EventBuilder {
	return &EventBuilder{State: state}
}

// BuildCreate answers the journal lines an ok create op implies,
// discriminating spec_node_kind (proposal_epic, cleanup, other) before
// consulting the spec graph. See "Proposal-Epic Ops" and "Cleanup-Create
// Ops" in arch_event_builder.md.
func (b *EventBuilder) BuildCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	return nil, fmt.Errorf("ingest: EventBuilder.BuildCreate: not implemented (bead:spexmachina-ugrs.2)")
}

// BuildClose answers the journal lines an ok close op implies: the
// removed event plus task_closed on a "Spec node removed" reason, or —
// when no create in the batch claimed the bead — the modified event
// plus task_closed on a "Spec node modified" reason. See "The
// Modified-Node Pair" in arch_event_builder.md.
func (b *EventBuilder) BuildClose(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	return nil, fmt.Errorf("ingest: EventBuilder.BuildClose: not implemented (bead:spexmachina-ugrs.2)")
}

// BuildRetarget answers the modified event plus task_retargeted receipt
// an ok retarget op implies. See "Retarget Ops" in
// arch_event_builder.md.
func (b *EventBuilder) BuildRetarget(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	return nil, fmt.Errorf("ingest: EventBuilder.BuildRetarget: not implemented (bead:spexmachina-ugrs.2)")
}

// BuildAbsorbed answers one modified event per entry in the changeset's
// top-level absorbed array, plus the single refresh receipt naming the
// batch's newly-built absorbed eids. See "Absorbed Entries" in
// arch_event_builder.md.
func (b *EventBuilder) BuildAbsorbed(cs plan.Changeset) ([]mapping.Event, error) {
	return nil, fmt.Errorf("ingest: EventBuilder.BuildAbsorbed: not implemented (bead:spexmachina-ugrs.2)")
}
