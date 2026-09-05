package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/schema"
)

// JournalEncoder turns a journal event or receipt into its serialized
// journal line and validates that line against the journal-line schema
// before it is written — invariant 5 of the mapping consistency
// invariants (requirement ee28b5d190ae, spec/ingest/module.json),
// expressed as a component rather than as helpers at the bottom of a
// larger file. A line that fails validation refuses the batch before
// the write path is reached; there is no partial append. Both
// Reconciler (normal mode, via checkInvariant5) and RefreshHandler
// (refresh mode) encode through it, so neither pathway can drift from
// the other's wire shape. See spec/ingest/arch_journal_encoder.md.
//
// Validate also holds the membership check the journal-line schema
// deliberately leaves open: a change event's node_type is only a
// type-name-shaped string as far as the schema is concerned (see
// schema/journal-line.schema.json), and which names are admissible is
// Profile's declaration. A nil Profile resolves to schema.DefaultProfile()
// — the caller's own resolved profile is what makes a profile.json's
// custom node types (or a dropped one) actually gate the write.
type JournalEncoder struct {
	// Profile is the resolved project profile whose declared node types
	// gate a change event's node_type. Nil defaults to
	// schema.DefaultProfile().
	Profile *schema.Profile
}

// NewJournalEncoder constructs a JournalEncoder that validates change
// events against the default profile. Callers holding a project's
// resolved profile (a profile.json may declare types the default does
// not) set Profile directly instead.
func NewJournalEncoder() *JournalEncoder {
	return &JournalEncoder{}
}

// resolvedProfile returns Profile, or schema.DefaultProfile() when unset.
func (e *JournalEncoder) resolvedProfile() *schema.Profile {
	if e.Profile != nil {
		return e.Profile
	}
	return schema.DefaultProfile()
}

// Encode renders one journal event or receipt as the wire JSON its
// event kind requires — field order the encoder's own business; callers
// parse, never byte-compare.
func (e *JournalEncoder) Encode(ev mapping.Event) ([]byte, error) {
	return encodeLine(ev)
}

// Validate encodes ev and validates the result against the journal-line
// schema, refusing the line before any write is attempted. The error
// names the violated constraint. A change event (added/modified/removed)
// that passes the schema is then checked against the resolved profile's
// declared node types — the schema fixes node_type's shape and
// enumerates nothing, so this membership check is Validate's own second
// gate, run only for the write path a caller drives through it.
func (e *JournalEncoder) Validate(ev mapping.Event) error {
	sch, err := getLineSchema()
	if err != nil {
		return err
	}
	raw, err := e.Encode(ev)
	if err != nil {
		return fmt.Errorf("ingest: journal encoder: encode %s line: %w", ev.Event, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("ingest: journal encoder: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("ingest: journal encoder: %s line: %w", ev.Event, err)
	}
	if err := e.checkNodeTypeDeclared(ev); err != nil {
		return err
	}
	return nil
}

// checkNodeTypeDeclared refuses a change event whose node_type names a
// kind the resolved profile does not declare — a profile-declared kind's
// event lands, a meta leaf, a retired kind or a misspelling never does.
// Every other journal-line kind carries no node_type and is exempt.
func (e *JournalEncoder) checkNodeTypeDeclared(ev mapping.Event) error {
	switch ev.Event {
	case "added", "modified", "removed":
	default:
		return nil
	}
	if nodeTypeDeclared(e.resolvedProfile(), ev.NodeType) {
		return nil
	}
	return fmt.Errorf("ingest: journal encoder: %s line: node_type %q is not declared by the resolved profile", ev.Event, ev.NodeType)
}

// nodeTypeDeclared reports whether profile declares a node type named
// name, in either scope — the encoder asks only whether the name exists
// at all, the same membership schema.Profile.Validate itself checks
// project- and module-scoped declarations against uniformly.
func nodeTypeDeclared(profile *schema.Profile, name string) bool {
	for _, t := range profile.NodeTypes {
		if t.Name == name {
			return true
		}
	}
	return false
}

// checkInvariant5 asserts that every line in the batch validates against
// the journal-line schema — and, for change events, the resolved
// profile's declared node types — before any of it is written; invariant
// 5, delegated line-by-line to JournalEncoder.Validate so Reconciler and
// RefreshHandler inherit the gate rather than re-implementing it. A nil
// profile validates against schema.DefaultProfile().
func checkInvariant5(batch []mapping.Event, profile *schema.Profile) error {
	enc := &JournalEncoder{Profile: profile}
	for _, ev := range batch {
		if err := enc.Validate(ev); err != nil {
			return fmt.Errorf("ingest: reconcile: invariant 5: %w", err)
		}
	}
	return nil
}

var (
	lineSchema     *jsonschema.Schema
	lineSchemaErr  error
	lineSchemaOnce sync.Once
)

// getLineSchema compiles the embedded journal-line schema once and caches
// it. JournalEncoder owns its own compiled copy rather than reaching into
// mapping's — MappingStore's is a read-time internal, and JournalEncoder
// is the format's only writer. Reads schema.JournalLineSchema (backed by
// schema/journal-line.schema.json) rather than the retired
// schema.BeadMapSchema — the two documents are byte-identical apart from
// their $id, but MappingStore (the journal's only other reader) has not
// migrated off schema.BeadMapSchema yet (schema/schema.go), so
// JournalLineSchema is not yet the name every reader resolves by — only
// this one.
func getLineSchema() (*jsonschema.Schema, error) {
	lineSchemaOnce.Do(func() {
		raw, err := schema.JournalLineSchema()
		if err != nil {
			lineSchemaErr = fmt.Errorf("load journal-line schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			lineSchemaErr = fmt.Errorf("parse journal-line schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("journal-line.schema.json", doc); err != nil {
			lineSchemaErr = fmt.Errorf("add journal-line schema: %w", err)
			return
		}
		lineSchema, lineSchemaErr = c.Compile("journal-line.schema.json")
	})
	return lineSchema, lineSchemaErr
}

// changeEventLine, registeredEventLine, taskReceiptLine and
// taskRetargetedLine mirror the journal-line shapes in
// schema/journal-line.schema.json exactly — changeEventLine always serialises
// its ten required keys (before/after admit null); taskReceiptLine omits
// whichever of for/proposal does not apply, since additionalProperties is
// false on every shape. registeredEventLine has no writer in this
// package — the proposal Registrar appends it through MappingStore
// directly — but Reconciler's tests seed one to exercise the epic-create
// referent lookup, and JournalEncoder must be able to validate one if it
// ever appeared in a batch.
type changeEventLine struct {
	Event    string  `json:"event"`
	EID      string  `json:"eid"`
	Node     string  `json:"node"`
	Name     string  `json:"name"`
	NodeType string  `json:"node_type"`
	Module   string  `json:"module"`
	Before   *string `json:"before"`
	After    *string `json:"after"`
	GitHead  string  `json:"git_head"`
	Proposal string  `json:"proposal"`
	Path     string  `json:"path,omitempty"`
}

type taskReceiptLine struct {
	Event    string `json:"event"`
	TaskID   string `json:"task_id"`
	For      string `json:"for,omitempty"`
	Proposal string `json:"proposal,omitempty"`
}

// taskRetargetedLine mirrors the taskRetargetedReceipt journal-line shape
// exactly: for is always required (no legacy proposal-slug arm ever
// existed for this kind — see arch_reconciler.md "Retarget Ops") and no
// proposal field is admitted at all.
type taskRetargetedLine struct {
	Event  string `json:"event"`
	TaskID string `json:"task_id"`
	For    string `json:"for"`
}

type registeredEventLine struct {
	Event    string `json:"event"`
	EID      string `json:"eid"`
	Proposal string `json:"proposal"`
	GitHead  string `json:"git_head"`
}

// refreshReceiptLine mirrors the refreshReceipt journal-line shape exactly:
// git_head is nullable (a refresh run with no --git-head records the
// absence as JSON null, not empty string) and absorbed always serialises
// as an array, even when empty — RefreshHandler is this line kind's only
// writer, see arch_refresh.md.
type refreshReceiptLine struct {
	Event    string   `json:"event"`
	GitHead  *string  `json:"git_head"`
	Absorbed []string `json:"absorbed"`
}

// encodeLine renders one mapping.Event as the wire JSON its event kind
// requires. JournalEncoder.Encode is a thin wrapper over this function;
// checkInvariant5 reaches it via JournalEncoder.Validate rather than
// calling it directly, so Reconciler and RefreshHandler can never drift
// apart on wire shape.
func encodeLine(ev mapping.Event) ([]byte, error) {
	switch ev.Event {
	case "added", "modified", "removed":
		return json.Marshal(changeEventLine{
			Event: ev.Event, EID: ev.EID, Node: ev.Node, Name: ev.Name,
			NodeType: ev.NodeType, Module: ev.Module, Before: ev.Before, After: ev.After,
			GitHead: ev.GitHead, Proposal: ev.Proposal, Path: ev.Path,
		})
	case "registered":
		return json.Marshal(registeredEventLine{
			Event: ev.Event, EID: ev.EID, Proposal: ev.Proposal, GitHead: ev.GitHead,
		})
	case "task_created", "task_closed":
		return json.Marshal(taskReceiptLine{
			Event: ev.Event, TaskID: ev.TaskID, For: ev.For, Proposal: ev.Proposal,
		})
	case "task_retargeted":
		return json.Marshal(taskRetargetedLine{Event: ev.Event, TaskID: ev.TaskID, For: ev.For})
	case "refresh":
		var gitHead *string
		if ev.GitHead != "" {
			gitHead = strPtr(ev.GitHead)
		}
		absorbed := ev.Absorbed
		if absorbed == nil {
			absorbed = []string{}
		}
		return json.Marshal(refreshReceiptLine{Event: ev.Event, GitHead: gitHead, Absorbed: absorbed})
	default:
		return nil, fmt.Errorf("unknown journal line kind %q", ev.Event)
	}
}
