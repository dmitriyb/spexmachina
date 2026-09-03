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
type JournalEncoder struct{}

// NewJournalEncoder constructs a JournalEncoder.
func NewJournalEncoder() *JournalEncoder {
	return &JournalEncoder{}
}

// Encode renders one journal event or receipt as the wire JSON its
// event kind requires — field order the encoder's own business; callers
// parse, never byte-compare.
func (e *JournalEncoder) Encode(ev mapping.Event) ([]byte, error) {
	return encodeLine(ev)
}

// Validate encodes ev and validates the result against the journal-line
// schema, refusing the line before any write is attempted. The error
// names the violated constraint.
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
	return nil
}

// checkInvariant5 asserts that every line in the batch validates against
// the journal-line schema before any of it is written — invariant 5,
// delegated line-by-line to JournalEncoder.Validate so Reconciler and
// RefreshHandler inherit the gate rather than re-implementing it.
func checkInvariant5(batch []mapping.Event) error {
	enc := NewJournalEncoder()
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
// their $id, but JournalLineSchema is the current name every other
// reader of this format resolves by.
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
