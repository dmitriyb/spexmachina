// Command gen-manifest generates the self-verifying release manifest
// spec/delivery/arch_manifest.md describes. It runs after GoReleaser has
// published a release's archives, reading GoReleaser's own artifacts.json
// and metadata.json records but recomputing every archive's sha256 and
// size from the bytes on disk rather than trusting GoReleaser's
// bookkeeping.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/delivery"
)

func main() {
	distDir := flag.String("dist", "dist", "GoReleaser output directory")
	out := flag.String("out", "", "manifest output path (default <dist>/manifest.json)")
	flag.Parse()

	if *out == "" {
		*out = filepath.Join(*distDir, "manifest.json")
	}

	if err := run(*distDir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "gen-manifest:", err)
		os.Exit(1)
	}
}

func run(distDir, out string) error {
	artifacts, err := delivery.LoadGoReleaserArtifacts(filepath.Join(distDir, "artifacts.json"))
	if err != nil {
		return err
	}
	meta, err := delivery.LoadGoReleaserMetadata(filepath.Join(distDir, "metadata.json"))
	if err != nil {
		return err
	}
	manifest, err := delivery.GenerateManifest(distDir, artifacts, meta)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}
