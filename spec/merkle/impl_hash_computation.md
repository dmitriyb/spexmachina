# Hash Computation Implementation

## Leaf Hashing

```go
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
```

Stream the file through SHA-256 rather than reading the entire file into memory. This handles arbitrarily large files efficiently.

## Interior Hashing

```go
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
```

Sort child hashes lexicographically before concatenation. This ensures the interior hash is independent of child discovery order.
