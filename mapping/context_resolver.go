package mapping

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// ContextResult holds everything spex map context resolves for one node,
// keyed by identity hash or task id. Exactly one shape is populated: the
// live shape (paths into the current spec) for a node the spec still
// declares, or the removed shape (the journal's biography) for a node the
// journal remembers but the spec no longer carries. Removed distinguishes
// them. The bracket — Eid, Event, BeforeHead, AfterHead — accompanies both
// shapes: it locates the node's latest change in git, off the journal
// rather than the spec, so a consumer can run
// `git diff <before_head> <after_head> -- <leaves>` instead of
// reconstructing the delta by hand. A live node with no task-bearing event
// yet carries a null (empty) bracket.
type ContextResult struct {
	Removed bool `json:"removed,omitempty"`

	// Live shape — populated when Removed is false.
	ArchFile   string   `json:"arch_file,omitempty"`
	TestFiles  []string `json:"test_files,omitempty"`
	FlowFiles  []string `json:"flow_files,omitempty"`
	ModuleFile string   `json:"module_file,omitempty"`

	// Removed shape — populated when Removed is true.
	Name     string `json:"name,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	Module   string `json:"module,omitempty"`
	Proposal string `json:"proposal,omitempty"`
	TaskID   string `json:"task_id,omitempty"`

	// Bracket — populated for both shapes off the journal.
	Eid        string `json:"eid,omitempty"`
	Event      string `json:"event,omitempty"`
	BeforeHead string `json:"before_head,omitempty"`
	AfterHead  string `json:"after_head,omitempty"`
}

// ResolveContext resolves all spec files needed to implement or review a
// node, given one key — an identity hash or a task id — the spec
// directory, and the resolved journal path (the lifecycle pre-flight's
// answer; ResolveContext computes no location of its own). It is a pure
// function: deterministic, no side effects beyond reading files, no
// tracker contact.
func ResolveContext(specDir, journalPath, key string) (ContextResult, error) {
	store := NewMappingStore(journalPath)

	hash := key
	if !nodeHashPattern.MatchString(key) {
		entry, err := store.Get(key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ContextResult{}, fmt.Errorf("context: %w: task %s", ErrNotFound, key)
			}
			return ContextResult{}, fmt.Errorf("context: %w", err)
		}
		hash = entry.Key
	}

	projPath := filepath.Join(specDir, "project.json")
	projData, err := os.ReadFile(projPath)
	if err != nil {
		return ContextResult{}, fmt.Errorf("context: read %s: %w", projPath, err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return ContextResult{}, fmt.Errorf("context: parse %s: %w", projPath, err)
	}

	for _, mod := range proj.Modules {
		modPath := filepath.Join(specDir, mod.Path, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			return ContextResult{}, fmt.Errorf("context: read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return ContextResult{}, fmt.Errorf("context: parse %s: %w", modPath, err)
		}

		for _, c := range ms.Components {
			if c.ID == hash {
				return liveResult(store, specDir, mod.Path, modPath, hash, c.Content, ms)
			}
		}
		for _, df := range ms.DataFlows {
			if df.ID == hash {
				return liveResult(store, specDir, mod.Path, modPath, hash, df.Content, ms)
			}
		}
		for _, sec := range ms.TestSections {
			if sec.ID == hash {
				return liveResult(store, specDir, mod.Path, modPath, hash, sec.Content, ms)
			}
		}
	}

	return removedResult(store, hash)
}

// liveResult builds the live shape for the node identified by hash, whose
// own declared content is content, declared by module ms at mod.Path
// (relative to specDir), whose module.json lives at modPath. The bracket
// comes from the journal, off the node's latest task-bearing event; a node
// with no task-bearing event yet carries a null bracket, never an error —
// the file set never depends on the journal.
func liveResult(store *MappingStore, specDir, modRelPath, modPath, hash, content string, ms schema.ModuleSpec) (ContextResult, error) {
	modDir := filepath.Join(specDir, modRelPath)

	var testFiles []string
	for _, sec := range ms.TestSections {
		if slices.Contains(sec.Describes, hash) {
			testFiles = append(testFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var flowFiles []string
	for _, df := range ms.DataFlows {
		if slices.Contains(df.Uses, hash) {
			flowFiles = append(flowFiles, filepath.Join(modDir, df.Content))
		}
	}

	result := ContextResult{
		ArchFile:   filepath.Join(modDir, content),
		TestFiles:  testFiles,
		FlowFiles:  flowFiles,
		ModuleFile: modPath,
	}

	history, err := store.History(hash)
	if err != nil {
		return ContextResult{}, fmt.Errorf("context: %w", err)
	}
	if eid, event, before, after, ok := latestTaskBearingBracket(history); ok {
		result.Eid = eid
		result.Event = event
		result.BeforeHead = before
		result.AfterHead = after
	}
	return result, nil
}

// latestTaskBearingBracket scans a node's history for its latest
// task-bearing change event — one a task_created or task_retargeted receipt
// names via `for`, the two folding identically — and returns the bracket
// anchored on it: that event's own eid and kind, and its own git_head as
// AfterHead. ok is false when the node has no task-bearing event yet.
//
// When the latest task-bearing event was named by task_created, before is
// the git_head of the change event immediately preceding it in the node's
// lineage — empty for an added, or when none precedes. When it was named by
// task_retargeted, the bracket widens instead: before comes from the change
// event preceding the task's *original* task_created referent, so
// consecutive retargets keep extending one bracket rather than each
// shrinking it to its own increment.
func latestTaskBearingBracket(history []Event) (eid, event, before, after string, ok bool) {
	var changeEvents []Event
	receiptFor := map[string]Event{}
	origTaskCreated := map[string]Event{}
	for _, ev := range history {
		switch ev.Event {
		case "added", "modified", "removed":
			changeEvents = append(changeEvents, ev)
		case "task_created":
			if ev.For != "" {
				receiptFor[ev.For] = ev
				if _, exists := origTaskCreated[ev.TaskID]; !exists {
					origTaskCreated[ev.TaskID] = ev
				}
			}
		case "task_retargeted":
			if ev.For != "" {
				receiptFor[ev.For] = ev
			}
		}
	}

	idx := -1
	for i := len(changeEvents) - 1; i >= 0; i-- {
		if _, named := receiptFor[changeEvents[i].EID]; named {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", "", "", "", false
	}

	latest := changeEvents[idx]
	receipt := receiptFor[latest.EID]

	if receipt.Event == "task_retargeted" {
		if orig, ok := origTaskCreated[receipt.TaskID]; ok {
			for i, ce := range changeEvents {
				if ce.EID == orig.For {
					if i > 0 && ce.Event != "added" {
						before = changeEvents[i-1].GitHead
					}
					break
				}
			}
		}
	} else if idx > 0 && latest.Event != "added" {
		before = changeEvents[idx-1].GitHead
	}

	return latest.EID, latest.Event, before, latest.GitHead, true
}

// removedResult resolves the biography of a node the spec no longer
// declares live, from its journal history: name, node type, module, the
// removing proposal, its last task, and the git_head refs bracketing its
// final change. A hash with no removed event in its history — including
// one with no history at all — is not-found: unknown to both spec and
// journal.
func removedResult(store *MappingStore, hash string) (ContextResult, error) {
	history, err := store.History(hash)
	if err != nil {
		return ContextResult{}, fmt.Errorf("context: %w", err)
	}

	var changeEvents []Event
	for _, ev := range history {
		switch ev.Event {
		case "added", "modified", "removed":
			changeEvents = append(changeEvents, ev)
		}
	}

	removedIdx := -1
	for i := len(changeEvents) - 1; i >= 0; i-- {
		if changeEvents[i].Event == "removed" {
			removedIdx = i
			break
		}
	}
	if removedIdx == -1 {
		return ContextResult{}, fmt.Errorf("context: %w: %s unknown to both spec and journal", ErrNotFound, hash)
	}
	removedEv := changeEvents[removedIdx]

	var beforeHead string
	if removedIdx > 0 {
		beforeHead = changeEvents[removedIdx-1].GitHead
	}

	var taskID string
	for _, ev := range history {
		if ev.Event == "task_created" {
			taskID = ev.TaskID
		}
	}

	return ContextResult{
		Removed:    true,
		Name:       removedEv.Name,
		NodeType:   removedEv.NodeType,
		Module:     removedEv.Module,
		Proposal:   removedEv.Proposal,
		TaskID:     taskID,
		Eid:        removedEv.EID,
		Event:      removedEv.Event,
		BeforeHead: beforeHead,
		AfterHead:  removedEv.GitHead,
	}, nil
}
