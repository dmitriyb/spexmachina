// Command curl is a minimal stand-in for the real curl(1), built and put
// on PATH only by the self-update integration tests in a sandbox that
// has no real curl. install.sh itself only ever invokes:
//
//	curl -fsSL <url>            (print body to stdout)
//	curl -fsSL -o <file> <url>  (write body to file)
//
// which is the entire surface this shim implements; it is never part of
// the shipped script or binary.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	var out, url string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o":
			i++
			if i < len(args) {
				out = args[i]
			}
		case len(a) > 0 && a[0] == '-':
			// combined short flags (-fsSL etc.) carry no state this shim needs
		default:
			url = a
		}
	}
	if url == "" {
		fmt.Fprintln(os.Stderr, "curl: no URL given")
		os.Exit(2)
	}

	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "curl:", err)
		os.Exit(7)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fmt.Fprintln(os.Stderr, "curl: http", resp.StatusCode)
		os.Exit(22)
	}

	w := io.Writer(os.Stdout)
	if out != "" {
		f, err := os.Create(out)
		if err != nil {
			fmt.Fprintln(os.Stderr, "curl:", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Fprintln(os.Stderr, "curl:", err)
		os.Exit(1)
	}
}
