package apply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// UpdateBeads processes a batch of review actions sequentially.
// Each failure is logged as a warning and accumulated. The batch continues
// even if individual updates fail. Returns an aggregated error of all
// warnings, or nil if all succeeded.
func UpdateBeads(ctx context.Context, cli BeadCLI, actions []Action, logger *slog.Logger) error {
	var errs []error

	for _, a := range actions {
		metadata := map[string]string{"spec_hash": a.SpecHash}
		if err := cli.Update(ctx, a.BeadID, metadata); err != nil {
			logger.WarnContext(ctx, "update bead failed",
				"bead_id", a.BeadID,
				"module", a.Module,
				"node", a.Node,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("update %s (%s/%s): %w", a.BeadID, a.Module, a.Node, err))
		}
	}

	return errors.Join(errs...)
}
