package mapping

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ErrNotFound is returned when a record lookup finds no match.
var ErrNotFound = errors.New("record not found")

// Record links a spec node ID to a bead ID with structured metadata.
type Record struct {
	ID          int    `json:"id"`
	SpecNodeID  string `json:"spec_node_id"`
	BeadID      string `json:"bead_id"`
	Module      string `json:"module"`
	Component   string `json:"component"`
	ContentFile string `json:"content_file"`
	SpecHash    string `json:"spec_hash"`
}

// Store defines CRUD operations on mapping records.
type Store interface {
	Create(r Record) (int, error)
	Get(id int) (Record, error)
	GetByBead(beadID string) (Record, error)
	GetBySpecNode(specNodeID string) (Record, error)
	Update(id int, updates map[string]string) error
	Delete(id int) error
	List() ([]Record, error)
}

// mapFile is the on-disk JSON structure for .bead-map.json.
type mapFile struct {
	NextID  int      `json:"next_id"`
	Records []Record `json:"records"`
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
		if existing.SpecNodeID == r.SpecNodeID {
			return 0, fmt.Errorf("map: duplicate spec_node_id %q", r.SpecNodeID)
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
	return Record{}, fmt.Errorf("map: %w: %d", ErrNotFound, id)
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
	return Record{}, fmt.Errorf("map: %w: bead_id %q", ErrNotFound, beadID)
}

func (s *fileStore) GetBySpecNode(specNodeID string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Record{}, err
	}

	for _, r := range data.Records {
		if r.SpecNodeID == specNodeID {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: spec_node_id %q", ErrNotFound, specNodeID)
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
			return s.save(data)
		}
	}
	return fmt.Errorf("map: %w: %d", ErrNotFound, id)
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
	return fmt.Errorf("map: %w: %d", ErrNotFound, id)
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
