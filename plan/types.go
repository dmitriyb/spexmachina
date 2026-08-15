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

// Label vocabulary the changeset and the external adapter agree on.
// IdempotencyLabelPrefix names the eid-keyed label an adapter matches
// before creating a bead; ObsoleteLabel and CommitLabelPrefix mark a
// close op's target as superseded as of the given git HEAD; CleanupLabel
// discriminates a cleanup create so the adapter and the reconciler can
// key off it without depending on SpecNodeKind alone.
const (
	IdempotencyLabelPrefix = "spex:"
	ObsoleteLabel          = "spex:obsolete"
	CommitLabelPrefix      = "commit:"
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
// "blocks") and Labels ("spex:cleanup" on cleanup only). Retarget ops use
// SpecNodeID, SpecHash, Target, Labels, Deps and Reason. Close ops use
// Target, Labels and Reason. Every other field is elided via omitempty.
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
