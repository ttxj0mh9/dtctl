package main

import (
	"fmt"
	"os"

	"github.com/dtctl/dtctl/cmd"
)

// version is set during build via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// Use exit code 2 for usage errors vs 1 for general errors
		// TODO: differentiate between usage errors (2) and runtime errors (1)
		os.Exit(1)
	}
}
