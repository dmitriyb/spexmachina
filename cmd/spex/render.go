package main

import (
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/render"
	"github.com/spf13/cobra"
)

func newRenderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render [dir]",
		Short: "Render spec as markdown, DOT, or JSON",
		Long: `Render the spec directory in the specified format.

Output formats:
  markdown  Collated spec document (default)
  dot       Graphviz DOT graph
  json      Machine-readable JSON graph

With --format json, --slim reduces the graph to nodes only, each carrying just
{id, type, name, module}. Inlined content and descriptions are dropped and node
IDs are the bare identity hashes, which makes the output a compact name→hash
lookup table. Edges are omitted; read them from module.json.

Examples:
  spex render --format markdown
  spex render --format dot | dot -Tpng > spec.png
  spex render --format json | jq '.nodes[] | select(.type == "component")'
  spex render --format json --slim | jq -r '.nodes[] | "\(.name)\t\(.id)"'`,
		Args: cobra.MaximumNArgs(1),
		RunE: runRenderE,
	}
	cmd.Flags().StringP("format", "f", "markdown", "output format: markdown, dot, json")
	cmd.Flags().Bool("slim", false, "json only: emit nodes only as {id, type, name, module}")
	return cmd
}

func runRenderE(cmd *cobra.Command, args []string) error {
	var specDir string
	var err error

	if len(args) > 0 {
		specDir = args[0]
	} else {
		specDir, err = resolveSpecDir(cmd)
		if err != nil {
			return err
		}
	}

	format, _ := cmd.Flags().GetString("format")
	slim, _ := cmd.Flags().GetBool("slim")

	switch format {
	case "markdown", "dot", "json":
		// valid
	case "":
		return fmt.Errorf("render: invalid format %q (valid: markdown, dot, json)", format)
	default:
		return fmt.Errorf("render: unknown format %q (valid: markdown, dot, json)", format)
	}

	if slim && format != "json" {
		return fmt.Errorf("render: --slim requires --format json (got %q)", format)
	}

	spec, err := render.ReadSpec(specDir)
	if err != nil {
		return err
	}

	switch format {
	case "markdown":
		return render.RenderMarkdown(spec, os.Stdout)
	case "dot":
		return render.RenderDOT(spec, os.Stdout)
	case "json":
		if slim {
			return render.RenderJSONSlim(spec, os.Stdout)
		}
		return render.RenderJSON(spec, os.Stdout)
	}

	return nil
}
