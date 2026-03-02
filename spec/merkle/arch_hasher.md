# Hasher

Computes cryptographic hashes for merkle tree nodes.

## Responsibilities

- Compute SHA-256 hash of a file's contents (leaf hash)
- Compute SHA-256 hash of a sorted list of child hashes (interior hash)
- Return hashes as hex-encoded strings

## Interface

```go
// HashFile computes the SHA-256 hash of a file's contents.
func HashFile(path string) (string, error)

// HashChildren computes the SHA-256 hash of sorted child hashes.
// Children are sorted lexicographically before concatenation.
func HashChildren(childHashes []string) string
```

## Design Rationale

### SHA-256

SHA-256 is used for its collision resistance and stdlib availability (`crypto/sha256`). The hash output is 64 hex characters, compact enough for snapshot files and git diffs.

### Sorted children

Interior node hashes are computed from sorted child hashes. This ensures determinism — the hash is independent of the order children are discovered during directory traversal. Two spec directories with identical content always produce identical trees.

### Hex encoding

Hashes are stored as hex strings, not raw bytes. This makes snapshot files human-readable and diff-friendly in git.
