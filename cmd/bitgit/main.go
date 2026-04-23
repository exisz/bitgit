// Command bitgit is a git+Bitbucket Data Center CLI with a hook-based plugin system.
//
// Status: pre-alpha chassis. Business logic lives in internal/cli, plugin
// runtime lives in internal/plugin. Plugins are discovered from
// ~/.bitgit/plugins/<name>/plugin.toml and communicate over JSON-RPC stdio.
package main

import (
	"fmt"
	"os"

	"github.com/exisz/bitgit/internal/cli"
)

// Version is set at build time by goreleaser via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bitgit:", err)
		os.Exit(1)
	}
}
