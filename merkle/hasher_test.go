package merkle

import (
	"crypto/sha256"
	"encoding/hex"
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
