package mapping

import (
	"encoding/json"
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
// them.
type ContextResult struct {
	Removed bool `json:"removed,omitempty"`

	// Live shape — populated when Removed is false.
	ArchFile   string   `json:"arch_file,omitempty"`
	TestFiles  []string `json:"test_files,omitempty"`
	FlowFiles  []string `json:"flow_files,omitempty"`
	ModuleFile string   `json:"module_file,omitempty"`

	// Removed shape — populated when Removed is true.
	Name       string `json:"name,omitempty"`
	NodeType   string `json:"node_type,omitempty"`
	Module     string `json:"module,omitempty"`
	Proposal   string `json:"proposal,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	BeforeHead string `json:"before_head,omitempty"`
	AfterHead  string `json:"after_head,omitempty"`
}

// ResolveContext resolves all spec files needed to implement or review a
// node, given one key — an identity hash or a task id — and the spec
// directory. It is a pure function: deterministic, no side effects beyond
// reading files, no tracker contact.
func ResolveContext(specDir, key string) (ContextResult, error) {
	store := NewMappingStore(specDir)

	hash := key
	if !nodeHashPattern.MatchString(key) {
		entry, err := store.Get(key)
		if err != nil {
			return ContextResult{}, fmt.Errorf("context: %w: task %s", ErrNotFound, key)
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
			if c.ID != hash {
				continue
			}
			return liveResult(specDir, mod.Path, modPath, c, ms), nil
		}
	}

	return removedResult(store, hash)
}

// liveResult builds the live shape for component c, declared by module ms
// at mod.Path (relative to specDir), whose module.json lives at modPath.
func liveResult(specDir, modRelPath, modPath string, c schema.Component, ms schema.ModuleSpec) ContextResult {
	modDir := filepath.Join(specDir, modRelPath)

	var testFiles []string
	for _, sec := range ms.TestSections {
		if slices.Contains(sec.Describes, c.ID) {
			testFiles = append(testFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var flowFiles []string
	for _, df := range ms.DataFlows {
		if slices.Contains(df.Uses, c.ID) {
			flowFiles = append(flowFiles, filepath.Join(modDir, df.Content))
		}
	}

	return ContextResult{
		ArchFile:   filepath.Join(modDir, c.Content),
		TestFiles:  testFiles,
		FlowFiles:  flowFiles,
		ModuleFile: modPath,
	}
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
		return ContextResult{}, err
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
		BeforeHead: beforeHead,
		AfterHead:  removedEv.GitHead,
	}, nil
}
