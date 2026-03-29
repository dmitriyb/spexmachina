package proposal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// execCommand runs a command and returns its stdout output.
func execCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}
