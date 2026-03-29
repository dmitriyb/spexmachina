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
func gitHEAD() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
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

	head, err := gitHEAD()
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
		if a.ChangeType == "removed" {
			rec, err := store.GetByBead(a.BeadID)
			if err != nil {
				logger.WarnContext(ctx, "mapping record not found for removed bead",
					"bead_id", a.BeadID,
					"error", err,
				)
				continue
			}
			if err := store.Delete(rec.ID); err != nil {
				logger.WarnContext(ctx, "delete mapping record failed",
					"bead_id", a.BeadID,
					"record_id", rec.ID,
					"error", err,
				)
				errs = append(errs, fmt.Errorf("delete mapping for %s: %w", a.BeadID, err))
			}
		}
	}

	return errors.Join(errs...)
}

// CloseBeads is the close phase of the two-phase obsolescence flow.
// It closes each bead with spex:obsolete and commit:<HEAD> labels.
// Each failure is logged as a warning and accumulated. The batch continues
// even if individual closes fail. Returns an aggregated error of all
// warnings, or nil if all succeeded.
func CloseBeads(ctx context.Context, cli BeadCLI, actions []Action, logger *slog.Logger) error {
	if len(actions) == 0 {
		return nil
	}

	head, err := gitHEAD()
	if err != nil {
		return err
	}

	var errs []error
	labels := []string{"spex:obsolete", fmt.Sprintf("commit:%s", head)}

	for _, a := range actions {
		if err := cli.Close(ctx, a.BeadID, labels); err != nil {
			logger.WarnContext(ctx, "close bead failed",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("close %s (%s/%s): %w", a.BeadID, a.Module, a.Node, err))
		}
	}

	return errors.Join(errs...)
}
