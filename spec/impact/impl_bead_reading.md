# Bead metadata reading

Implementation of BeadReader as a pure parser over tracker `list --json` output.

## Full Implementation

```go
package impact

import (
    "encoding/json"
    "fmt"
    "io"
    "strconv"
    "strings"
)

type BeadSpec struct {
    ID       string
    RecordID int
    Status   string
    Labels   []string
}

// trackerBead mirrors the input JSON shape.
type trackerBead struct {
    ID     string   `json:"id"`
    Status string   `json:"status"`
    Labels []string `json:"labels"`
}

type wrappedInput struct {
    Issues []trackerBead `json:"issues"`
}

func ReadBeads(r io.Reader) ([]BeadSpec, error) {
    data, err := io.ReadAll(r)
    if err != nil {
        return nil, fmt.Errorf("impact: read beads: %w", err)
    }
    return ReadBeadsBytes(data)
}

func ReadBeadsBytes(data []byte) ([]BeadSpec, error) {
    var beads []trackerBead

    // Try wrapped shape first: {"issues": [...]}.
    var wrap wrappedInput
    if err := json.Unmarshal(data, &wrap); err == nil && wrap.Issues != nil {
        beads = wrap.Issues
    } else {
        // Fall back to bare array.
        if err := json.Unmarshal(data, &beads); err != nil {
            return nil, fmt.Errorf("impact: read beads: parse: %w", err)
        }
    }

    out := make([]BeadSpec, 0, len(beads))
    for i, b := range beads {
        if b.ID == "" {
            return nil, fmt.Errorf("impact: read beads: missing bead id at index %d", i)
        }
        recID, ok := extractRecordID(b.Labels)
        if !ok {
            continue // not spec-managed
        }
        out = append(out, BeadSpec{
            ID:       b.ID,
            RecordID: recID,
            Status:   b.Status,
            Labels:   b.Labels,
        })
    }
    return out, nil
}

func extractRecordID(labels []string) (int, bool) {
    for _, lbl := range labels {
        if strings.HasPrefix(lbl, "spex:") {
            if n, err := strconv.Atoi(strings.TrimPrefix(lbl, "spex:")); err == nil {
                return n, true
            }
        }
    }
    return 0, false
}
```

## Test Rewrite Note

The legacy tests under `impact/bead_reader_test.go` that depended on a stub `br` binary (`testdata/mock-br`) are retired. New tests feed canned JSON fixtures directly:

```go
func TestReadBeads_WrappedShape(t *testing.T) {
    data := []byte(`{"issues":[{"id":"sm-1","status":"open","labels":["spex:42"]}]}`)
    got, err := ReadBeadsBytes(data)
    if err != nil { t.Fatal(err) }
    // assert got[0] == BeadSpec{ID:"sm-1", RecordID:42, Status:"open", Labels:["spex:42"]}
}
```

## Determinism

The output `[]BeadSpec` preserves the order of the input JSON array. Clients that need sorted output sort post-call.

## Performance

- One JSON parse per call. O(n) in the input byte length.
- Label-prefix scan is O(labels-per-bead) per bead; practically bounded.
- No allocations beyond the decoded slice and the output `BeadSpec` list.

For large trackers (10k+ beads), streaming parse would matter. The reference impl does an all-at-once ReadAll; real-world call sites are bounded by the count of spec-managed beads per proposal wave (dozens to low hundreds).
