package ingest

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/mapping"
)

// InvariantChecker asserts the journal consistency invariants
// (requirement ee28b5d190ae, spec/ingest/module.json) over the existing
// journal plus the constructed batch, after the whole batch exists and
// before anything reaches disk. Reconciler runs it as the last step
// before the commit; a failure refuses the run with the on-disk journal
// untouched. See spec/ingest/arch_invariant_checker.md.
//
// Checks run in numeric order, so the first message a caller sees names
// the most upstream cause:
//
//  1. every ok create pairs exactly one task_created with exactly one
//     referent event; every ok retarget pairs exactly one
//     task_retargeted with its own modified event; the batch's absorbed
//     events are closed by exactly one refresh receipt naming them.
//  2. no receipt references an eid neither the journal nor the batch
//     contains — for fields and a refresh receipt's absorbed entries
//     alike.
//  3. the batch minus already-present lines is what lands — enforced by
//     construction in EventBuilder's eid predicate; no check here.
//  4. snapshot saved iff receipts are complete — SnapshotSaver's gate;
//     no check here.
//  5. every line validates against the journal-line schema —
//     JournalEncoder's gate; no check here.
//
// The numbering is contractual, not incidental: it traces to the five
// numbered invariants the requirement states and to the test section
// titled for them, so checks 3, 4 and 5 stay named here even though
// their enforcement lives elsewhere.
//
// TODO(bead:spexmachina-ugrs.3): implement checks 1 and 2 as this
// component's own methods, each carrying a doc comment naming the
// invariant it enforces and the spec section it traces to. The working
// logic currently lives inline as Reconciler's checkInvariant1 /
// checkInvariant2 package functions (ingest/reconciler.go), pending
// extraction into this component by this bead.
type InvariantChecker struct{}

// NewInvariantChecker constructs an InvariantChecker.
func NewInvariantChecker() *InvariantChecker {
	return &InvariantChecker{}
}

// Check runs every invariant this component owns (1 and 2) over existing
// (the journal as parsed at the start of the run) plus batch (the lines
// EventBuilder constructed), in numeric order, and returns the first
// failure. No candidate batch is written on any failure.
func (c *InvariantChecker) Check(existing, batch []mapping.Event) error {
	return fmt.Errorf("ingest: InvariantChecker.Check: not implemented (bead:spexmachina-ugrs.3)")
}
