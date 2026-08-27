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
	if err := c.checkInvariant1(existing, batch); err != nil {
		return err
	}
	if err := c.checkInvariant2(existing, batch); err != nil {
		return err
	}
	return nil
}

// checkInvariant1 asserts invariant 1 (requirement ee28b5d190ae,
// "The Five Invariants" #1 in test_consistency_invariants.md): every
// task_created line — across the existing journal and the batch under
// construction — pairs with exactly one referent: no two task_created
// lines may share a for-eid. The proposal-slug arm guards only legacy
// lines already on disk (see arch_reconciler.md "Proposal-Epic Ops") —
// new epic receipts always carry for, never proposal. It asserts the
// same one-referent property, in its own namespace, for
// task_retargeted — a retarget's modified event is never the referent
// of a task_created, so the two kinds are checked separately rather
// than sharing one seen-set. Missing-referent failures are caught
// earlier, during EventBuilder's construction (a cleanup with no
// matching removed event, an epic with no registered event, a
// close-removed with no journal entry for its bead) — this pass
// catches the aggregate double-pairing case construction cannot see
// one op at a time.
func (c *InvariantChecker) checkInvariant1(existing, batch []mapping.Event) error {
	seenFor := map[string]bool{}
	seenProposal := map[string]bool{}
	seenRetargetFor := map[string]bool{}
	check := func(ev mapping.Event) error {
		switch ev.Event {
		case "task_created":
			if ev.Proposal != "" {
				if seenProposal[ev.Proposal] {
					return fmt.Errorf("ingest: reconcile: invariant 1: proposal %s paired by more than one task_created", ev.Proposal)
				}
				seenProposal[ev.Proposal] = true
				return nil
			}
			if ev.For != "" {
				if seenFor[ev.For] {
					return fmt.Errorf("ingest: reconcile: invariant 1: event %s paired by more than one task_created", ev.For)
				}
				seenFor[ev.For] = true
			}
		case "task_retargeted":
			if ev.For != "" {
				if seenRetargetFor[ev.For] {
					return fmt.Errorf("ingest: reconcile: invariant 1: event %s paired by more than one task_retargeted", ev.For)
				}
				seenRetargetFor[ev.For] = true
			}
		}
		return nil
	}
	for _, ev := range existing {
		if err := check(ev); err != nil {
			return err
		}
	}
	for _, ev := range batch {
		if err := check(ev); err != nil {
			return err
		}
	}
	return nil
}

// checkInvariant2 asserts invariant 2 (requirement ee28b5d190ae,
// "The Five Invariants" #2 in test_consistency_invariants.md): every
// receipt's for-eid, in the batch under construction, names an event id
// present in the journal-plus-batch — task_created, task_closed and
// task_retargeted alike — and every eid a refresh receipt's absorbed
// list names does too.
func (c *InvariantChecker) checkInvariant2(existing, batch []mapping.Event) error {
	known := map[string]bool{}
	for _, ev := range existing {
		if ev.EID != "" {
			known[ev.EID] = true
		}
	}
	for _, ev := range batch {
		if ev.EID != "" {
			known[ev.EID] = true
		}
	}
	for _, ev := range batch {
		switch ev.Event {
		case "task_created", "task_closed", "task_retargeted":
			if ev.For != "" && !known[ev.For] {
				return fmt.Errorf("ingest: receipt references unknown event %s", ev.For)
			}
		case "refresh":
			for _, a := range ev.Absorbed {
				if !known[a] {
					return fmt.Errorf("ingest: receipt references unknown event %s", a)
				}
			}
		}
	}
	return nil
}
