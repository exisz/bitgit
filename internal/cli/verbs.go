package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/gitutil"
	"github.com/exisz/bitgit/internal/host"
	"github.com/exisz/bitgit/internal/plugin"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// hookContext builds a plugin.Context from the current git state.
func hookContext(hook string, cfg *config.Config) plugin.Context {
	branch, _ := gitutil.CurrentBranch()
	remoteURL, _ := gitutil.RemoteURL(cfg.DefaultRemote)
	projectKey, repoSlug := gitutil.ParseProjectSlugFromURL(remoteURL)
	return plugin.Context{
		RemoteURL:  remoteURL,
		ProjectKey: projectKey,
		RepoSlug:   repoSlug,
		Branch:     branch,
		Hook:       hook,
	}
}

// loadPlugins discovers plugins, filtering out disabled ones.
func loadPlugins(cfg *config.Config) ([]plugin.Manifest, error) {
	manifests, err := plugin.Discover(cfg.PluginsDir())
	if err != nil {
		return nil, fmt.Errorf("discover plugins: %w", err)
	}
	disabled := map[string]bool{}
	for _, d := range cfg.Plugins.Disabled {
		disabled[d] = true
	}
	var out []plugin.Manifest
	for _, m := range manifests {
		if !disabled[m.Name] {
			out = append(out, m)
		}
	}
	return out, nil
}

// detectHost loads config + detects git host adapter.
func detectHost(cfg *config.Config) (host.Host, error) {
	h, _, err := host.DetectFromCWD(cfg.DefaultRemote, cfg)
	return h, err
}

// dispatch fires a hook and returns the (potentially mutated) payload.
// It returns a VetoError if any plugin vetoed.
func dispatch(ctx context.Context, manifests []plugin.Manifest, hookCtx plugin.Context, payload map[string]any) error {
	return plugin.Dispatch(ctx, manifests, hookCtx, payload)
}

// postDispatch fires a post-hook. Vetoes are silently ignored per spec.
func postDispatch(ctx context.Context, manifests []plugin.Manifest, hookCtx plugin.Context, payload map[string]any) {
	_ = plugin.Dispatch(ctx, manifests, hookCtx, payload)
}

// stringsFromPayload coerces a payload field to []string.
func stringsFromPayload(payload map[string]any, key string) []string {
	v, ok := payload[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func strFromPayload(payload map[string]any, key, fallback string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func boolFromPayload(payload map[string]any, key string, fallback bool) bool {
	if v, ok := payload[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}

// runGit runs a git command streaming stdout/stderr to the terminal.
func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ---------------------------------------------------------------------------
// pr create
// ---------------------------------------------------------------------------

func newPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Pull request operations",
	}
	cmd.AddCommand(
		newPRCreateCmd(),
		newPRShowCmd(),
		newPRListCmd(),
		newPRCommentCmd(),
		newPRCommentsCmd(),
		newPRReplyCmd(),
		newPRReadyCmd(),
		newPRMergeCmd(),
		newPRBlockersCmd(),
	)
	return cmd
}

func newPRCreateCmd() *cobra.Command {
	var (
		title       string
		description string
		descFile    string
		source      string
		target      string
		draft       bool
		reviewers   []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			// Resolve source branch
			if source == "" {
				source, err = gitutil.CurrentBranch()
				if err != nil {
					return fmt.Errorf("cannot determine current branch: %w", err)
				}
			}
			if target == "" {
				return fmt.Errorf("--target is required")
			}

			// Read description from file if -F given
			if descFile != "" {
				data, err := os.ReadFile(descFile)
				if err != nil {
					return fmt.Errorf("read description file: %w", err)
				}
				description = string(data)
			}

			payload := map[string]any{
				"source_branch": source,
				"target_branch": target,
				"title":         title,
				"description":   description,
				"draft":         draft,
				"reviewers":     reviewers,
			}

			hookCtx := hookContext("pre-pr-create", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}

			// Apply mutations
			source = strFromPayload(payload, "source_branch", source)
			target = strFromPayload(payload, "target_branch", target)
			title = strFromPayload(payload, "title", title)
			description = strFromPayload(payload, "description", description)
			draft = boolFromPayload(payload, "draft", draft)
			reviewers = stringsFromPayload(payload, "reviewers")

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}

			pr, err := h.CreatePR(ctx, host.CreatePRInput{
				Title:        title,
				Description:  description,
				SourceBranch: source,
				TargetBranch: target,
				Draft:        draft,
				Reviewers:    reviewers,
			})
			if err != nil {
				return fmt.Errorf("create PR: %w", err)
			}

			fmt.Fprintf(c.OutOrStdout(), "PR #%s created: %s\n", pr.ID, pr.URL)

			// post-pr-create (informational; cannot veto)
			hookCtx.Hook = "post-pr-create"
			postDispatch(ctx, manifests, hookCtx, map[string]any{
				"pr_id":         pr.ID,
				"pr_url":        pr.URL,
				"title":         pr.Title,
				"description":   pr.Description,
				"source_branch": pr.SourceBranch,
				"target_branch": pr.TargetBranch,
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "PR title (required)")
	cmd.Flags().StringVar(&description, "description", "", "PR description")
	cmd.Flags().StringVarP(&descFile, "file", "F", "", "Read description from file")
	cmd.Flags().StringVar(&source, "source", "", "Source branch (default: current branch)")
	cmd.Flags().StringVar(&target, "target", "", "Target branch (required)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create as draft PR")
	cmd.Flags().StringArrayVar(&reviewers, "reviewer", nil, "Reviewer username (repeatable)")
	return cmd
}

// ---------------------------------------------------------------------------
// pr show
// ---------------------------------------------------------------------------

func newPRShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}

			pr, err := h.GetPR(ctx, args[0])
			if err != nil {
				return fmt.Errorf("get PR: %w", err)
			}

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(pr)
			}

			w := c.OutOrStdout()
			sha := pr.HeadSHA
			if len(sha) > 12 {
				sha = sha[:12]
			}
			fmt.Fprintf(w, "PR #%s  %s\n", pr.ID, pr.Title)
			fmt.Fprintf(w, "State:     %s\n", pr.State)
			fmt.Fprintf(w, "Branch:    %s → %s\n", pr.SourceBranch, pr.TargetBranch)
			fmt.Fprintf(w, "Head SHA:  %s\n", sha)
			fmt.Fprintf(w, "CI:        %s\n", pr.CIState)
			fmt.Fprintf(w, "Approvals: %d (%s)\n", len(pr.Approvals), strings.Join(pr.Approvals, ", "))
			fmt.Fprintf(w, "Blockers:  %d\n", len(pr.Blockers))
			fmt.Fprintf(w, "Reviewers: %s\n", strings.Join(pr.Reviewers, ", "))
			if pr.LastComment != nil {
				fmt.Fprintf(w, "Last comment: %s at %s\n", pr.LastComment.Author, pr.LastComment.Timestamp)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// pr list
// ---------------------------------------------------------------------------

func newPRListCmd() *cobra.Command {
	var (
		mine   bool
		state  string
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}

			prs, err := h.ListPRs(ctx, state, mine)
			if err != nil {
				return fmt.Errorf("list PRs: %w", err)
			}

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(prs)
			}

			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTITLE\tSTATE\tSOURCE\tTARGET")
			for _, pr := range prs {
				fmt.Fprintf(tw, "#%s\t%s\t%s\t%s\t%s\n",
					pr.ID, pr.Title, pr.State, pr.SourceBranch, pr.TargetBranch)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&mine, "mine", false, "Only show my PRs")
	cmd.Flags().StringVar(&state, "state", "OPEN", "Filter by state (OPEN|MERGED|DECLINED|ALL)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// pr comment (top-level only; use pr reply for threaded replies)
// ---------------------------------------------------------------------------

func newPRCommentCmd() *cobra.Command {
	var topLevel bool
	cmd := &cobra.Command{
		Use:   "comment <id> <text>",
		Short: "Post a top-level comment on a PR (requires --top-level)",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			if !topLevel {
				return fmt.Errorf("pr comment requires --top-level flag (use \"pr reply <comment-id> <text>\" for in-thread replies)")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			prID := args[0]
			text := args[1]

			payload := map[string]any{
				"pr_id":    prID,
				"text":     text,
				"reply_to": 0,
			}
			hookCtx := hookContext("pre-pr-comment", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}
			text = strFromPayload(payload, "text", text)

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}
			if err := h.CommentPR(ctx, prID, text, ""); err != nil {
				return fmt.Errorf("comment PR: %w", err)
			}
			fmt.Fprintln(c.OutOrStdout(), "comment posted")
			return nil
		},
	}
	cmd.Flags().BoolVar(&topLevel, "top-level", false, "Explicitly post as a top-level comment")
	return cmd
}

// ---------------------------------------------------------------------------
// pr comments (list)
// ---------------------------------------------------------------------------

func newPRCommentsCmd() *cobra.Command {
	var asJSON bool
	return &cobra.Command{
		Use:   "comments <id>",
		Short: "List all comments on a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}
			comments, err := h.ListComments(ctx, args[0])
			if err != nil {
				return fmt.Errorf("list comments: %w", err)
			}
			if len(comments) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "no comments")
				return nil
			}
			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(comments)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tAUTHOR\tTIMESTAMP\tBODY")
			for _, cm := range comments {
				body := cm.Body
				if len(body) > 60 {
					body = body[:57] + "..."
				}
				// strip newlines for table display
				body = strings.ReplaceAll(body, "\n", " ")
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", cm.ID, cm.Author, cm.Timestamp, body)
			}
			return tw.Flush()
		},
	}
}

// ---------------------------------------------------------------------------
// pr reply (in-thread)
// ---------------------------------------------------------------------------

func newPRReplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reply <pr-id> <comment-id> <text>",
		Short: "Reply in-thread to a PR comment",
		Args:  cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			prID := args[0]
			commentID := args[1]
			text := args[2]

			replyInt, _ := strconv.Atoi(commentID)
			payload := map[string]any{
				"pr_id":    prID,
				"text":     text,
				"reply_to": replyInt,
			}
			hookCtx := hookContext("pre-pr-comment", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}
			text = strFromPayload(payload, "text", text)

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}
			// replyTo forces threading; adapter handles parent comment linkage
			if err := h.CommentPR(ctx, prID, text, commentID); err != nil {
				return fmt.Errorf("reply: %w", err)
			}
			fmt.Fprintln(c.OutOrStdout(), "reply posted")
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// pr ready
// ---------------------------------------------------------------------------

func newPRReadyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready <id>",
		Short: "Mark a draft PR as ready for review",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}

			prID := args[0]
			pr, err := h.GetPR(ctx, prID)
			if err != nil {
				return fmt.Errorf("get PR: %w", err)
			}

			payload := map[string]any{
				"pr_id":               prID,
				"current_title":       pr.Title,
				"current_description": pr.Description,
				"current_reviewers":   pr.Reviewers,
				"head_sha":            pr.HeadSHA,
				"ci_state":            pr.CIState,
				"add_reviewers":       []string{},
			}
			hookCtx := hookContext("pre-pr-ready", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}

			newTitle := strFromPayload(payload, "title", pr.Title)
			newDesc := strFromPayload(payload, "description", pr.Description)
			addReviewers := stringsFromPayload(payload, "add_reviewers")

			if err := h.UpdatePR(ctx, prID, newTitle, newDesc, addReviewers); err != nil {
				return fmt.Errorf("update PR: %w", err)
			}
			fmt.Fprintf(c.OutOrStdout(), "PR #%s marked ready for review\n", prID)
			return nil
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// pr merge
// ---------------------------------------------------------------------------

func newPRMergeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge <id>",
		Short: "Merge a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}

			prID := args[0]
			pr, err := h.GetPR(ctx, prID)
			if err != nil {
				return fmt.Errorf("get PR: %w", err)
			}

			payload := map[string]any{
				"pr_id":         prID,
				"source_branch": pr.SourceBranch,
				"target_branch": pr.TargetBranch,
				"head_sha":      pr.HeadSHA,
				"approvals":     pr.Approvals,
				"blockers":      pr.Blockers,
			}
			hookCtx := hookContext("pre-pr-merge", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}

			mergeCommit, err := h.MergePR(ctx, prID)
			if err != nil {
				return fmt.Errorf("merge PR: %w", err)
			}

			fmt.Fprintf(c.OutOrStdout(), "PR #%s merged (commit: %s)\n", prID, mergeCommit)

			hookCtx.Hook = "post-pr-merge"
			postDispatch(ctx, manifests, hookCtx, map[string]any{
				"pr_id":         prID,
				"source_branch": pr.SourceBranch,
				"target_branch": pr.TargetBranch,
				"merge_commit":  mergeCommit,
			})
			return nil
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// pr blockers (stretch)
// ---------------------------------------------------------------------------

func newPRBlockersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "blockers <id>",
		Short: "List blocker comments on a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			h, err := detectHost(cfg)
			if err != nil {
				return err
			}
			pr, err := h.GetPR(ctx, args[0])
			if err != nil {
				return err
			}
			if len(pr.Blockers) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "no blocker comments")
				return nil
			}
			for _, id := range pr.Blockers {
				fmt.Fprintf(c.OutOrStdout(), "blocker comment id: %s\n", id)
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// commit
// ---------------------------------------------------------------------------

func newCommitCmd() *cobra.Command {
	var message string
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Stage a commit (fires pre-commit hook)",
		RunE: func(_ *cobra.Command, _ []string) error {
			if message == "" {
				return fmt.Errorf("-m message is required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			staged, _ := gitutil.StagedFiles()
			stats, _ := gitutil.StagedDiffStats()

			payload := map[string]any{
				"message":      message,
				"staged_files": staged,
				"diff_stats": map[string]any{
					"files_changed": stats.FilesChanged,
					"insertions":    stats.Insertions,
					"deletions":     stats.Deletions,
				},
			}
			hookCtx := hookContext("pre-commit", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}
			message = strFromPayload(payload, "message", message)

			return runGit("commit", "-m", message)
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message (required)")
	return cmd
}

// ---------------------------------------------------------------------------
// push
// ---------------------------------------------------------------------------

func newPushCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push to remote (fires pre-push hook)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			remote := cfg.DefaultRemote
			if remote == "" {
				remote = "origin"
			}
			branch, _ := gitutil.CurrentBranch()
			sha, _ := gitutil.HeadSHAShort()
			parents, _ := gitutil.HeadParents()
			ahead, _ := gitutil.CommitsAhead(branch, remote)
			refspecs := gitutil.Refspecs(branch)

			payload := map[string]any{
				"remote":         remote,
				"refspecs":       refspecs,
				"current_branch": branch,
				"head_sha":       sha,
				"head_parents":   parents,
				"commits_ahead":  ahead,
				"force":          force,
			}
			hookCtx := hookContext("pre-push", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}

			gitArgs := []string{"push", remote, branch}
			if force {
				gitArgs = append(gitArgs, "--force")
			}
			return runGit(gitArgs...)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Force push")
	return cmd
}

// ---------------------------------------------------------------------------
// branch new
// ---------------------------------------------------------------------------

func newBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Branch operations",
	}
	cmd.AddCommand(newBranchNewCmd())
	return cmd
}

func newBranchNewCmd() *cobra.Command {
	var fromRef string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			manifests, err := loadPlugins(cfg)
			if err != nil {
				return err
			}

			name := args[0]
			payload := map[string]any{
				"name":     name,
				"from_ref": fromRef,
			}
			hookCtx := hookContext("pre-branch-new", cfg)
			if err := dispatch(ctx, manifests, hookCtx, payload); err != nil {
				if plugin.IsVeto(err) {
					return fmt.Errorf("vetoed: %w", err)
				}
				return err
			}
			name = strFromPayload(payload, "name", name)

			gitArgs := []string{"checkout", "-b", name}
			if fromRef != "" {
				gitArgs = append(gitArgs, fromRef)
			}
			return runGit(gitArgs...)
		},
	}
	cmd.Flags().StringVar(&fromRef, "from", "", "Create branch from ref (default: HEAD)")
	return cmd
}

// ---------------------------------------------------------------------------
// doctor
// ---------------------------------------------------------------------------

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose bitgit + plugin setup",
		RunE: func(c *cobra.Command, _ []string) error {
			w := c.OutOrStdout()
			fmt.Fprintln(w, "=== bitgit doctor ===")
			fmt.Fprintln(w)

			cfg, cfgErr := config.Load()
			if cfgErr != nil {
				fmt.Fprintf(w, "config: ERROR: %v\n", cfgErr)
				cfg, _ = config.Load() // use defaults
			} else {
				fmt.Fprintf(w, "config dir:     %s\n", cfg.Dir())
				fmt.Fprintf(w, "default remote: %s\n", cfg.DefaultRemote)
				fmt.Fprintf(w, "hosts:          %d configured\n", len(cfg.Hosts))
				if cfg.InsecureSkipVerify {
					fmt.Fprintln(w, "WARNING: insecure_skip_verify is enabled!")
				}
			}
			fmt.Fprintln(w)

			// Plugins
			manifests, err := loadPlugins(cfg)
			if err != nil {
				fmt.Fprintf(w, "plugins: ERROR: %v\n", err)
			} else {
				fmt.Fprintf(w, "plugins (%d found in %s):\n", len(manifests), cfg.PluginsDir())
				for _, m := range manifests {
					fmt.Fprintf(w, "  %-20s v%-8s hooks: %v  match: %+v\n", m.Name, m.Version, m.Hooks, m.Match)
				}
				if len(cfg.Plugins.Disabled) > 0 {
					fmt.Fprintf(w, "  disabled: %v\n", cfg.Plugins.Disabled)
				}
			}
			fmt.Fprintln(w)

			// Git repo context
			if !gitutil.IsGitRepo() {
				fmt.Fprintln(w, "git: not inside a git repository")
				return nil
			}

			branch, _ := gitutil.CurrentBranch()
			sha, _ := gitutil.HeadSHA()
			parents, _ := gitutil.HeadParents()
			remote := cfg.DefaultRemote
			if remote == "" {
				remote = "origin"
			}
			remoteURL, _ := gitutil.RemoteURL(remote)
			projectKey, repoSlug := gitutil.ParseProjectSlugFromURL(remoteURL)

			fmt.Fprintf(w, "git repo:\n")
			fmt.Fprintf(w, "  branch:      %s\n", branch)
			fmt.Fprintf(w, "  head sha:    %s\n", sha)
			fmt.Fprintf(w, "  parents:     %d (%v)\n", len(parents), parents)
			if len(parents) == 0 {
				fmt.Fprintln(w, "  WARNING: orphan commit (no parents)")
			}
			fmt.Fprintf(w, "  remote:      %s\n", remote)
			fmt.Fprintf(w, "  remote url:  %s\n", remoteURL)
			fmt.Fprintf(w, "  project key: %s\n", projectKey)
			fmt.Fprintf(w, "  repo slug:   %s\n", repoSlug)

			if gitutil.IsShallowRepo() {
				fmt.Fprintln(w, "  WARNING: shallow clone detected (≤50 commits). Some operations may fail.")
			}

			// Detect host
			fmt.Fprintln(w)
			fmt.Fprintf(w, "host detection:\n")
			h, _, hostErr := host.DetectFromCWD(remote, cfg)
			if hostErr != nil {
				fmt.Fprintf(w, "  ERROR: %v\n", hostErr)
			} else {
				fmt.Fprintf(w, "  host adapter loaded OK\n")
				// Connectivity check
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				user, connErr := h.CurrentUser(ctx)
				if connErr != nil {
					fmt.Fprintf(w, "  connectivity: ERROR: %v\n", connErr)
				} else {
					if user != "" {
						fmt.Fprintf(w, "  connectivity: OK (user: %s)\n", user)
					} else {
						fmt.Fprintln(w, "  connectivity: OK (anonymous or user lookup not supported)")
					}
				}
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, "doctor complete — no fixes applied")
			return nil
		},
	}
}
