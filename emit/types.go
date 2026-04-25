package emit

// Changeset version constant. Bumped when the schema changes in a
// non-backwards-compatible way; emit always writes the latest.
const ChangesetVersion = 1

// Op type vocabulary. Tool-agnostic — adapters for br, bd, GitHub Issues,
// or Jira consume the same vocabulary.
const (
	OpCreate = "create"
	OpClose  = "close"
	OpLabel  = "label"
	OpTag    = "tag"
)

// Ref kind discriminator values. Each Ref is exactly one of these three.
const (
	RefBead     = "bead"
	RefOp       = "op"
	RefSpecNode = "spec_node"
)

// Type tiers control op ordering. Within each tier TopologicalSorter
// applies Kahn's algorithm with lex-spec_node_id tiebreak. Lower tiers
// emit first.
const (
	TierProposalEpic  = 0
	TierFeatureOrFlow = 1
	TierMultiCompTest = 2
)

// FallbackPriority is applied when a component's
// implements → module_requirement.preq_id → project_requirement.priority
// chain cannot be walked. Mid-range value chosen so unspec'd work neither
// blocks high-priority items nor sinks below low-priority cleanup work.
const FallbackPriority = 3

// Ref encodes a forward-resolvable reference. Exactly one of BeadID, OpID,
// or SpecNodeID is set per Ref; omitempty keeps the JSON clean.
//
//	{ "ref": "bead",      "bead_id":      "<id>" }   pre-existing bead
//	{ "ref": "op",        "op_id":        "<id>" }   another op in this changeset
//	{ "ref": "spec_node", "spec_node_id": "<id>" }   adapter-time fallback
//
// EdgeType is optional and carries the dep edge label (e.g. "blocks") on
// obsolete+create lineage refs.
type Ref struct {
	Kind       string `json:"ref"`
	BeadID     string `json:"bead_id,omitempty"`
	OpID       string `json:"op_id,omitempty"`
	SpecNodeID string `json:"spec_node_id,omitempty"`
	EdgeType   string `json:"type,omitempty"`
}

// Idem carries the idempotency label the adapter matches against the
// tracker before creating a bead. A pre-existing match yields a
// was_existing=true skipped receipt rather than a duplicate bead.
type Idem struct {
	Label string `json:"label"`
}

// Op is the canonical operation record. Field order on this struct IS the
// canonical JSON field order — do not reorder. Create ops use SpecNodeKind
// through Body; close ops use Target / Labels / Reason. Other fields are
// elided via omitempty.
type Op struct {
	OpID         string   `json:"op_id"`
	Type         string   `json:"type"`
	SpecNodeKind string   `json:"spec_node_kind,omitempty"`
	SpecNodeID   string   `json:"spec_node_id,omitempty"`
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

// Changeset is the v1 output schema. Field order is fixed; ops are emitted
// in the order TopologicalSorter produced and never re-sorted at write time.
type Changeset struct {
	Version  int    `json:"version"`
	GitHead  string `json:"git_head"`
	Proposal string `json:"proposal"`
	Ops      []Op   `json:"ops"`
}

// CreateAction is the per-create slice of an impact report relevant to
// emit. Sorter and Resolver consume it; impact.Action's obsolete-only
// fields are dropped at the emit boundary.
type CreateAction struct {
	SpecNodeID     string
	NodeType       string
	Module         string
	Node           string
	SpecHash       string
	OldBeadID      string
	DepSpecNodeIDs []string
}

// OrderedOp pairs a CreateAction with the op_id assigned by Sorter.
// Resolver consumes []OrderedOp + Sorter's batchMap to classify each
// create's deps and parent into the three ref shapes.
type OrderedOp struct {
	OpID   string
	Action CreateAction
}
