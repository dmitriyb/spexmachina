package apply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/dmitriyb/spexmachina/mapping"
)

// gitHEAD returns the current git HEAD commit hash.
func gitHEAD(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("apply: resolve HEAD: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// LabelObsoletes is the label phase of the two-phase obsolescence flow.
// It adds spex:obsolete and commit:<HEAD> labels to each bead via Update
// (keeping beads open), and deletes mapping records for removed nodes.
// Modified nodes' mapping records are left intact for BeadCreator to update.
// Failures are logged as warnings; the batch continues.
func LabelObsoletes(ctx context.Context, cli BeadCLI, store mapping.Store, actions []Action, logger *slog.Logger) error {
	if len(actions) == 0 {
		return nil
	}

	head, err := gitHEAD(ctx)
	if err != nil {
		return err
	}

	var errs []error

	for _, a := range actions {
		metadata := map[string]string{
			"spex":   "obsolete",
			"commit": head,
		}
		if err := cli.Update(ctx, a.BeadID, metadata); err != nil {
			logger.WarnContext(ctx, "label obsolete bead failed",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("label %s (%s/%s): %w", a.BeadID, a.Module, a.Node, err))
			continue
		}

		// Delete mapping record for removed nodes only.
		// Looked up by the action's SpecNodeID identity hash — the same key
		// the merkle diff, the impact report, and the mapping store all share.
		if a.ChangeType == "removed" {
			recs, err := store.GetBySpecNode(a.SpecNodeID)
			if err != nil || len(recs) == 0 {
				logger.WarnContext(ctx, "mapping record not found for removed node",
					"bead_id", a.BeadID,
					"spec_node_id", a.SpecNodeID,
					"error", err,
				)
				continue
			}
			if err := store.Delete(recs[0].ID); err != nil {
				logger.WarnContext(ctx, "delete mapping record failed",
					"bead_id", a.BeadID,
					"spec_node_id", a.SpecNodeID,
					"record_id", recs[0].ID,
					"error", err,
				)
				errs = append(errs, fmt.Errorf("delete mapping for %s: %w", a.BeadID, err))
			}
		}
	}

	return errors.Join(errs...)
}

// CloseBeads is the close phase of the two-phase obsolescence flow.
// It closes each bead that was previously labeled by LabelObsoletes.
// Before attempting close, it checks the bead's current status — beads
// that are already closed are skipped with a Warn log. Real close failures
// are logged at Error level. The batch continues even if individual closes
// fail. Returns an aggregated error of real failures only, or nil.
func CloseBeads(ctx context.Context, cli BeadCLI, actions []Action, logger *slog.Logger) error {
	if len(actions) == 0 {
		return nil
	}

	var errs []error

	for _, a := range actions {
		status, err := cli.Status(ctx, a.BeadID)
		if err != nil {
			logger.ErrorContext(ctx, "check bead status failed",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("close %s (%s/%s): %w", a.BeadID, a.Module, a.Node, err))
			continue
		}

		if status == "closed" {
			logger.WarnContext(ctx, "bead already closed, skipping",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
			)
			continue
		}

		if err := cli.Close(ctx, a.BeadID, nil); err != nil {
			logger.ErrorContext(ctx, "close bead failed",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("close %s (%s/%s): %w", a.BeadID, a.Module, a.Node, err))
			continue
		}

		logger.InfoContext(ctx, "bead closed",
			"bead_id", a.BeadID,
			"module", a.Module,
			"node", a.Node,
		)
	}

	return errors.Join(errs...)
}
