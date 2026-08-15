package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/ingest"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

func newIngestCmd() *cobra.Command {
	var changesetPath, receiptsPath, mode, gitHead string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Reconcile the task journal and save snapshot from a changeset+receipts pair",
		Long: `Ingest reads a changeset.json (produced by spex plan) and the
receipts.json an adapter wrote after applying it, reconciles the task
journal (spec/.history.jsonl), and writes spec/.snapshot.json when the
run is complete.

With --mode refresh (empty changeset + empty receipts), ingest instead
absorbs spec drift: one change event per drifted or absorbable
added/removed leaf is appended to the task journal, closed by a refresh
receipt, and the snapshot is rewritten — atomically, with no bead
lifecycle. Added/removed leaves are refused unless their node type is
in the refresh absorbable set — requirement and api in both directions,
component in the removed direction only — and a removed node with a
still-open task is refused regardless of its type.

Inputs:
  --changeset <file>   changeset JSON (required)
  --receipts <file>    receipts JSON (required)
  --mode <mode>        normal (default) or refresh
  --git-head <sha>     refresh mode only: commit stamped on the refresh
                        receipt. Normal mode ignores it — the changeset
                        already carries its own git_head.

Exit codes:
  0 — success (complete OR partial with no reconciler errors)
  1 — input error (bad flags, malformed JSON, op_id mismatch, IO failure,
      missing pre-refresh snapshot, non-empty refresh artifacts)
  2 — invariant failure (journal unchanged on disk) or refresh refusal
      (non-absorbable added/removed entries, a live task pairing on a
      removed node)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			specDir, err := resolveSpecDir(cmd)
			if err != nil {
				return ingestInputErr(err)
			}

			cs, err := loadChangeset(changesetPath)
			if err != nil {
				return ingestInputErr(err)
			}
			rc, err := loadReceipts(receiptsPath)
			if err != nil {
				return ingestInputErr(err)
			}
			if err := preflightPair(cs, rc); err != nil {
				return ingestInputErr(err)
			}

			if mode == "refresh" {
				return runRefreshMode(cmd, specDir, cs, rc, gitHead)
			}
			if mode != "normal" {
				return ingestInputErr(fmt.Errorf("ingest: --mode must be normal or refresh, got %q", mode))
			}

			graph, err := newIngestSpecGraph(specDir)
			if err != nil {
				return ingestInputErr(fmt.Errorf("ingest: load spec graph: %w", err))
			}

			reconciler := &ingest.Reconciler{SpecDir: specDir, SpecGraph: graph}
			sum, err := reconciler.Apply(cs, rc)
			if err != nil {
				return ingestInvariantErr(err)
			}

			saver := &ingest.Saver{SpecDir: specDir}
			wrote, err := saver.Save(rc.Status)
			if err != nil {
				return ingestInputErr(fmt.Errorf("ingest: snapshot: %w", err))
			}

			final := ingest.Summary{
				Ok:               sum.OkCreates + sum.OkCloses,
				Skipped:          sum.Skipped,
				Errors:           sum.Errors,
				EventsAppended:   sum.EventsAppended,
				ReceiptsAppended: sum.ReceiptsAppended,
				SnapshotSaved:    wrote,
				Status:           rc.Status,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(final)
		},
	}

	cmd.Flags().StringVar(&changesetPath, "changeset", "", "path to changeset.json")
	cmd.Flags().StringVar(&receiptsPath, "receipts", "", "path to receipts.json")
	cmd.Flags().StringVar(&mode, "mode", "normal", "run mode: normal or refresh")
	cmd.Flags().StringVar(&gitHead, "git-head", "", "refresh mode only: commit stamped on the refresh receipt")
	_ = cmd.MarkFlagRequired("changeset")
	_ = cmd.MarkFlagRequired("receipts")
	return cmd
}

// runRefreshMode dispatches to the RefreshHandler pathway. Refusals
// (structural diff entries, a live task pairing on a removed node) map
// to the invariant exit code 2; pre-flight failures (missing snapshot,
// non-empty artifacts) and IO errors map to input-error exit code 1,
// per arch_ingest_command.md. An empty gitHead records the refresh
// receipt's git_head as absent, per RefreshHandler's GitHead contract.
func runRefreshMode(cmd *cobra.Command, specDir string, cs plan.Changeset, rc adapters.Receipts, gitHead string) error {
	h := &ingest.RefreshHandler{
		Changeset: &cs,
		Receipts:  &rc,
	}
	if gitHead != "" {
		h.GitHead = &gitHead
	}
	sum, err := h.Apply(specDir)
	if err != nil {
		var refusal *ingest.RefreshRefusal
		if errors.As(err, &refusal) {
			return ingestInvariantErr(err)
		}
		return ingestInputErr(err)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(sum)
}

// loadChangeset parses changeset.json as plan.Changeset (v3) — the same
// type Reconciler and RefreshHandler consume, so no bridging conversion
// sits between the file and either pathway.
func loadChangeset(path string) (plan.Changeset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plan.Changeset{}, fmt.Errorf("ingest: read changeset: %w", err)
	}
	var cs plan.Changeset
	if err := json.Unmarshal(data, &cs); err != nil {
		return plan.Changeset{}, fmt.Errorf("ingest: parse changeset: %w", err)
	}
	return cs, nil
}

func loadReceipts(path string) (adapters.Receipts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return adapters.Receipts{}, fmt.Errorf("ingest: read receipts: %w", err)
	}
	var rc adapters.Receipts
	if err := json.Unmarshal(data, &rc); err != nil {
		return adapters.Receipts{}, fmt.Errorf("ingest: parse receipts: %w", err)
	}
	return rc, nil
}

// preflightPair validates the version envelope on each artifact and
// asserts that changeset and receipts cover exactly the same op_id set.
// Either side missing an op is a contract violation by plan or the
// adapter — input error, not invariant failure.
func preflightPair(cs plan.Changeset, rc adapters.Receipts) error {
	if cs.Version != plan.ChangesetVersion {
		return fmt.Errorf("ingest: changeset version must be %d, got %d", plan.ChangesetVersion, cs.Version)
	}
	if rc.Version != adapters.ReceiptsVersion {
		return fmt.Errorf("ingest: receipts version must be %d, got %d", adapters.ReceiptsVersion, rc.Version)
	}

	csOps := make(map[string]bool, len(cs.Ops))
	for _, op := range cs.Ops {
		csOps[op.OpID] = true
	}
	for _, rop := range rc.Ops {
		if !csOps[rop.OpID] {
			return fmt.Errorf("ingest: receipt op_id %s not in changeset", rop.OpID)
		}
	}

	rcOps := make(map[string]bool, len(rc.Ops))
	for _, rop := range rc.Ops {
		rcOps[rop.OpID] = true
	}
	for _, op := range cs.Ops {
		if !rcOps[op.OpID] {
			return fmt.Errorf("ingest: no receipt for op %s", op.OpID)
		}
	}
	return nil
}

// ingestError carries a process exit code alongside the wrapped error.
// main inspects the ExitCode interface to honour the codes documented in
// arch_ingest_command.md (1 for input errors, 2 for invariant failures).
type ingestError struct {
	code int
	err  error
}

func (e *ingestError) Error() string { return e.err.Error() }
func (e *ingestError) Unwrap() error { return e.err }
func (e *ingestError) ExitCode() int { return e.code }

func ingestInputErr(err error) error     { return &ingestError{code: ingest.ExitInputError, err: err} }
func ingestInvariantErr(err error) error { return &ingestError{code: ingest.ExitInvariant, err: err} }

// ingestSpecGraph implements ingest.SpecGraph by reading project.json
// and each module.json under specDir, then computing the merkle leaf
// hash for every component / data_flow / test_section
// content file.
type ingestSpecGraph struct {
	nodes map[string]ingest.NodeMetadata
}

func newIngestSpecGraph(specDir string) (*ingestSpecGraph, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}

	g := &ingestSpecGraph{nodes: map[string]ingest.NodeMetadata{}}
	for _, mod := range proj.Modules {
		modDir := filepath.Join(specDir, mod.Path)
		modPath := filepath.Join(modDir, "module.json")
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
		if err := g.registerModule(mod, modDir, ms); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *ingestSpecGraph) registerModule(mod schema.Module, modDir string, ms schema.ModuleSpec) error {
	for _, c := range ms.Components {
		if err := g.registerLeaf(mod, modDir, c.ID, c.Name, c.Content, "component"); err != nil {
			return err
		}
	}
	for _, f := range ms.DataFlows {
		if err := g.registerLeaf(mod, modDir, f.ID, f.Name, f.Content, "data_flow"); err != nil {
			return err
		}
	}
	for _, t := range ms.TestSections {
		if err := g.registerLeaf(mod, modDir, t.ID, t.Name, t.Content, "test_section"); err != nil {
			return err
		}
	}
	return nil
}

func (g *ingestSpecGraph) registerLeaf(mod schema.Module, modDir, id, name, content, nodeType string) error {
	if id == "" {
		return nil
	}
	md := ingest.NodeMetadata{
		Module:    mod.Name,
		Component: name,
		NodeType:  nodeType,
	}
	if content != "" {
		md.ContentFile = filepath.Join("spec", mod.Path, content)
		hash, err := merkle.HashFile(filepath.Join(modDir, content))
		if err != nil {
			return fmt.Errorf("hash %s: %w", md.ContentFile, err)
		}
		md.SpecHash = hash
	}
	g.nodes[id] = md
	return nil
}

func (g *ingestSpecGraph) NodeMetadata(id string) (ingest.NodeMetadata, error) {
	md, ok := g.nodes[id]
	if !ok {
		return ingest.NodeMetadata{}, fmt.Errorf("no node %s", id)
	}
	return md, nil
}
