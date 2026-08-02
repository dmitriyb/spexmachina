package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

// gitHeadRe enforces the --git-head pre-flight: 7-40 hex chars, lowercase.
var gitHeadRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func newEmitCmd() *cobra.Command {
	var proposal, gitHead, impactPath, outPath, mapPath string

	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Emit a tool-agnostic changeset from an impact report",
		Long: `Emit reads an impact report, the bead mapping store, and the spec
graph, then writes a deterministic changeset.json (v1) describing the
ordered create/close/label/tag operations an external adapter must apply.

Inputs:
  --impact <file>   impact report JSON (default: stdin)
  --proposal <ref>  proposal filename stem (e.g. 2026-04-18-decouple-spex-from-br)
  --git-head <sha>  git HEAD SHA (caller-supplied, 7-40 hex chars)

Outputs:
  --out <file>      changeset JSON (default: stdout)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if proposal == "" {
				return validationErr(fmt.Errorf("emit: --proposal is required"))
			}
			if err := validateGitHead(gitHead); err != nil {
				return validationErr(err)
			}

			specDir, err := resolveSpecDir(cmd)
			if err != nil {
				return validationErr(err)
			}

			report, err := readImpactReport(impactPath, cmd.InOrStdin())
			if err != nil {
				return validationErr(err)
			}

			store := mapping.NewFileStore(resolveMapPath(mapPath, specDir))

			specGraph, err := newEmitSpecGraph(specDir)
			if err != nil {
				return validationErr(fmt.Errorf("emit: load spec graph: %w", err))
			}

			builder := &emit.Builder{
				SpecGraph:    specGraph,
				MappingStore: store,
				GitHead:      gitHead,
				Proposal:     proposal,
			}
			cs, err := builder.Build(report)
			if err != nil {
				return builderErr(err)
			}

			return writeChangeset(cs, outPath, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&proposal, "proposal", "", "proposal ref (filename stem)")
	cmd.Flags().StringVar(&gitHead, "git-head", "", "git HEAD SHA (7-40 hex chars)")
	cmd.Flags().StringVar(&impactPath, "impact", "", "impact report path (default: stdin)")
	cmd.Flags().StringVar(&outPath, "out", "", "changeset output path (default: stdout)")
	cmd.Flags().StringVar(&mapPath, "map", ".bead-map.json", "path to bead mapping file")
	_ = cmd.MarkFlagRequired("proposal")
	_ = cmd.MarkFlagRequired("git-head")
	return cmd
}

// validateGitHead applies the pre-flight regex from arch_emit_command.md.
func validateGitHead(s string) error {
	if !gitHeadRe.MatchString(s) {
		return fmt.Errorf("emit: --git-head must be a hex SHA (7-40 chars), got %q", s)
	}
	return nil
}

// resolveMapPath joins a relative --map path with the spec dir's parent
// (matching the impact command's convention) so callers can pass either
// an absolute path or a path relative to the project root.
func resolveMapPath(path, specDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(filepath.Dir(specDir), path)
}

// readImpactReport decodes the report from the named file or, when path is
// empty, from stdin. The wrapping struct also captures an optional `errors`
// array — emit double-checks the impact gate so a stale piped report cannot
// slip past, even though the impact command itself rejects diff errors
// upstream.
func readImpactReport(path string, stdin io.Reader) (impact.ImpactReport, error) {
	src, closer, err := openImpactSource(path, stdin)
	if err != nil {
		return impact.ImpactReport{}, err
	}
	defer closer()

	var raw struct {
		impact.ImpactReport
		Errors []json.RawMessage `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(src).Decode(&raw); err != nil {
		return impact.ImpactReport{}, fmt.Errorf("emit: decode impact: %w", err)
	}
	if len(raw.Errors) > 0 {
		return impact.ImpactReport{}, fmt.Errorf("emit: impact report carries %d errors; refusing to proceed", len(raw.Errors))
	}
	return raw.ImpactReport, nil
}

// openImpactSource opens the named file or returns stdin. The caller is
// responsible for invoking the returned closer; for stdin it is a no-op.
func openImpactSource(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "" {
		return stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("emit: open impact: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

// writeChangeset serializes cs as canonical 2-space-indented JSON. With an
// empty outPath the encoded form goes to stdout. Otherwise a temp file +
// rename ensures partial writes never leave a half-written changeset on
// disk if encoding fails mid-stream.
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
	if err != nil {
		return fmt.Errorf("emit: create out: %w", err)
	}
	if err := enc(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("emit: encode changeset: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("emit: close out: %w", err)
	}
	return os.Rename(tmp, outPath)
}

// emitError carries a process exit code alongside the wrapped error. main
// inspects the ExitCode interface to honor the codes documented in
// arch_emit_command.md (1 for input/validation, 2 for builder errors).
type emitError struct {
	code int
	err  error
}

func (e *emitError) Error() string { return e.err.Error() }
func (e *emitError) Unwrap() error { return e.err }
func (e *emitError) ExitCode() int { return e.code }

func validationErr(err error) error { return &emitError{code: 1, err: err} }
func builderErr(err error) error    { return &emitError{code: 2, err: err} }

// emitSpecGraph adapts the on-disk project + module schema into the
// emit.SpecGraph interface (Resolver's priority chain). It is constructed
// once per command invocation and indexed by spec_node_id for O(1) reads.
type emitSpecGraph struct {
	components  map[string]emit.Component
	moduleReqs  map[string]emit.ModuleRequirement
	projectReqs map[string]emit.ProjectRequirement
	paths       map[string]emit.NodePaths
}

func newEmitSpecGraph(specDir string) (*emitSpecGraph, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}

	g := &emitSpecGraph{
		components:  map[string]emit.Component{},
		moduleReqs:  map[string]emit.ModuleRequirement{},
		projectReqs: map[string]emit.ProjectRequirement{},
		paths:       map[string]emit.NodePaths{},
	}
	for _, r := range proj.Requirements {
		g.projectReqs[r.ID] = emit.ProjectRequirement{Priority: r.Priority}
	}

	for _, mod := range proj.Modules {
		modPath := filepath.Join(specDir, mod.Path, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return nil, fmt.Errorf("parse %s: %w", modPath, err)
		}
		for _, mr := range ms.Requirements {
			g.moduleReqs[mr.ID] = emit.ModuleRequirement{PreqID: mr.PreqID}
		}
		// Index every content-bearing node's repo-relative paths for
		// create-op body links. The "spec/" prefix matches the form the
		// mapping store records in content_file.
		modFile := filepath.Join("spec", mod.Path, "module.json")
		addPath := func(id, content string) {
			if content == "" {
				return
			}
			g.paths[id] = emit.NodePaths{
				Content: filepath.Join("spec", mod.Path, content),
				Module:  modFile,
			}
		}
		for _, c := range ms.Components {
			g.components[c.ID] = emit.Component{Implements: c.Implements}
			addPath(c.ID, c.Content)
		}
		for _, f := range ms.DataFlows {
			addPath(f.ID, f.Content)
		}
		for _, ts := range ms.TestSections {
			addPath(ts.ID, ts.Content)
		}
	}
	return g, nil
}

func (g *emitSpecGraph) Component(id string) (emit.Component, bool) {
	c, ok := g.components[id]
	return c, ok
}

func (g *emitSpecGraph) ModuleRequirement(id string) (emit.ModuleRequirement, bool) {
	r, ok := g.moduleReqs[id]
	return r, ok
}

func (g *emitSpecGraph) ProjectRequirement(id string) (emit.ProjectRequirement, bool) {
	r, ok := g.projectReqs[id]
	return r, ok
}

func (g *emitSpecGraph) Paths(id string) (emit.NodePaths, bool) {
	p, ok := g.paths[id]
	return p, ok
}
