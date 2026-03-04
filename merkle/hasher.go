package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
)

// HashFile computes the SHA-256 hash of a file by streaming its contents.
// Returns the hex-encoded hash string.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("merkle: hash %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("merkle: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashChildren computes a deterministic SHA-256 hash for an interior node
// by sorting child hashes lexicographically before concatenation.
// Inputs must be fixed-length hex hash strings (e.g. from HashFile).
func HashChildren(childHashes []string) string {
	sorted := make([]string, len(childHashes))
	copy(sorted, childHashes)
	sort.Strings(sorted)
	h := sha256.New()
	for _, ch := range sorted {
		h.Write([]byte(ch))
	}
	return hex.EncodeToString(h.Sum(nil))
}
