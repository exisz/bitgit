package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// All verbs are intentional stubs. Yellow / downstream maintainers fill in
// real logic. The chassis only guarantees the command surface and plugin
// dispatch points (pre-* / post-* hooks fire around real implementations).

func notImplemented(verb string) error {
	return fmt.Errorf("%s: not implemented in this chassis release (v0.0.x). "+
		"Implementation lives downstream; this CLI exists to define the surface "+
		"and host plugin hooks", verb)
}

func newPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Pull request operations (create, show, list, merge)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "create",
			Short: "Create a pull request (fires pre-pr-create / post-pr-create hooks)",
			RunE:  func(c *cobra.Command, args []string) error { return notImplemented("pr create") },
		},
		&cobra.Command{
			Use:   "show <id>",
			Short: "Show a pull request",
			Args:  cobra.ExactArgs(1),
			RunE:  func(c *cobra.Command, args []string) error { return notImplemented("pr show") },
		},
		&cobra.Command{
			Use:   "list",
			Short: "List pull requests",
			RunE:  func(c *cobra.Command, args []string) error { return notImplemented("pr list") },
		},
		&cobra.Command{
			Use:   "merge <id>",
			Short: "Merge a pull request (fires pre-pr-merge hook)",
			Args:  cobra.ExactArgs(1),
			RunE:  func(c *cobra.Command, args []string) error { return notImplemented("pr merge") },
		},
	)
	return cmd
}

func newCommitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commit",
		Short: "Commit (fires pre-commit hook)",
		RunE:  func(c *cobra.Command, args []string) error { return notImplemented("commit") },
	}
}

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push (fires pre-push hook)",
		RunE:  func(c *cobra.Command, args []string) error { return notImplemented("push") },
	}
}

func newBranchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "branch",
		Short: "Branch operations",
		RunE:  func(c *cobra.Command, args []string) error { return notImplemented("branch") },
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose bitgit + plugin setup",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Fprintln(c.OutOrStdout(), "bitgit doctor — chassis OK")
			fmt.Fprintln(c.OutOrStdout(), "(real diagnostics added by downstream implementation)")
			return nil
		},
	}
}
