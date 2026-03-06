package apply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// CloseBeads processes a batch of close actions sequentially.
// Each failure is logged as a warning and accumulated. The batch continues
// even if individual closes fail. Returns an aggregated error of all
// warnings, or nil if all succeeded.
func CloseBeads(ctx context.Context, cli BeadCLI, actions []Action, logger *slog.Logger) error {
	var errs []error

	for _, a := range actions {
		reason := fmt.Sprintf("Spec node removed: %s/%s", a.Module, a.Node)
		if err := cli.Close(ctx, a.BeadID, reason); err != nil {
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
