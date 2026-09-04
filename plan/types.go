package plan

// ChangesetVersion is the wire-format version of changeset.json. Version 4
// retired the close-and-recreate step and its lineage dependency, dropped
// the reserved label and tag op kinds, and renamed the task ref shape to
// {"ref":"task","task_id":…} — a v3 consumer would resolve no v4 task ref
// at all and must refuse the document rather than execute half of it.
// Version 3 added the retarget op and the top-level absorbed array; version
// 2 had already dropped the ref:spec_node shape from the adapter-facing
// vocabulary, and v4 keeps that: an op's deps and target reach the adapter
// as ref:op or ref:task only.
//
// This bump lands plan's wire shape ahead of the component rewrites that
// consume it. ActionClassifier (spexmachina-swvx.16) now implements the
// target shape: a genuinely changed node with an open task retargets in
// place, and a task absent from the artifact yields one plain create
// carrying no old task id and no lineage dep — no close-and-recreate step.
// Resolver (spexmachina-swvx.19) now drops a dep on a fold pairing whose
// status is anything other than live (open or in_progress) rather than
// checking for the literal "closed" the obsolete-then-recreate shape used
// to write — there is no closed status in the vocabulary, only absence.
// ChangesetBuilder (spexmachina-swvx.20) still carries remnants of the
// pre-task-lifecycle "obsolete, then recreate" shape pending its own bead;
// IdempotencyLabeler (spexmachina-swvx.13) already self-mints a cleanup
// create's label from its own (git_head, op_id) when neither the journal
// fold nor a same-batch close answers. The task-state artifact (--tasks,
// TaskReader) that replaces --beads/BeadReader is spexmachina-swvx.14 and
// spexmachina-swvx.7. See spec/plan/flow_plan.md.
const ChangesetVersion = 4

// Op type vocabulary. Tool-agnostic — adapters for br, bd, GitHub Issues,
// or Jira consume the same vocabulary. Exactly the three kinds v4 emits:
// the reserved label and tag kinds left the vocabulary with the bump to 4,
// because a vocabulary is closed only when it lists what is actually
// produced.
const (
	OpCreate   = "create"
	OpClose    = "close"
	OpRetarget = "retarget"
)

// spec_node_kind vocabulary carried by create ops. The closure is
// enforced upstream by ActionClassifier: it never emits an action
// carrying node_type=api, so no changeset op ever names one. On a
// conventional create the builder copies Action.NodeType into
// Op.SpecNodeKind verbatim; KindCleanup is the one override, applied to
// the cleanup-bead shape regardless of the removed node's own kind.
const (
	KindProposalEpic = "proposal_epic"
	KindComponent    = "component"
	KindDataFlow     = "data_flow"
	KindTestSection  = "test_section"
	KindCleanup      = "cleanup"
)

// Ref kind discriminator values. Each Ref is exactly one of these two —
// the only two shapes v2 kept and later versions do not reopen: a dep
// that is neither in-batch (ref:op) nor already tracked (ref:task) is a
// plan error at build time, never a deferred adapter-side lookup. v4
// renames the tracked-task shape's kind from "bead" to "task" — see
// Ref.TaskID.
const (
	RefTask = "task"
	RefOp   = "op"
)

// Action type vocabulary — ActionClassifier's answer for what happened to
// a spec node (see spec/plan/arch_action_classifier.md, "State Transition
// Table"). Distinct from the Op type vocabulary above: an ActionObsolete
// becomes an OpClose at the builder, never an op literally named
// "obsolete".
const (
	ActionCreate   = "create"
	ActionObsolete = "obsolete"
	ActionRetarget = "retarget"
)

// Type tiers control create-op ordering (spec/plan/arch_topological_sorter.md,
// "Ordering Rules"). Within each tier TopologicalSorter applies Kahn's
// algorithm with a lex-spec_node_id tiebreak. Lower tiers emit first: the
// proposal epic, then components and data_flows together, then
// multi-component test sections.
//
// TODO(bead:spexmachina-swvx.38): spec/plan/flow_plan.md step 5, as
// corrected by the 822b817 baseline (module.json's abfb10394fdd, "Layer
// order as blocking edges"), replaces this fixed three-tier split with one
// layer per plan-relevant node type in the resolved profile's own order —
// under the default profile that separates data_flow from component
// instead of sharing TierFeatureOrFlow — plus a final layer for cleanup
// creates, which this table does not carve out today. These three
// constants and plan/sorter.go's tierOf stay as the pre-correction shape
// until TopologicalSorter's own bead replaces them with a profile-driven
// layer list.
const (
	TierProposalEpic  = 0
	TierFeatureOrFlow = 1
	TierMultiCompTest = 2
)

// FallbackPriority is the priority a create op carries when the
// implements -> preq_id -> priority chain cannot be walked
// (spec/plan/flow_plan.md, "Error Paths": "Missing project requirement in
// priority chain -> default priority 3, silently"). No error, no warning,
// and nothing in the changeset marks the op.
const FallbackPriority = 3

// IdempotencyLabelPrefix names the eid-keyed label an adapter matches
// before creating a bead. The obsolete-close markers (spex:obsolete,
// commit:<HEAD>) and the spex:cleanup discriminator are retired: a close
// op carries no labels at all — close idempotency keys on the tracker's
// own status — and a cleanup create is distinguished by its
// SpecNodeKind, never by a label. CleanupLabel is kept as a historical
// constant for callers outside this package that still reference its
// literal value; ChangesetBuilder no longer emits it.
const (
	IdempotencyLabelPrefix = "spex:"
	CleanupLabel           = "spex:cleanup"
)

// DeriveEID derives a journal event id from this run's git_head and an
// op's own id — the one formula every participant in
// spec/map/flow_task_mapping.md's data flow shares: Labeler mints it to
// build the spex:<eid> idempotency label a create or retarget op carries,
// and ingest's EventBuilder derives it again, unchanged, to mint the
// referent event the label names. Sharing the formula, rather than each
// side reimplementing "git_head + op_id", is what keeps the label and the
// event it points at one fact instead of two copies that could drift.
// Node-bearing creates and retargets call this with their own op_id.
// cleanupLabel's same-batch-close fallback (plan/labeler.go) is also a
// caller: when the fold carries no removed event yet, it passes the
// same-batch close op's id in place of a create's own, deriving the label
// from the removal that close op is about to record. A cleanup whose
// removal already landed in an earlier batch reads the fold's removed
// event instead, and a proposal-epic create reads the run's registration;
// neither of those two calls this.
func DeriveEID(gitHead, opID string) string {
	return gitHead + ":" + opID
}

// Ref encodes a forward-resolvable reference. Exactly one of TaskID or
// OpID is set per Ref; omitempty keeps the JSON clean. v4 renamed the
// pre-existing-task shape's kind from "bead" to "task" and its id field
// from bead_id to task_id (spec/plan/flow_plan.md, "changeset.json
// (output)").
//
//	{ "ref": "task", "task_id": "<id>" }   pre-existing task
//	{ "ref": "op",   "op_id":   "<id>" }   another op in this changeset
//
// EdgeType is optional and carries the dep edge label ("blocks") on
// obsolete+create and cleanup-create lineage refs.
//
// TODO(bead:spexmachina-swvx.20): spec/plan/flow_plan.md's target v4 ref
// shape carries no edge-type field at all — the lineage edge was the only
// typed dep, and it leaves the vocabulary with the close-and-recreate step
// once ChangesetBuilder (and EventBuilder, spexmachina-swvx.22) stop
// needing it. EdgeType stays until that lands, so the field-rename this
// bead lands does not also strand the still-in-use obsolete+create/cleanup
// lineage mechanism mid-migration.
type Ref struct {
	Kind     string `json:"ref"`
	TaskID   string `json:"task_id,omitempty"`
	OpID     string `json:"op_id,omitempty"`
	EdgeType string `json:"type,omitempty"`
}

// Idem carries the idempotency label the adapter matches against the
// tracker before creating a task. A pre-existing match yields a
// was_existing=true receipt rather than a duplicate task. Retarget ops
// carry no Idem: updates are naturally idempotent, so there is nothing
// to probe for — the run's modified-event label rides in Op.Labels
// instead.
type Idem struct {
	Label string `json:"label"`
}

// Op is the canonical operation record. Field order on this struct IS
// the canonical JSON field order — do not reorder (see
// spec/plan/arch_changeset_builder.md, "Canonical Output"): op_id, type,
// spec_node_kind, spec_node_id, spec_hash, idempotency, parent, deps,
// priority, title, body, target, labels, reason.
//
// Conventional creates use SpecNodeKind through Body; a cleanup or
// modify-pair create additionally carries Deps (lineage, edge type
// "blocks") but no Labels — creates never populate Labels. Retarget ops
// use SpecNodeID, SpecHash, Target, Labels, Deps and Reason: Labels is
// the one field only a retarget populates, carrying this run's
// modified-event label. Close ops use Target and Reason alone, no
// Labels — close idempotency keys on the tracker's own status. Every
// other field is elided via omitempty.
type Op struct {
	OpID         string   `json:"op_id"`
	Type         string   `json:"type"`
	SpecNodeKind string   `json:"spec_node_kind,omitempty"`
	SpecNodeID   string   `json:"spec_node_id,omitempty"`
	SpecHash     string   `json:"spec_hash,omitempty"`
	Idempotency  *Idem    `json:"idempotency,omitempty"`
	Parent       *Ref     `json:"parent,omitempty"`
	Deps         []Ref    `json:"deps,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Title        string   `json:"title,omitempty"`
	Body         string   `json:"body,omitempty"`
	Target       *Ref     `json:"target,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// AbsorbedEntry is one --absorb mark's finished record in the
// changeset's top-level absorbed array: the node's identity hash and its
// before/after content hashes from the diff, plus the authored reason
// copied from the absorb file. PlanCommand composes these off the
// withheld diff entries; ChangesetBuilder writes them verbatim in this
// field order. The adapter ignores the array; ingest consumes it to mint
// modified events keyed by (node, before, after).
type AbsorbedEntry struct {
	Node   string `json:"node"`
	Before string `json:"before"`
	After  string `json:"after"`
	Reason string `json:"reason"`
}

// Changeset is the v4 output schema written to stdout or --out. Field
// order is fixed: version, git_head, proposal, ops, absorbed. Ops are
// emitted in the order the classifier's deterministic action order and
// TopologicalSorter produced them — creates, then retargets, then
// closes — and are never re-sorted at write time.
type Changeset struct {
	Version  int             `json:"version"`
	GitHead  string          `json:"git_head"`
	Proposal string          `json:"proposal"`
	Ops      []Op            `json:"ops"`
	Absorbed []AbsorbedEntry `json:"absorbed"`
}

// Action is ActionClassifier's per-node verdict — the classifier ->
// builder-chain shape spec/plan/flow_plan.md's "ActionClassifier -> the
// builder chain" section names, tabulated in full by
// spec/plan/arch_action_classifier.md's "Interface" table. It carries
// spec-graph ids, never task ids: Resolver classifies DepSpecNodeIDs into
// Ref shapes three steps later in the same process, so this type crosses
// no file boundary and no command seam.
//
// TaskID is the existing task on an obsolete or retarget, empty on a
// create — a create never names a prior task
// (spec/plan/arch_action_classifier.md's Interface table). SpecHash is set
// on a create or retarget. OldTaskID is never set by ActionClassifier's own
// output any more — spexmachina-swvx.16 retired the close-and-recreate step
// whose create carried it as lineage to the task it replaced — but the
// field itself stays on Action for plan/builder.go's own use pending
// Resolver (spexmachina-swvx.19) and ChangesetBuilder (spexmachina-swvx.20).
// ChangeType ("modified" or "removed") is set on an obsolete only.
// DepSpecNodeIDs is collected for create and retarget actions only — an
// obsolete inherits its task's existing graph position.
type Action struct {
	Type           string
	TaskID         string
	Module         string
	Node           string
	NodeType       string
	SpecNodeID     string
	SpecHash       string
	OldTaskID      string
	DepSpecNodeIDs []string
	ChangeType     string
	Reason         string
}

// OrderedOp pairs an Action with the provisional op_id TopologicalSorter
// assigns it (spec/plan/arch_topological_sorter.md, "Interface").
// IdempotencyLabeler and Resolver consume []OrderedOp plus the sorter's
// spec_node_id-to-op_id map; ChangesetBuilder keeps the order and
// discards both, renumbering every op itself once the retarget and close
// ops are counted.
//
// TODO(bead:spexmachina-swvx.20): spec/plan/flow_plan.md's 822b817-corrected
// step 8 renumbers from each op's own canonical key — its kind plus the
// node or task it acts on — never from position; plan/builder.go's
// digit-padded op-%0*d renumbering is the pre-correction shape until
// ChangesetBuilder's own bead switches to the canonical-key form the
// changeset.json example (spec/plan/flow_plan.md, "changeset.json
// (output)") now shows.
type OrderedOp struct {
	OpID   string
	Action Action
}
