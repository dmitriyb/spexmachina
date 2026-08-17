package plan

// ChangesetVersion is the wire-format version of changeset.json. Version 3
// added the retarget op and the top-level absorbed array to the
// vocabulary — a v2 consumer must refuse a version it does not know
// rather than silently drop ops it cannot execute. Version 2 had already
// dropped the ref:spec_node shape from the adapter-facing vocabulary; v3
// keeps that: an op's deps and target reach the adapter as ref:op or
// ref:bead only.
const ChangesetVersion = 3

// Op type vocabulary. Tool-agnostic — adapters for br, bd, GitHub Issues,
// or Jira consume the same vocabulary. Label and tag are reserved for
// future flows (e.g. cross-proposal tagging); no plan component emits
// them yet.
const (
	OpCreate   = "create"
	OpClose    = "close"
	OpRetarget = "retarget"
	OpLabel    = "label"
	OpTag      = "tag"
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
// the only two shapes v2 kept and v3 does not reopen: a dep that is
// neither in-batch (ref:op) nor already tracked (ref:bead) is a plan
// error at build time, never a deferred adapter-side lookup.
const (
	RefBead = "bead"
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

// Type tiers control create-op ordering (spec/plan/flow_plan.md, step 5;
// spec/plan/arch_topological_sorter.md, "Ordering Rules"). Within each
// tier TopologicalSorter applies Kahn's algorithm with a lex-spec_node_id
// tiebreak. Lower tiers emit first: the proposal epic, then components and
// data_flows together, then multi-component test sections.
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

// Ref encodes a forward-resolvable reference. Exactly one of BeadID or
// OpID is set per Ref; omitempty keeps the JSON clean.
//
//	{ "ref": "bead", "bead_id": "<id>" }   pre-existing bead
//	{ "ref": "op",   "op_id":   "<id>" }   another op in this changeset
//
// EdgeType is optional and carries the dep edge label ("blocks") on
// obsolete+create and cleanup-create lineage refs.
type Ref struct {
	Kind     string `json:"ref"`
	BeadID   string `json:"bead_id,omitempty"`
	OpID     string `json:"op_id,omitempty"`
	EdgeType string `json:"type,omitempty"`
}

// Idem carries the idempotency label the adapter matches against the
// tracker before creating a bead. A pre-existing match yields a
// was_existing=true receipt rather than a duplicate bead. Retarget ops
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

// Changeset is the v3 output schema written to stdout or --out. Field
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
// spec-graph ids, never bead ids: Resolver classifies DepSpecNodeIDs into
// Ref shapes three steps later in the same process, so this type crosses
// no file boundary and no command seam.
//
// BeadID is the existing bead on an obsolete or retarget, empty on a
// create. SpecHash is set on a create or retarget; OldBeadID is set on a
// create that replaces an obsoleted bead. ChangeType ("modified" or
// "removed") is set on an obsolete only. DepSpecNodeIDs is collected for
// create and retarget actions only — an obsolete inherits its bead's
// existing graph position.
type Action struct {
	Type           string
	BeadID         string
	Module         string
	Node           string
	NodeType       string
	SpecNodeID     string
	SpecHash       string
	OldBeadID      string
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
type OrderedOp struct {
	OpID   string
	Action Action
}
