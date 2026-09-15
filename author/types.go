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
// now attach to the tree, and — for `spex node rename` only — the
// retired name the vocabulary sweep needs
// (spec/author/arch_author_commands.md, "Exit codes and output").
// Obligations reuses merkle.DiffError, the completeness checker's own
// entry type, unchanged: they are printed here, not re-derived
// (spec/author/arch_obligation_reporter.md, "Obligations are printed, not
// discovered").
type WriteReport struct {
	Written     []string           `json:"written"`
	Obligations []merkle.DiffError `json:"obligations"`
	RetiredName string             `json:"retired_name,omitempty"`
}
