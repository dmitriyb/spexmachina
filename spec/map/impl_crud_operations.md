# CRUD Operations

## Create

```go
func (s *fileStore) Create(r Record) (int, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    data, err := s.load()
    if err != nil {
        return 0, err
    }

    // Uniqueness check — bead ID must be unique; spec node ID may repeat
    for _, existing := range data.Records {
        if existing.BeadID == r.BeadID {
            return 0, fmt.Errorf("map: duplicate bead_id %q", r.BeadID)
        }
    }

    r.ID = data.NextID
    data.NextID++
    data.Records = append(data.Records, r)
    return r.ID, s.save(data)
}
```

## Read

Lookup methods scan the records array. Three access patterns:

- **By ID**: `Get(id int)` — primary key lookup
- **By bead ID**: `GetByBead(beadID string)` — used by `spex check`
- **By spec node ID**: `GetBySpecNode(specNodeID string)` — used by impact analysis

`Get` and `GetByBead` return `(Record, error)` where error is a not-found sentinel if no match.
`GetBySpecNode` returns `([]Record, error)` since one spec node may have many beads.

## Update

```go
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
                data.Records[i].BeadID = v
            }
            return s.save(data)
        }
    }
    return fmt.Errorf("map: record not found: %d", id)
}
```

Both `spec_hash` and `bead_id` are updatable — `bead_id` changes when a bead is obsoleted and replaced by a new one. Other fields (`spec_node_id`, `bead_type`, `module`, `component`, `content_file`) are immutable after creation.

A `CreateOrUpdate` method combines create and update: if a record with the same `spec_node_id` exists, update its `bead_id` and `spec_hash`; otherwise create a new record.

## Delete

Remove the record from the array. Do NOT decrement `next_id` — deleted IDs are never reused.

## Atomic File Writes

All mutations follow the write-rename pattern:
1. Write to a temporary file in the same directory
2. `os.Rename` the temp file to `.bead-map.json`

This ensures the file is always in a valid state — a crash mid-write leaves the old file intact.

## Concurrency

A `sync.Mutex` serializes all read-modify-write operations within a single process. Cross-process safety is provided by the atomic file write — the worst case is a lost write, not corruption.
