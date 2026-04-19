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
		// Exit code 1 for all errors for now
		// See: https://github.com/dynatrace-oss/dtctl/issues for upstream discussion
		os.Exit(1)
	}
}
