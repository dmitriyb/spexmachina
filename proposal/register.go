package proposal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/dmitriyb/spexmachina/mapping"
)

// projectSections are the required H2 headings for a project proposal.
var projectSections = []string{"vision", "modules", "key requirements", "design decisions"}

// changeSections are the required H2 headings for a change proposal.
var changeSections = []string{"context", "proposed change", "impact expectation"}

// Register validates a proposal file, appends the registered event that
// opens its lifecycle to the task journal, and copies it to
// specDir/proposals/. It detects the proposal type from headings, checks
// required sections, generates a dated filename, then — in that order —
// appends the journal event and copies the file: every refusal happens
// before either mark lands, so a refused proposal leaves neither. gitHead
// is caller-supplied; spex never calls git itself.
//
// The append is idempotent by eid (<gitHead>:<stem>, deterministic per
// proposal and head), so the one partial state a crash can leave — event
// appended, file not yet copied — is repaired by re-running: the
// already-registered check finds no file, the append finds its eid already
// present and adds nothing, and the copy lands.
func Register(proposalPath, specDir, gitHead string) (string, error) {
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

	filename := targetName(proposalPath, string(content))
	stem := strings.TrimSuffix(filename, ".md")

	proposalsDir := filepath.Join(specDir, "proposals")
	destPath := filepath.Join(proposalsDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("proposal: already registered: %s", filename)
	}

	store := mapping.NewMappingStore(specDir)
	eid := gitHead + ":" + stem
	already, err := registeredEventExists(store, eid)
	if err != nil {
		return "", fmt.Errorf("proposal: read journal: %w", err)
	}
	if !already {
		if err := store.Append([]mapping.Event{{
			Event:    "registered",
			EID:      eid,
			Proposal: stem,
			GitHead:  gitHead,
		}}); err != nil {
			return "", fmt.Errorf("proposal: append registered event: %w", err)
		}
	}

	if err := os.MkdirAll(proposalsDir, 0755); err != nil {
		return "", fmt.Errorf("proposal: create proposals dir: %w", err)
	}

	if err := copyFile(proposalPath, destPath); err != nil {
		return "", fmt.Errorf("proposal: copy to %s: %w", destPath, err)
	}

	return filename, nil
}

// registeredEventExists reports whether the journal already holds a
// registered event with the given eid — the idempotency check a recovery
// re-run relies on to avoid appending a duplicate line.
func registeredEventExists(store *mapping.MappingStore, eid string) (bool, error) {
	events, err := store.Parse()
	if err != nil {
		return false, err
	}
	for _, ev := range events {
		if ev.Event == "registered" && ev.EID == eid {
			return true, nil
		}
	}
	return false, nil
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
// It reads with bufio.Reader rather than bufio.Scanner: a proposal body
// paragraph can be arbitrarily long (no line-length limit is imposed on
// proposal content), and Scanner's fixed token buffer would abort the scan
// — and silently drop every heading past that point — the moment one line
// exceeds it.
func extractH2Headings(content string) []string {
	var headings []string
	reader := bufio.NewReader(strings.NewReader(content))
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimSpace(line[3:]))
		}
		if err != nil {
			break
		}
	}
	return headings
}

// datedName matches the YYYY-MM-DD-<name>.md convention a registered
// proposal is stored under.
var datedName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-.+\.md$`)

// targetName is the basename the copy is written under. A source already
// following the convention keeps its own name: re-dating it would file
// today's registration under a date the proposal was not written on, and
// break the spec_proposal:<stem> label every bead already carries. Anything
// else is renamed to today's date plus a slug from its first heading.
func targetName(proposalPath, content string) string {
	base := filepath.Base(proposalPath)
	if datedName.MatchString(base) {
		return base
	}
	return fmt.Sprintf("%s-%s.md", time.Now().Format("2006-01-02"), slugFromHeading(content, proposalPath))
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
