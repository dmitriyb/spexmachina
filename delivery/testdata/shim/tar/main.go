// Command tar is a minimal stand-in for the real tar(1), built and put
// on PATH only by the self-update integration tests in a sandbox that
// has no real tar. install.sh itself only ever invokes:
//
//	tar -xzf <archive> -C <dir>
//
// which is the entire surface this shim implements; it is never part of
// the shipped script or binary.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	var archive, destDir string
	extract := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-xzf", "-xzvf", "xzf":
			extract = true
			i++
			if i < len(args) {
				archive = args[i]
			}
		case "-C":
			i++
			if i < len(args) {
				destDir = args[i]
			}
		}
	}
	if !extract || archive == "" {
		fmt.Fprintln(os.Stderr, "tar: usage: tar -xzf <archive> -C <dir>")
		os.Exit(2)
	}
	if destDir == "" {
		destDir = "."
	}

	if err := extractTarGz(archive, destDir); err != nil {
		fmt.Fprintln(os.Stderr, "tar:", err)
		os.Exit(1)
	}
}

func extractTarGz(archive, destDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		outPath := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, tr)
		out.Close()
		if cerr != nil {
			return cerr
		}
	}
}
