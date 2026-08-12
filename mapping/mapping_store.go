package mapping

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dmitriyb/spexmachina/schema"
)

// ErrNotFound is returned when Get finds no fold entry for the given key.
var ErrNotFound = errors.New("map: not found")

// nodeHashPattern matches the identity-hash shape: 12 lowercase hex
// characters. Get uses it to distinguish a node key from a task id — the
// two are interchangeable ways to reach one node's fold entry.
var nodeHashPattern = regexp.MustCompile(`^[a-f0-9]{12}$`)

// Event is one line of the task journal (spec/.history.jsonl): a change
// event (added/removed/modified), a task receipt (task_created/task_closed)
// or a refresh receipt. Fields are populated according to the line's shape —
// see spec/map/arch_mapping_store.md for the full field table.
type Event struct {
	Event    string   `json:"event"`
	EID      string   `json:"eid,omitempty"`
	Node     string   `json:"node,omitempty"`
	Name     string   `json:"name,omitempty"`
	NodeType string   `json:"node_type,omitempty"`
	Module   string   `json:"module,omitempty"`
	Before   *string  `json:"before,omitempty"`
	After    *string  `json:"after,omitempty"`
	GitHead  string   `json:"git_head,omitempty"`
	Path     string   `json:"path,omitempty"`
	Proposal string   `json:"proposal,omitempty"`
	For      string   `json:"for,omitempty"`
	TaskID   string   `json:"task_id,omitempty"`
	Absorbed []string `json:"absorbed,omitempty"`

	// Line is the 1-based line number the event was parsed from. It is not
	// part of the on-disk shape; list output is ordered by it.
	Line int `json:"-"`
}

// ParseError names the journal line and the violation found there — either
// invalid JSON or a journal-line schema violation. The map query surface
// (spex map get/list) surfaces it as a hard failure naming the line;
// gating callers, like the diff removal sweep, type-assert for it and
// degrade to "journal absent" instead of failing, because the journal is
// never load-bearing for the pipeline.
type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("journal line %d: %v", e.Line, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// FoldEntry is one node's or proposal-epic's current linkage: the latest
// task-bearing event, or — once a node is removed — its biography.
type FoldEntry struct {
	// Key is the identity hash for a node entry, or the proposal slug for
	// a proposal-epic entry.
	Key string
	// TaskID is the current task id. Empty when Removed is true: a removed
	// node's fold entry carries its biography instead of a live task.
	TaskID string
	// Removed is true once the node's latest journal state is a removal.
	Removed bool
	// Source is the event that carries this entry's identity — the change
	// event for a live node (whose name/node_type/module belong to it), the
	// removing event for a removed node (whose proposal/git_head answer the
	// removal), or the task_created receipt itself for a proposal-epic
	// entry, which has no change event.
	Source Event
}

// DanglingReceipt is a task_created receipt whose `for` names an eid no
// change event carries. One bad pairing does not poison the rest of the
// journal — Fold reports it rather than failing.
type DanglingReceipt struct {
	Receipt Event
}

// Fold is the outcome of folding the journal: the current linkage per
// node/epic, plus any receipts that could not be paired to a change event.
type Fold struct {
	Entries  []FoldEntry
	Dangling []DanglingReceipt
}

// MappingStore provides access to the task journal
// (<spec-dir>/.history.jsonl), the append-only event log linking spec node
// identity hashes to tracker task ids. Parsing, scanning and folding it is
// most of the store: the fold — the latest task-bearing event per node —
// is the current linkage every consumer derives on demand, never a stored
// cache. The store is also the journal's one writer-owner: every append
// goes through Append, its atomic, schema-validating primitive.
type MappingStore struct {
	path string
	mu   sync.Mutex
}

// NewMappingStore returns a MappingStore reading the journal at
// <specDir>/.history.jsonl. The journal's location is a function of
// --spec-dir alone — there is no separate --map/--map-file flag.
func NewMappingStore(specDir string) *MappingStore {
	return &MappingStore{path: filepath.Join(specDir, ".history.jsonl")}
}

var (
	journalLineSchema     *jsonschema.Schema
	journalLineSchemaErr  error
	journalLineSchemaOnce sync.Once
)

// getJournalLineSchema compiles the embedded journal-line schema
// (schema.BeadMapSchema) once and caches it.
func getJournalLineSchema() (*jsonschema.Schema, error) {
	journalLineSchemaOnce.Do(func() {
		raw, err := schema.BeadMapSchema()
		if err != nil {
			journalLineSchemaErr = fmt.Errorf("load journal-line schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			journalLineSchemaErr = fmt.Errorf("parse journal-line schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("bead-map.schema.json", doc); err != nil {
			journalLineSchemaErr = fmt.Errorf("add journal-line schema: %w", err)
			return
		}
		journalLineSchema, journalLineSchemaErr = c.Compile("bead-map.schema.json")
	})
	return journalLineSchema, journalLineSchemaErr
}

// Parse reads and validates the journal, returning every event in file
// order. A missing or empty journal is a first-class state — it returns a
// nil slice and a nil error, not a failure. A line that is not valid JSON,
// or that violates the journal-line schema, yields a *ParseError naming
// the line number; parsing stops at that line, since a journal that fails
// to parse cleanly cannot be trusted to fold correctly past it.
func (s *MappingStore) Parse() ([]Event, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	sch, err := getJournalLineSchema()
	if err != nil {
		return nil, err
	}

	var events []Event
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(line))
		if err != nil {
			return nil, &ParseError{Line: lineNo, Err: fmt.Errorf("invalid JSON: %w", err)}
		}
		if err := sch.Validate(doc); err != nil {
			return nil, &ParseError{Line: lineNo, Err: err}
		}

		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &ParseError{Line: lineNo, Err: err}
		}
		ev.Line = lineNo
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	return events, nil
}

// fold walks events in file order and computes the current linkage per
// node: the latest task-bearing event, or, once a removed change event is
// seen, its biography. A registered event opens a proposal epic's
// lifecycle before any spec change exists; a task_created referencing it
// folds the epic keyed by the slug the registered event carries, sourced
// from that event. Legacy proposal-epic receipts (task_created carrying
// proposal instead of for) fold the same way without any change or
// registered event to reference. A task_created whose for names no known
// eid is reported as a dangling receipt rather than dropped or failed on.
func fold(events []Event) Fold {
	byEID := make(map[string]Event, len(events))
	entries := make(map[string]FoldEntry)
	var dangling []DanglingReceipt

	for _, ev := range events {
		switch ev.Event {
		case "added", "modified", "removed":
			byEID[ev.EID] = ev
			if ev.Event == "removed" {
				entries[ev.Node] = FoldEntry{Key: ev.Node, Removed: true, Source: ev}
			}
		case "registered":
			byEID[ev.EID] = ev
		case "task_created":
			if ev.Proposal != "" {
				entries[ev.Proposal] = FoldEntry{Key: ev.Proposal, TaskID: ev.TaskID, Source: ev}
				continue
			}
			referent, ok := byEID[ev.For]
			if !ok {
				dangling = append(dangling, DanglingReceipt{Receipt: ev})
				continue
			}
			if referent.Event == "registered" {
				entries[referent.Proposal] = FoldEntry{Key: referent.Proposal, TaskID: ev.TaskID, Source: referent}
				continue
			}
			entries[referent.Node] = FoldEntry{Key: referent.Node, TaskID: ev.TaskID, Removed: referent.Event == "removed", Source: referent}
		}
	}

	out := make([]FoldEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source.Line < out[j].Source.Line })
	return Fold{Entries: out, Dangling: dangling}
}

// List returns the folded current linkage — one entry per task-bearing
// node plus one per removed node, in journal file-position order. Backs
// `spex map list`.
func (s *MappingStore) List() (Fold, error) {
	events, err := s.Parse()
	if err != nil {
		return Fold{}, err
	}
	return fold(events), nil
}

// Get resolves one key — an identity hash or a task id — to its fold
// entry. The two keys are interchangeable ways to reach one node,
// distinguished by shape: a 12-character lowercase hex string is treated
// as a node identity hash and looked up directly; anything else is treated
// as a task id and matched against each entry's current TaskID. Backs
// `spex map get`.
func (s *MappingStore) Get(key string) (FoldEntry, error) {
	f, err := s.List()
	if err != nil {
		return FoldEntry{}, err
	}

	if nodeHashPattern.MatchString(key) {
		for _, e := range f.Entries {
			if e.Key == key {
				return e, nil
			}
		}
		return FoldEntry{}, fmt.Errorf("%w: node %s", ErrNotFound, key)
	}

	for _, e := range f.Entries {
		if e.TaskID == key {
			return e, nil
		}
	}
	return FoldEntry{}, fmt.Errorf("%w: task %s", ErrNotFound, key)
}

// History returns every event touching one node — the change events where
// it is the subject, plus the receipts paired to those events — oldest
// first. This is how lineage questions ("which tasks has this node had?")
// are answered without any stored back-pointers.
func (s *MappingStore) History(node string) ([]Event, error) {
	events, err := s.Parse()
	if err != nil {
		return nil, err
	}

	eids := map[string]bool{}
	var history []Event
	for _, ev := range events {
		switch ev.Event {
		case "added", "modified", "removed":
			if ev.Node == node {
				eids[ev.EID] = true
				history = append(history, ev)
			}
		case "task_created", "task_closed":
			if ev.For != "" && eids[ev.For] {
				history = append(history, ev)
			}
		}
	}
	return history, nil
}

// AppendError names the batch line whose event violates the journal-line
// schema. Line is 1-based and counts within the batch passed to Append,
// not within the file — Append validates every line before writing any of
// them, so a refused batch changes nothing on disk.
type AppendError struct {
	Line int
	Err  error
}

func (e *AppendError) Error() string {
	return fmt.Sprintf("append: batch line %d: %v", e.Line, e.Err)
}

func (e *AppendError) Unwrap() error {
	return e.Err
}

// changeEventLine, registeredEventLine, taskReceiptLine and
// refreshReceiptLine mirror the journal-line shapes in
// schema/bead-map.schema.json exactly, since additionalProperties is false
// on every shape: changeEventLine always serialises its ten required keys
// (before/after admit null), registeredEventLine carries no node because
// registration precedes any spec change, and taskReceiptLine omits
// whichever of for/proposal does not apply.
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

type registeredEventLine struct {
	Event    string `json:"event"`
	EID      string `json:"eid"`
	Proposal string `json:"proposal"`
	GitHead  string `json:"git_head"`
}

type taskReceiptLine struct {
	Event    string `json:"event"`
	TaskID   string `json:"task_id"`
	For      string `json:"for,omitempty"`
	Proposal string `json:"proposal,omitempty"`
}

// refreshReceiptLine mirrors the refreshReceipt journal-line shape
// exactly: git_head is nullable (a refresh run with no --git-head records
// the absence as JSON null, not empty string) and absorbed always
// serialises as an array, even when empty.
type refreshReceiptLine struct {
	Event    string   `json:"event"`
	GitHead  *string  `json:"git_head"`
	Absorbed []string `json:"absorbed"`
}

// encodeLine renders one Event as the wire JSON its event kind requires.
// Append uses it both to schema-validate a batch and to write it.
func encodeLine(ev Event) ([]byte, error) {
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
	case "refresh":
		var gitHead *string
		if ev.GitHead != "" {
			h := ev.GitHead
			gitHead = &h
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

// Append validates every event in the batch against the journal-line
// schema and, only if the whole batch passes, lands it in one atomic
// write-and-rename — existing bytes are preserved verbatim and the new
// lines appended after them. A batch that fails validation is refused in
// full: nothing is written, and the on-disk journal is left
// byte-identical to its pre-append state, naming the offending line via
// *AppendError. Append is the journal's one write path; its two callers
// are `spex ingest`, appending reconciliation and refresh batches at
// baselining, and the proposal Registrar, appending the registered event.
func (s *MappingStore) Append(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sch, err := getJournalLineSchema()
	if err != nil {
		return err
	}

	encoded := make([][]byte, len(events))
	for i, ev := range events {
		raw, err := encodeLine(ev)
		if err != nil {
			return &AppendError{Line: i + 1, Err: err}
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return &AppendError{Line: i + 1, Err: err}
		}
		if err := sch.Validate(doc); err != nil {
			return &AppendError{Line: i + 1, Err: err}
		}
		encoded[i] = raw
	}

	existing, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("append: read %s: %w", s.path, err)
		}
		existing = nil
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	for _, raw := range encoded {
		buf.Write(raw)
		buf.WriteByte('\n')
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("append: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("append: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("append: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("append: rename %s: %w", s.path, err)
	}
	return nil
}
