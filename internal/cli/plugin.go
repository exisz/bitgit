package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/exisz/bitgit/internal/plugin"
	"github.com/spf13/cobra"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage bitgit plugins",
	}
	cmd.AddCommand(newPluginListCmd(), newPluginInfoCmd())
	return cmd
}

func pluginsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bitgit", "plugins"), nil
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins discovered under ~/.bitgit/plugins",
		RunE: func(c *cobra.Command, args []string) error {
			dir, err := pluginsDir()
			if err != nil {
				return err
			}
			plugins, err := plugin.Discover(dir)
			if err != nil {
				return err
			}
			if len(plugins) == 0 {
				fmt.Fprintf(c.OutOrStdout(), "no plugins found in %s\n", dir)
				return nil
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tVERSION\tHOOKS\tMATCH")
			for _, p := range plugins {
				fmt.Fprintf(tw, "%s\t%s\t%v\t%v\n", p.Name, p.Version, p.Hooks, p.Match)
			}
			return tw.Flush()
		},
	}
}

func newPluginInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show details of a single plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			dir, err := pluginsDir()
			if err != nil {
				return err
			}
			plugins, err := plugin.Discover(dir)
			if err != nil {
				return err
			}
			for _, p := range plugins {
				if p.Name == args[0] {
					fmt.Fprintf(c.OutOrStdout(), "Name:       %s\nVersion:    %s\nEntrypoint: %s\nHooks:      %v\nMatch:      %v\nDir:        %s\n",
						p.Name, p.Version, p.Entrypoint, p.Hooks, p.Match, p.Dir)
					return nil
				}
			}
			return fmt.Errorf("plugin %q not found in %s", args[0], dir)
		},
	}
}
