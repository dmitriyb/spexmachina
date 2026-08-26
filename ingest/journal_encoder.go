package ingest

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/mapping"
)

// JournalEncoder turns a journal event or receipt into its serialized
// journal line and validates that line against the journal-line schema
// before it is written — invariant 5 of the mapping consistency
// invariants (requirement ee28b5d190ae, spec/ingest/module.json),
// expressed as a component rather than as helpers at the bottom of a
// larger file. A line that fails validation refuses the batch before
// the write path is reached; there is no partial append. Both
// Reconciler (normal mode) and RefreshHandler (refresh mode) encode
// through it, so neither pathway can drift from the other's wire shape.
// See spec/ingest/arch_journal_encoder.md.
//
// TODO(bead:spexmachina-ugrs.4): implement Encode and Validate. The
// working logic — the changeEventLine / taskReceiptLine /
// taskRetargetedLine / registeredEventLine / refreshReceiptLine wire
// shapes, encodeLine and checkInvariant5/getLineSchema — currently
// lives inline in ingest/reconciler.go, and RefreshHandler
// (ingest/refresh.go) calls checkInvariant5 directly as a
// package-private function pending extraction into this component by
// this bead.
type JournalEncoder struct{}

// NewJournalEncoder constructs a JournalEncoder.
func NewJournalEncoder() *JournalEncoder {
	return &JournalEncoder{}
}

// Encode renders one journal event or receipt as the wire JSON its
// event kind requires — field order the encoder's own business; callers
// parse, never byte-compare.
func (e *JournalEncoder) Encode(ev mapping.Event) ([]byte, error) {
	return nil, fmt.Errorf("ingest: JournalEncoder.Encode: not implemented (bead:spexmachina-ugrs.4)")
}

// Validate encodes ev and validates the result against the journal-line
// schema, refusing the line before any write is attempted. The error
// names the violated constraint.
func (e *JournalEncoder) Validate(ev mapping.Event) error {
	return fmt.Errorf("ingest: JournalEncoder.Validate: not implemented (bead:spexmachina-ugrs.4)")
}
