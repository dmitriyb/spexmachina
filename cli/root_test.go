package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// withSubcommands adds dummy subcommands to the root command so cobra's
// subcommand dispatch, help listing, and unknown-command detection all fire.
func withSubcommands(root *cobra.Command) *cobra.Command {
	for _, name := range []string{"validate", "hash", "diff", "impact", "apply", "map", "check"} {
		n := name
		root.AddCommand(&cobra.Command{
			Use:   n,
			Short: "dummy " + n,
			RunE:  func(cmd *cobra.Command, args []string) error { return nil },
		})
	}
	return root
}

func TestFR1_NoArgsPrintsHelp(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected help output containing 'Usage:', got:\n%s", out)
	}
	if !strings.Contains(out, "Available Commands:") {
		t.Errorf("expected help to list available commands, got:\n%s", out)
	}
}

func TestFR1_HelpFlagPrintsHelp(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected help output containing 'Usage:', got:\n%s", out)
	}
}

func TestFR1_SpecDirPersistentFlag(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	cmd.SetArgs([]string{"--spec-dir", "/custom/path", "validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := cmd.PersistentFlags().GetString("spec-dir")
	if err != nil {
		t.Fatalf("get spec-dir flag: %v", err)
	}
	if val != "/custom/path" {
		t.Errorf("want spec-dir=/custom/path, got %q", val)
	}
}

func TestFR1_SpecDirShortFlag(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	cmd.SetArgs([]string{"-s", "/short/path", "validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := cmd.PersistentFlags().GetString("spec-dir")
	if err != nil {
		t.Fatalf("get spec-dir flag: %v", err)
	}
	if val != "/short/path" {
		t.Errorf("want spec-dir=/short/path, got %q", val)
	}
}

func TestFR1_SpecDirDefault(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	cmd.SetArgs([]string{"validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := cmd.PersistentFlags().GetString("spec-dir")
	if err != nil {
		t.Fatalf("get spec-dir flag: %v", err)
	}
	if val != "spec/" {
		t.Errorf("want default spec-dir=spec/, got %q", val)
	}
}

func TestFR1_SilenceSettings(t *testing.T) {
	cmd := NewRootCmd()
	if !cmd.SilenceUsage {
		t.Error("SilenceUsage should be true")
	}
	if !cmd.SilenceErrors {
		t.Error("SilenceErrors should be true")
	}
}

func TestFR1_UnknownSubcommand(t *testing.T) {
	cmd := withSubcommands(NewRootCmd())
	cmd.SetArgs([]string{"foobar"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' in error, got: %v", err)
	}
}

func TestFR1_CompletionSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "bash") {
		t.Errorf("expected completion help to mention bash, got:\n%s", out)
	}
}

func TestFR1_SpecDirInheritedBySubcommand(t *testing.T) {
	cmd := NewRootCmd()
	var got string
	child := &cobra.Command{
		Use: "sub",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			got, err = cmd.Flags().GetString("spec-dir")
			return err
		},
	}
	cmd.AddCommand(child)
	cmd.SetArgs([]string{"--spec-dir", "/inherited", "sub"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/inherited" {
		t.Errorf("want spec-dir=/inherited in subcommand, got %q", got)
	}
}
