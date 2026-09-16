package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dmitriyb/spexmachina/author"
	"golang.org/x/term"
)

// authorRefusalError is a contract refusal from one of the nine AuthorCommands
// surfaces: ObligationReporter (or a worker's own guard) refused the change
// before anything was written. Its structured error document has already
// been printed to stdout by the time this is constructed; main.go reads
// ExitCode() to exit with author.ExitRefusal (2) and prints Error() as the
// one stderr line spec/author/arch_author_commands.md's "Exit codes and
// output" documents.
type authorRefusalError struct {
	cmdName string
	count   int
}

func (e *authorRefusalError) Error() string {
	return fmt.Sprintf("%s: refused (%d finding(s))", e.cmdName, e.count)
}

func (e *authorRefusalError) ExitCode() int { return author.ExitRefusal }

// printAuthorJSON writes v to w as one JSON document: compact when piped,
// indented two spaces on a terminal — the shape every writing command's
// report and every refusal's error document share
// (spec/author/arch_author_commands.md, "Exit codes and output"; the same
// rule author.ShowProfile already applies to `spex profile show`).
func printAuthorJSON(w io.Writer, v any, pretty bool) error {
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

// finishAuthorResult is the shared tail of every writing command's RunE: it
// turns a worker's three-outcome return (report, refusals, err) into the
// exit-code and stdout-shape contract every surface in this module shares
// (spec/author/flow_authoring.md, "On stdout"):
//
//   - err != nil: an input error, wrapped with cmdName and returned as-is —
//     main.go exits 1, nothing was printed.
//   - refusals non-empty: the error document goes to stdout, and an
//     authorRefusalError carries author.ExitRefusal (2) back to main.go.
//   - otherwise: report goes to stdout as the write report, exit 0.
//
// report is passed as any because the callers' report types differ
// (*author.WriteReport for seven of the nine surfaces, *author.MigrateReport
// for spex migrate) yet print exactly the same way.
func finishAuthorResult(cmdName string, report any, refusals []author.RefusalEntry, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", cmdName, err)
	}

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if len(refusals) > 0 {
		if encErr := printAuthorJSON(os.Stdout, refusals, isTTY); encErr != nil {
			return fmt.Errorf("%s: %w", cmdName, encErr)
		}
		return &authorRefusalError{cmdName: cmdName, count: len(refusals)}
	}

	if encErr := printAuthorJSON(os.Stdout, report, isTTY); encErr != nil {
		return fmt.Errorf("%s: %w", cmdName, encErr)
	}
	return nil
}

// parseFieldFlags turns a repeated --field key=value flag into the
// map[string]string NodeAddInput.Fields carries, one entry per declared
// field value (spec/author/flow_authoring.md, "Into a worker": "field
// values keyed by declared field name"). A malformed entry — no "=" — is a
// missing-or-malformed-flag input error (spec/author/arch_author_commands.md,
// "Exit codes and output"), never a refusal: nothing about the spec has
// been judged yet.
func parseFieldFlags(raw []string) (map[string]string, error) {
	fields := make(map[string]string, len(raw))
	for _, kv := range raw {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("--field must be key=value, got %q", kv)
		}
		fields[key] = val
	}
	return fields, nil
}
