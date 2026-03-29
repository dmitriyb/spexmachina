package proposal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// projectSections are the required H2 headings for a project proposal.
var projectSections = []string{"vision", "modules", "key requirements", "design decisions"}

// changeSections are the required H2 headings for a change proposal.
var changeSections = []string{"context", "proposed change", "impact expectation"}

// Register validates a proposal file and copies it to specDir/proposals/.
// It detects the proposal type from headings, checks required sections,
// generates a dated filename, and copies the file.
func Register(proposalPath, specDir string) (string, error) {
	content, err := os.ReadFile(proposalPath)
	if err != nil {
		return "", fmt.Errorf("proposal: read %s: %w", proposalPath, err)
	}

	ptype, err := detectType(string(content))
	if err != nil {
		return "", err
	}

	if err := validateSections(string(content), ptype); err != nil {
		return "", err
	}

	slug := slugFromHeading(string(content), proposalPath)
	filename := fmt.Sprintf("%s-%s.md", time.Now().Format("2006-01-02"), slug)

	proposalsDir := filepath.Join(specDir, "proposals")
	if err := os.MkdirAll(proposalsDir, 0755); err != nil {
		return "", fmt.Errorf("proposal: create proposals dir: %w", err)
	}

	destPath := filepath.Join(proposalsDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("proposal: already registered: %s", filename)
	}

	if err := copyFile(proposalPath, destPath); err != nil {
		return "", fmt.Errorf("proposal: copy to %s: %w", destPath, err)
	}

	return filename, nil
}

// detectType determines if a proposal is "project" or "change" by scanning H2 headings.
func detectType(content string) (string, error) {
	headings := extractH2Headings(content)
	for _, h := range headings {
		if strings.EqualFold(h, "vision") {
			return "project", nil
		}
	}
	for _, h := range headings {
		if strings.EqualFold(h, "proposed change") {
			return "change", nil
		}
	}
	return "", fmt.Errorf("proposal: cannot detect type from headings")
}

// validateSections checks that all required sections for the given type are present.
func validateSections(content, ptype string) error {
	headings := extractH2Headings(content)
	headingSet := make(map[string]bool, len(headings))
	for _, h := range headings {
		headingSet[strings.ToLower(h)] = true
	}

	var required []string
	switch ptype {
	case "project":
		required = projectSections
	case "change":
		required = changeSections
	}

	var errs []error
	for _, s := range required {
		if !headingSet[s] {
			errs = append(errs, fmt.Errorf("proposal: missing required section: %q", s))
		}
	}
	return errors.Join(errs...)
}

// extractH2Headings returns all H2 heading texts from markdown content.
func extractH2Headings(content string) []string {
	var headings []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimSpace(line[3:]))
		}
	}
	return headings
}

// slugFromHeading derives a URL-safe slug from the first H1 heading,
// or falls back to the base filename.
func slugFromHeading(content, filePath string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(line[2:])
			// Strip common prefixes like "Project Proposal: " or "Change Proposal: "
			if idx := strings.Index(heading, ": "); idx >= 0 {
				heading = heading[idx+2:]
			}
			return toSlug(heading)
		}
	}
	// Fallback: use filename without extension
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// toSlug converts a string to a URL-safe slug.
func toSlug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

// copyFile copies src to dst with permissions 0644.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
