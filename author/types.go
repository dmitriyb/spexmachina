package author

import "github.com/dmitriyb/spexmachina/merkle"

// Exit-code vocabulary for the eight `spex node|edge|leaf|profile|migrate`
// surfaces, per arch_author_commands.md's "Exit codes and output". Unlike
// spex diff/plan/ingest, there is no not-a-spex-project code here: none of
// these commands runs a pre-flight against .spex/, because an uninitialised
// project is their intended first use — a spec directory holding no
// project.json is ExitInputError like any other malformed input.
const (
	ExitOK         = 0
	ExitInputError = 1
	ExitRefusal    = 2
)

// NodeAddInput is what `spex node add` hands to NodeEditor: a
// profile-declared type name, the node's name, a module name when the
// type is module-scoped (empty for a project-scoped type or for
// --type module itself), and one value per field the profile declares
// for that type, keyed by declared field name. NodeEditor derives
// everything the caller does not name — array, id, content path — from
// the resolved profile and the identity contract; this struct never
// carries any of them (spec/author/arch_node_editor.md, "Adding a node").
type NodeAddInput struct {
	TypeName string
	Name     string
	Module   string
	Fields   map[string]string
}

// NodeRemoveInput is what `spex node remove` hands to NodeEditor: the
// node's id, and whether to remove it over inbound references that still
// name it (spec/author/arch_node_editor.md, "Removing a node").
type NodeRemoveInput struct {
	ID    string
	Force bool
}

// NodeSetInput is what `spex node set` hands to NodeEditor: an existing
// node's id, one or more declared field values to write, one or more
// declared field names to unset, or both in one invocation — a name given
// to both is refused. Fields is keyed by declared field name exactly as
// NodeAddInput.Fields is, and its values are converted by kind the same
// way `spex node add` converts them: an integer field refuses a
// non-integer and an enumerated field refuses a value outside its
// enumeration, each with the validator's own schema entry and the
// declared kind or enumeration as the fix
// (spec/author/arch_node_editor.md, "Setting a field"). A reference
// field, `name`, `id`, `content` or a module id named in Fields or Unset
// is refused with the surface that owns it — NodeEditor's own guard, not
// the validator's — naming `spex edge add`/`spex edge remove`,
// `spex node rename`, "derived", or the hand edit respectively
// (spec/author/arch_node_editor.md, "Setting a field": "Every field this
// command will not touch has a surface that owns it").
type NodeSetInput struct {
	ID     string
	Fields map[string]string
	Unset  []string
}

// RenameInput is what `spex node rename` hands to NodeRenamer: the
// node's id and its new name. NodeRenamer derives the new id itself from
// the node's scope, type and the new name (spec/author/arch_node_renamer.md).
type RenameInput struct {
	ID      string
	NewName string
}

// EdgeInput is what `spex edge add` and `spex edge remove` hand to
// EdgeEditor: one entry of one reference field on one node, named by id
// throughout — a source node, a field name and a target node
// (spec/author/arch_edge_editor.md, "One entry, four checks").
type EdgeInput struct {
	SourceID string
	Field    string
	TargetID string
}

// ScaffoldInput is what `spex leaf scaffold` hands to LeafScaffolder: an
// existing node's id (spec/author/arch_leaf_scaffolder.md).
type ScaffoldInput struct {
	ID string
}

// RefusalEntry is one finding in a refusal document: the validator's own
// check and message for an entry the after-state introduced that the
// before-state did not carry, plus the fix ObligationReporter computed
// for it — the command, flag or value that resolves it
// (spec/author/arch_obligation_reporter.md, "Every refusal names its
// fix"). Field order is the canonical JSON field order
// flow_authoring.md's "Out of the reporter" names: check, message, path,
// fix. Fix is never empty — every refusal carries one.
type RefusalEntry struct {
	Check   string `json:"check"`
	Message string `json:"message"`
	Path    string `json:"path"`
	Fix     string `json:"fix"`
}

// WriteReport is the write-report document a writing command prints on
// stdout once ObligationReporter has accepted a change and the caller has
// written it: the files written, the obligations the completeness rules
// now attach to the tree, — for `spex node rename` and for a
// `spex node remove` of a name-declarable node — the retired name the
// vocabulary sweep needs, and — for a `spex edge add` that retargeted a
// cardinality-one field already holding a different target — the target
// it displaced
// (spec/author/arch_author_commands.md, "Exit codes and output";
// spec/author/flow_authoring.md, "On stdout"). Obligations reuses
// merkle.DiffError, the completeness checker's own entry type, unchanged:
// they are printed here, not re-derived
// (spec/author/arch_obligation_reporter.md, "Obligations are printed, not
// discovered").
//
// ReplacedTarget is EdgeEditor's field, populated by AddEdge when a
// cardinality-one field already held a different target
// (spec/author/arch_edge_editor.md, "Idempotence": "add sets it, and
// adding a different target replaces the one held, the write report
// carrying the replaced target under `replaced_target`").
type WriteReport struct {
	Written        []string           `json:"written"`
	Obligations    []merkle.DiffError `json:"obligations"`
	RetiredName    string             `json:"retired_name,omitempty"`
	ReplacedTarget string             `json:"replaced_target,omitempty"`
}
