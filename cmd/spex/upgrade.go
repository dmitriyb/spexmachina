package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/delivery"
	"github.com/spf13/cobra"
)

// installScript is the installer script this command drives via its
// upgrade-mode invocation. A package variable rather than a direct
// reference to delivery.InstallScript at each call site, so tests can
// substitute a stub script that captures argv and environment without
// touching the production copy.
var installScript = delivery.InstallScript

func newUpgradeCmd() *cobra.Command {
	var versionPin string
	var check bool
	var rollback bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Self-update the installed binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgradeE(cmd, versionPin, check, rollback)
		},
	}

	cmd.Flags().StringVar(&versionPin, "version", "", "install this exact release, in any direction")
	cmd.Flags().BoolVar(&check, "check", false, "report the comparison and change nothing")
	cmd.Flags().BoolVar(&check, "dry-run", false, "alias for --check")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "restore the previous binary from its kept backup")

	return cmd
}

// runUpgradeE translates the upgrade flag surface into the embedded
// installer's upgrade-mode invocation, per arch_upgrade_command.md's
// "Translation contract": the running binary's own resolved path as the
// target, the compiled-in version stamp as the current version (omitted
// entirely for a dev build, letting the script probe the target), a
// pinned --version through the script's SPEX_INSTALL_VERSION environment
// contract rather than a new flag, and check/rollback appended as given.
// The script's exit status becomes the command's own.
func runUpgradeE(cmd *cobra.Command, versionPin string, check, rollback bool) error {
	target, err := os.Executable()
	if err != nil {
		return fmt.Errorf("upgrade: resolve running binary path: %w", err)
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("upgrade: resolve running binary path: %w", err)
	}

	scriptArgs := []string{"--upgrade", "--target", target}
	if version != "dev" {
		scriptArgs = append(scriptArgs, "--current-version", version)
	}
	if check {
		scriptArgs = append(scriptArgs, "--check")
	}
	if rollback {
		scriptArgs = append(scriptArgs, "--rollback")
	}

	var env []string
	if versionPin != "" {
		env = append(env, "SPEX_INSTALL_VERSION="+versionPin)
	}

	exitCode, err := runInstallScript(cmd.Context(), scriptArgs, env, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}
	if exitCode != 0 {
		return &upgradeError{
			code: exitCode,
			err:  fmt.Errorf("upgrade: installer exited with status %d", exitCode),
		}
	}
	return nil
}

// runInstallScript stages installScript to a temp file and runs it under
// bash with args and env (in addition to the process environment),
// streaming stdout/stderr/stdin and returning its exit code.
func runInstallScript(ctx context.Context, args, env []string, stdout, stderr io.Writer, stdin io.Reader) (int, error) {
	dir, err := os.MkdirTemp("", "spex-upgrade-")
	if err != nil {
		return 0, fmt.Errorf("stage installer script: %w", err)
	}
	defer os.RemoveAll(dir)

	scriptPath := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(scriptPath, []byte(installScript), 0o700); err != nil {
		return 0, fmt.Errorf("stage installer script: %w", err)
	}

	execCmd := exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
	execCmd.Env = append(os.Environ(), env...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	execCmd.Stdin = stdin

	if err := execCmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, fmt.Errorf("run installer script: %w", err)
	}
	return 0, nil
}

// upgradeError carries a process exit code alongside the wrapped error.
// main inspects the ExitCode interface to honor the installer script's own
// exit status as the command's exit status, per arch_upgrade_command.md's
// "Translation contract": a refusal propagates, a report exits 0.
type upgradeError struct {
	code int
	err  error
}

func (e *upgradeError) Error() string { return e.err.Error() }
func (e *upgradeError) Unwrap() error { return e.err }
func (e *upgradeError) ExitCode() int { return e.code }
