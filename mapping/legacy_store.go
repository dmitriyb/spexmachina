package mapping

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed bead-map-envelope.schema.json
var envelopeSchemaFS embed.FS

// ErrRecordNotFound is returned when a record lookup finds no match.
var ErrRecordNotFound = errors.New("record not found")

// Record links a spec node ID to a bead ID with structured metadata.
type Record struct {
	ID          int    `json:"id"`
	SpecNodeID  string `json:"spec_node_id"`
	BeadID      string `json:"bead_id"`
	BeadType    string `json:"bead_type"`
	NodeType    string `json:"node_type,omitempty"`
	Module      string `json:"module"`
	Component   string `json:"component"`
	ContentFile string `json:"content_file"`
	SpecHash    string `json:"spec_hash"`
	BeadStatus  string `json:"bead_status,omitempty"`
}

// Store defines CRUD operations on mapping records.
type Store interface {
	Create(r Record) (int, error)
	Get(id int) (Record, error)
	GetByBead(beadID string) (Record, error)
	GetBySpecNode(specNodeID string) ([]Record, error)
	Update(id int, updates map[string]string) error
	Delete(id int) error
	List() ([]Record, error)
	// NextRecordID reads the persisted monotonic counter that the next
	// Create would assign. Used by emit's IdempotencyLabeler to reserve
	// spex:<id> labels without advancing the counter — emit is pure;
	// ingest commits the advance.
	NextRecordID() (int, error)
	// GetByProposalEpic returns the open proposal-epic record for the
	// given proposal ref, used by emit's Resolver to detect re-run cases
	// where the epic bead already exists. Closed epic records are
	// ignored. Returns ErrRecordNotFound if no open epic record matches.
	GetByProposalEpic(proposal string) (Record, error)
	// Replace atomically rewrites the full mapping state — both the
	// records list and the next-id counter — to disk. Used by ingest's
	// Reconciler to commit an in-memory working copy after invariants
	// have been asserted. Records are validated against the bead-map
	// schema before the rename; a schema violation aborts the write
	// and leaves the on-disk file unchanged.
	Replace(records []Record, nextID int) error
}

// mapFile is the on-disk JSON structure for .bead-map.json.
type mapFile struct {
	NextID  int      `json:"next_id"`
	Records []Record `json:"records"`
}

var (
	beadMapSchema    *jsonschema.Schema
	beadMapSchemaErr error
	beadMapOnce      sync.Once
)

// getBeadMapSchema compiles the embedded .bead-map.json envelope schema once
// and caches it. This schema is local to mapping (schema.JournalLineSchema
// validates one line of the journal, spec/.history.jsonl, not this envelope)
// and goes away with MappingStore's migration onto the journal
// (spexmachina-y0wc.19).
func getBeadMapSchema() (*jsonschema.Schema, error) {
	beadMapOnce.Do(func() {
		raw, err := envelopeSchemaFS.ReadFile("bead-map-envelope.schema.json")
		if err != nil {
			beadMapSchemaErr = fmt.Errorf("map: load bead-map schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			beadMapSchemaErr = fmt.Errorf("map: parse bead-map schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("bead-map-envelope.schema.json", doc); err != nil {
			beadMapSchemaErr = fmt.Errorf("map: add bead-map schema: %w", err)
			return
		}
		beadMapSchema, beadMapSchemaErr = c.Compile("bead-map-envelope.schema.json")
	})
	return beadMapSchema, beadMapSchemaErr
}

// fileStore implements Store backed by a JSON file.
type fileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore creates a Store that reads/writes the given .bead-map.json path.
func NewFileStore(path string) Store {
	return &fileStore{path: path}
}

func (s *fileStore) Create(r Record) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return 0, err
	}

	for _, existing := range data.Records {
		if existing.BeadID == r.BeadID {
			return 0, fmt.Errorf("map: duplicate bead_id %q", r.BeadID)
		}
		// Proposal epic records share spec_node_id across apply runs by design
		// (one new epic per run, all referencing the same proposal). Skip the
		// uniqueness check for those; other node types still enforce it.
		if r.NodeType != "proposal" && existing.SpecNodeID == r.SpecNodeID {
			return 0, fmt.Errorf("map: duplicate spec_node_id %q (record %d)", r.SpecNodeID, existing.ID)
		}
	}

	r.ID = data.NextID
	data.NextID++
	data.Records = append(data.Records, r)
	return r.ID, s.save(data)
}

func (s *fileStore) Get(id int) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Record{}, err
	}

	for _, r := range data.Records {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: %d", ErrRecordNotFound, id)
}

func (s *fileStore) GetByBead(beadID string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Record{}, err
	}

	for _, r := range data.Records {
		if r.BeadID == beadID {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: bead_id %q", ErrRecordNotFound, beadID)
}

func (s *fileStore) GetBySpecNode(specNodeID string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, err
	}

	var matches []Record
	for _, r := range data.Records {
		if r.SpecNodeID == specNodeID {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("map: %w: spec_node_id %q", ErrRecordNotFound, specNodeID)
	}
	return matches, nil
}

func (s *fileStore) Update(id int, updates map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return err
	}

	for i, r := range data.Records {
		if r.ID == id {
			if v, ok := updates["spec_hash"]; ok {
				data.Records[i].SpecHash = v
			}
			if v, ok := updates["bead_id"]; ok {
				for _, other := range data.Records {
					if other.ID != id && other.BeadID == v {
						return fmt.Errorf("map: duplicate bead_id %q (record %d)", v, other.ID)
					}
				}
				data.Records[i].BeadID = v
			}
			return s.save(data)
		}
	}
	return fmt.Errorf("map: %w: %d", ErrRecordNotFound, id)
}

func (s *fileStore) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return err
	}

	for i, r := range data.Records {
		if r.ID == id {
			data.Records = append(data.Records[:i], data.Records[i+1:]...)
			return s.save(data)
		}
	}
	return fmt.Errorf("map: %w: %d", ErrRecordNotFound, id)
}

func (s *fileStore) NextRecordID() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return 0, err
	}
	return data.NextID, nil
}

func (s *fileStore) GetByProposalEpic(proposal string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Record{}, err
	}

	// Multiple proposal epics can share a SpecNodeID (one per apply run).
	// Skip closed runs and return the highest-ID open record so re-runs see
	// the latest epic.
	var match Record
	var found bool
	for _, r := range data.Records {
		if r.NodeType != "proposal" || r.SpecNodeID != proposal {
			continue
		}
		if r.BeadStatus == "closed" {
			continue
		}
		if !found || r.ID > match.ID {
			match = r
			found = true
		}
	}
	if !found {
		return Record{}, fmt.Errorf("map: %w: proposal epic %q", ErrRecordNotFound, proposal)
	}
	return match, nil
}

func (s *fileStore) Replace(records []Record, nextID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Record, len(records))
	copy(out, records)
	data := &mapFile{
		NextID:  nextID,
		Records: out,
	}

	sch, err := getBeadMapSchema()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("map: marshal: %w", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("map: parse: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("map: schema validation: %w", err)
	}

	return s.save(data)
}

func (s *fileStore) List() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, err
	}

	result := make([]Record, len(data.Records))
	copy(result, data.Records)
	return result, nil
}

// load reads and parses the mapping file. Returns an empty mapFile if the file
// does not exist.
func (s *fileStore) load() (*mapFile, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &mapFile{NextID: 1, Records: []Record{}}, nil
		}
		return nil, fmt.Errorf("map: read %s: %w", s.path, err)
	}

	// Validate against bead-map JSON Schema before parsing.
	sch, err := getBeadMapSchema()
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("map: parse %s: %w", s.path, err)
	}
	if err := sch.Validate(doc); err != nil {
		return nil, fmt.Errorf("map: schema validation %s: %w", s.path, err)
	}

	var data mapFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("map: parse %s: %w", s.path, err)
	}
	if data.Records == nil {
		data.Records = []Record{}
	}
	return &data, nil
}

// save writes the mapping file atomically using write-rename.
// Records are sorted by ID for diff-friendly output.
func (s *fileStore) save(data *mapFile) error {
	sort.Slice(data.Records, func(i, j int) bool {
		return data.Records[i].ID < data.Records[j].ID
	})

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("map: marshal: %w", err)
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".bead-map-*.tmp")
	if err != nil {
		return fmt.Errorf("map: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("map: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("map: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("map: rename %s: %w", s.path, err)
	}
	return nil
}
