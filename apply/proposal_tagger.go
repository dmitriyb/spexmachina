package apply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// TagWithProposal tags all affected beads with the proposal reference that
// triggered the change. Each failure is logged as a warning and accumulated.
// The batch continues even if individual tags fail. Returns an aggregated
// error of all warnings, or nil if all succeeded.
func TagWithProposal(ctx context.Context, cli BeadCLI, beadIDs []string, proposalRef string, logger *slog.Logger) error {
	var errs []error

	for _, id := range beadIDs {
		metadata := map[string]string{"spec_proposal": proposalRef}
		if err := cli.Update(ctx, id, metadata); err != nil {
			logger.WarnContext(ctx, "tag bead with proposal failed",
				"bead_id", id,
				"proposal", proposalRef,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("tag %s with proposal %s: %w", id, proposalRef, err))
		}
	}

	return errors.Join(errs...)
}
