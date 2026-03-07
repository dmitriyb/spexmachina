package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spex <command> [args]")
		fmt.Fprintln(os.Stderr, "commands: hash")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "hash":
		os.Exit(runHash(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "spex: unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
