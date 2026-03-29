package proposal

import "context"

// StubBeadLister is a test double for BeadLister that returns fixed data.
// Exported so integration tests in cmd/spex can use it.
type StubBeadLister struct {
	Beads []BeadRecord
	Err   error
}

// ListBeads returns the stub's fixed beads or error.
func (s *StubBeadLister) ListBeads(_ context.Context) ([]BeadRecord, error) {
	return s.Beads, s.Err
}
