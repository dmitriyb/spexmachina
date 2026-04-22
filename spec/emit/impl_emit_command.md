# Emit command implementation

Implementation notes for `cmd/spex/emit.go`.

## Skeleton

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"

    "github.com/dmitriyb/spexmachina/cli"
    "github.com/dmitriyb/spexmachina/emit"
    "github.com/dmitriyb/spexmachina/impact"
    "github.com/dmitriyb/spexmachina/mapping"
    "github.com/dmitriyb/spexmachina/spec"
    "github.com/spf13/cobra"
)

func newEmitCmd() *cobra.Command {
    var proposal, gitHead, impactPath, outPath string
    cmd := &cobra.Command{
        Use:   "emit",
        Short: "Emit a tool-agnostic changeset from an impact report",
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := validateGitHead(gitHead); err != nil {
                return err
            }
            if proposal == "" {
                return fmt.Errorf("emit: --proposal is required")
            }

            reportReader, closeReader, err := openImpact(impactPath, os.Stdin)
            if err != nil { return err }
            defer closeReader()

            var report impact.Report
            if err := json.NewDecoder(reportReader).Decode(&report); err != nil {
                return fmt.Errorf("emit: decode impact: %w", err)
            }
            if len(report.Errors) > 0 {
                return fmt.Errorf("emit: impact report carries errors: %v", report.Errors)
            }

            store, err := mapping.Open("./.bead-map.json")
            if err != nil { return err }
            defer store.Close()

            graph, err := spec.Load("./spec")
            if err != nil { return err }

            builder := &emit.Builder{
                SpecGraph:    graph,
                MappingStore: store,
                GitHead:      gitHead,
                Proposal:     proposal,
            }
            cs, err := builder.Build(report)
            if err != nil { return err }

            return writeChangeset(cs, outPath, cmd.OutOrStdout())
        },
    }
    cmd.Flags().StringVar(&proposal, "proposal", "", "proposal ref (filename stem)")
    cmd.Flags().StringVar(&gitHead, "git-head", "", "git HEAD SHA")
    cmd.Flags().StringVar(&impactPath, "impact", "", "impact report path (default: stdin)")
    cmd.Flags().StringVar(&outPath, "out", "", "changeset output path (default: stdout)")
    _ = cmd.MarkFlagRequired("proposal")
    _ = cmd.MarkFlagRequired("git-head")
    return cmd
}
```

## Registration

Register via the `cli` module's subcommand framework in `cmd/spex/main.go`:

```go
root.AddCommand(newEmitCmd())
```

## Input Openers

```go
func openImpact(path string, stdin io.Reader) (io.Reader, func(), error) {
    if path == "" {
        return stdin, func() {}, nil
    }
    f, err := os.Open(path)
    if err != nil {
        return nil, nil, fmt.Errorf("emit: open impact: %w", err)
    }
    return f, func() { _ = f.Close() }, nil
}
```

## Output Writers

```go
func writeChangeset(cs emit.Changeset, outPath string, stdout io.Writer) error {
    enc := func(w io.Writer) error {
        e := json.NewEncoder(w)
        e.SetIndent("", "  ")
        e.SetEscapeHTML(false)
        return e.Encode(cs)
    }
    if outPath == "" {
        return enc(stdout)
    }
    tmp := outPath + ".tmp"
    f, err := os.Create(tmp)
    if err != nil { return fmt.Errorf("emit: create out: %w", err) }
    if err := enc(f); err != nil {
        _ = f.Close()
        _ = os.Remove(tmp)
        return err
    }
    if err := f.Close(); err != nil { return err }
    return os.Rename(tmp, outPath)
}
```

## Validation Helper

```go
var gitHeadRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func validateGitHead(s string) error {
    if !gitHeadRe.MatchString(s) {
        return fmt.Errorf("emit: --git-head must be a hex SHA (7-40 chars), got %q", s)
    }
    return nil
}
```

## Composability Notes

- `spex emit` never writes to `.bead-map.json` or `spec/.snapshot.json`. Those belong to `spex ingest`.
- Atomic `--out` write via temp-file + rename ensures partially-written changesets never appear on disk.

## Testing

See `test_emit_command.md` for the CLI surface tests, and `test_changeset_builder.md` for the core Build-function tests.
