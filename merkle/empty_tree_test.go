package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestEmptyTree_Shape pins the empty-tree contract from
// spec/merkle/flow_hash_computation.md ("SnapshotStore.Load missing-file
// contract"): the root node has no children, type "project", and the hash
// equals SHA-256 of the empty string.
func TestEmptyTree_Shape(t *testing.T) {
	tree := EmptyTree()

	if tree == nil {
		t.Fatal("EmptyTree returned nil")
	}
	if tree.Type != "project" {
		t.Errorf("Type = %q, want %q", tree.Type, "project")
	}
	if len(tree.Children) != 0 {
		t.Errorf("Children = %d entries, want 0 (empty baseline)", len(tree.Children))
	}
}

// TestEmptyTree_HashIsSHA256OfEmptyString locks the wire-level contract:
// the empty-tree root hash equals SHA-256("") so that diffing a current
// tree against the empty baseline reports every leaf as "added" with
// stable, well-known old_hash values.
func TestEmptyTree_HashIsSHA256OfEmptyString(t *testing.T) {
	want := sha256.Sum256(nil)
	wantHex := hex.EncodeToString(want[:])

	got := EmptyTree().Hash
	if got != wantHex {
		t.Errorf("Hash = %q, want SHA-256(\"\") = %q", got, wantHex)
	}
}

// TestEmptyTree_Deterministic checks that successive calls produce
// byte-identical trees. The contract is keyed on a canonical empty
// baseline, so any non-determinism here would corrupt the diff output
// for first-run bootstrap.
func TestEmptyTree_Deterministic(t *testing.T) {
	a := EmptyTree()
	b := EmptyTree()

	if a.Hash != b.Hash {
		t.Errorf("Hash mismatch: a=%q b=%q", a.Hash, b.Hash)
	}
	if a.Key != b.Key {
		t.Errorf("Key mismatch: a=%q b=%q", a.Key, b.Key)
	}
	if a.Type != b.Type {
		t.Errorf("Type mismatch: a=%q b=%q", a.Type, b.Type)
	}
}

// TestEmptyTree_DiffsAsAllAdded verifies the operational contract: a
// non-empty current tree compared against the empty baseline reports
// every leaf as "added". This is what enables bootstrap inside spex
// diff without a pre-seeded snapshot.
func TestEmptyTree_DiffsAsAllAdded(t *testing.T) {
	leaf := &Node{
		Key:      "abc123def456",
		Hash:     "deadbeef",
		Type:     "leaf",
		NodeType: "component",
		Module:   "modhash",
	}
	current := &Node{
		Key:      "project",
		Hash:     HashChildren([]string{leaf.Hash}),
		Type:     "project",
		Children: []*Node{leaf},
	}

	changes := Diff(current, EmptyTree())

	var added int
	for _, c := range changes {
		if c.Type == Added {
			added++
		}
		if c.Type == Modified || c.Type == Removed {
			t.Errorf("unexpected non-added change against empty baseline: %+v", c)
		}
	}
	if added == 0 {
		t.Fatal("Diff against empty baseline produced no added changes; bootstrap path is broken")
	}
}

// TestEmptyTree_HashMatchesHashChildrenOfEmptySlice ties the empty-tree
// hash to the existing HashChildren primitive: an interior node with no
// children hashes to HashChildren(nil), which itself is SHA-256 of the
// empty byte stream.
func TestEmptyTree_HashMatchesHashChildrenOfEmptySlice(t *testing.T) {
	want := HashChildren(nil)
	got := EmptyTree().Hash
	if got != want {
		t.Errorf("EmptyTree().Hash = %q, want HashChildren(nil) = %q", got, want)
	}
}
