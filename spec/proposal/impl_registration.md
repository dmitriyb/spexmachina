# Registration Implementation

## Algorithm

1. Read the proposal file
2. Parse markdown headings to detect proposal type
3. Check that all required sections for the detected type are present
4. Generate the target filename: `YYYY-MM-DD-<slug>.md`
5. Copy the file to `spec/proposals/<filename>`

## Section Detection

Scan for `## ` prefixed lines (H2 headings). Compare against required sections for each proposal type. Matching is case-insensitive.

```go
func detectType(content string) (string, error) {
    headings := extractH2Headings(content)
    if contains(headings, "vision") {
        return "project", nil
    }
    if contains(headings, "proposed change") {
        return "change", nil
    }
    return "", fmt.Errorf("proposal: cannot detect type from headings")
}
```

## Error Reporting

If required sections are missing, report all missing sections (not just the first). Use `errors.Join` for multi-error aggregation.

## File Copy

Use `io.Copy` from source to destination. Set file permissions to 0644 (readable, not executable).
