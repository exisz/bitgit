// Package cli wires the cobra command tree.
//
// Each verb is a thin shell. Real work belongs in dedicated packages under
// internal/. Verbs MUST stay business-logic-free in this chassis release —
// they exist to let downstream maintainers fill in implementations behind
// stable command surfaces.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// BuildInfo is injected by main at startup.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd builds the bitgit command tree.
func NewRootCmd(b BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:   "bitgit",
		Short: "git + Bitbucket DC CLI with a hook-based plugin system",
		Long: `bitgit wraps git and Bitbucket Data Center operations behind generic verbs
(pr, commit, push, branch, doctor) and routes every action through a
hook-based plugin system. Plugins auto-match on remote URL or project key
and may veto or mutate operations via JSON-RPC over stdio.

Status: pre-alpha. Verbs are stubbed.`,
		Version:      fmt.Sprintf("%s (commit %s, built %s)", b.Version, b.Commit, b.Date),
		SilenceUsage: true,
	}

	root.AddCommand(
		newPRCmd(),
		newCommitCmd(),
		newPushCmd(),
		newBranchCmd(),
		newDoctorCmd(),
		newPluginCmd(),
	)
	return root
}
