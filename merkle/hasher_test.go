package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestREQ1_HashFile_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello, merkle\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}

	if h1 != h2 {
		t.Fatalf("determinism: hash1=%s hash2=%s", h1, h2)
	}

	// Verify against known SHA-256
	s := sha256.Sum256(content)
	want := hex.EncodeToString(s[:])
	if h1 != want {
		t.Fatalf("want %s, got %s", want, h1)
	}
}

// TestS1_HashFile_MatchesIndependentSHA256 covers test_hashing.md S1: the
// hex string HashFile returns for a given file equals sha256Hex of that
// file's exact content, and is exactly 64 characters long.
func TestS1_HashFile_MatchesIndependentSHA256(t *testing.T) {
	dir := t.TempDir()
	content := "# Widget\nHandles widgets."
	path := filepath.Join(dir, "arch_widget.md")
	writeFile(t, dir, "arch_widget.md", content)

	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}

	s := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(s[:])
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
	if len(got) != 64 {
		t.Fatalf("hash length: want 64, got %d", len(got))
	}
}

// TestS2_HashFile_StreamsOrdinarySizeFile covers test_hashing.md S2: HashFile
// on a file of ordinary size returns a valid 64-character hex hash with no
// error, and that hash matches sha256Hex of the same content. HashFile
// streams via io.Copy (merkle/hasher.go:21); no test here exercises a large
// file, per the scenario's own note.
func TestS2_HashFile_StreamsOrdinarySizeFile(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("The quick brown fox jumps over the lazy dog.\n", 200)
	path := filepath.Join(dir, "ordinary.txt")
	writeFile(t, dir, "ordinary.txt", content)

	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("hash length: want 64, got %d", len(got))
	}

	s := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(s[:])
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestREQ1_HashFile_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	p2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(p1, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("beta"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := HashFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashFile(p2)
	if err != nil {
		t.Fatal(err)
	}

	if h1 == h2 {
		t.Fatal("different content should produce different hashes")
	}
}

func TestREQ1_HashFile_NonexistentFile(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("want error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "merkle: hash") {
		t.Fatalf("want wrapped error, got: %v", err)
	}
}

// TestE1_HashFile_NonexistentFile covers test_hashing.md E1: HashFile on a
// missing path returns an error wrapping the OS error, and the message
// contains the file path for debuggability.
func TestE1_HashFile_NonexistentFile(t *testing.T) {
	path := "/nonexistent/path/file.txt"
	_, err := HashFile(path)
	if err == nil {
		t.Fatal("want error for nonexistent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want error wrapping os.ErrNotExist, got: %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("want error message to contain path %q, got: %v", path, err)
	}
}

func TestREQ1_HashFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	h, err := HashFile(path)
	if err != nil {
		t.Fatalf("hash empty file: %v", err)
	}

	s := sha256.Sum256([]byte{})
	want := hex.EncodeToString(s[:])
	if h != want {
		t.Fatalf("want %s, got %s", want, h)
	}
}

func TestREQ6_HashChildren_OrderIndependent(t *testing.T) {
	hashes := []string{
		"abc123def456",
		"111222333444",
		"zzzyyyxxxwww",
	}
	reversed := []string{
		"zzzyyyxxxwww",
		"111222333444",
		"abc123def456",
	}

	h1 := HashChildren(hashes)
	h2 := HashChildren(reversed)

	if h1 != h2 {
		t.Fatalf("order independence: %s != %s", h1, h2)
	}
}

func TestREQ6_HashChildren_Deterministic(t *testing.T) {
	hashes := []string{"aaa", "bbb", "ccc"}

	h1 := HashChildren(hashes)
	h2 := HashChildren(hashes)

	if h1 != h2 {
		t.Fatalf("determinism: %s != %s", h1, h2)
	}

	// Verify against manual computation
	sorted := make([]string, len(hashes))
	copy(sorted, hashes)
	sort.Strings(sorted)
	s := sha256.New()
	for _, ch := range sorted {
		s.Write([]byte(ch))
	}
	want := hex.EncodeToString(s.Sum(nil))
	if h1 != want {
		t.Fatalf("want %s, got %s", want, h1)
	}
}

func TestREQ6_HashChildren_DoesNotMutateInput(t *testing.T) {
	hashes := []string{"ccc", "aaa", "bbb"}
	original := make([]string, len(hashes))
	copy(original, hashes)

	HashChildren(hashes)

	for i, h := range hashes {
		if h != original[i] {
			t.Fatalf("input mutated at index %d: want %s, got %s", i, original[i], h)
		}
	}
}

func TestREQ6_HashChildren_DifferentSets(t *testing.T) {
	h1 := HashChildren([]string{"aaa", "bbb"})
	h2 := HashChildren([]string{"aaa", "ccc"})

	if h1 == h2 {
		t.Fatal("different child sets should produce different hashes")
	}
}

// TestS4_HashChildren_SingleChild covers test_hashing.md S4: a single-element
// child hash slice is a boundary case where sorting is a no-op and the
// concatenation is trivial — the result must still equal sha256Hex of that
// one hash.
func TestS4_HashChildren_SingleChild(t *testing.T) {
	got := HashChildren([]string{"abcdef1234"})

	s := sha256.Sum256([]byte("abcdef1234"))
	want := hex.EncodeToString(s[:])
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

// TestS3_HashChildren_SortsBeforeConcatenation covers test_hashing.md S3:
// HashChildren("cccc", "aaaa", "bbbb") equals sha256Hex("aaaabbbbcccc") — the
// exact ascending-sorted concatenation, not merely order-independence.
// A consistently-wrong sort direction would still be order-independent (it
// would pass TestREQ6_HashChildren_OrderIndependent) but would fail this
// exact-value check.
func TestS3_HashChildren_SortsBeforeConcatenation(t *testing.T) {
	got := HashChildren([]string{"cccc", "aaaa", "bbbb"})

	s := sha256.Sum256([]byte("aaaabbbbcccc"))
	want := hex.EncodeToString(s[:])
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}

	gotAlreadySorted := HashChildren([]string{"aaaa", "bbbb", "cccc"})
	if got != gotAlreadySorted {
		t.Fatalf("sorted vs unsorted input mismatch: %s != %s", got, gotAlreadySorted)
	}
}

// TestREQ6_HashChildren_Empty also covers test_hashing.md E2: HashChildren on
// an empty slice returns sha256Hex("") — the degenerate case for a module
// with no content files.
func TestREQ6_HashChildren_Empty(t *testing.T) {
	h1 := HashChildren([]string{})
	h2 := HashChildren(nil)

	// Both should produce the same hash (hash of empty input)
	if h1 != h2 {
		t.Fatalf("empty vs nil: %s != %s", h1, h2)
	}

	// Should be hash of empty string
	s := sha256.Sum256([]byte{})
	want := hex.EncodeToString(s[:])
	if h1 != want {
		t.Fatalf("want %s, got %s", want, h1)
	}
}
