package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/gitutil"
	"github.com/exisz/bitgit/internal/host"
	"github.com/exisz/bitgit/internal/notify"
	"github.com/exisz/bitgit/internal/watchstore"
)

// hostFromRemoteURL extracts the bare host (no scheme/port path) from a git
// remote URL. Returns "" on parse failure.
func hostFromRemoteURL(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	// SSH form: git@bitbucket.example.com:PROJ/repo.git
	if strings.HasPrefix(remoteURL, "git@") {
		rest := strings.TrimPrefix(remoteURL, "git@")
		if i := strings.Index(rest, ":"); i > 0 {
			return strings.ToLower(rest[:i])
		}
	}
	// http(s)://host/...
	u, err := url.Parse(remoteURL)
	if err == nil && u.Host != "" {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// openWatchStore returns a Store rooted at <bitgit_dir>/state/pr-watch.json.
func openWatchStore(cfg *config.Config) (*watchstore.Store, error) {
	return watchstore.New(watchstore.DefaultPath(cfg.Dir()))
}

// notifyClient builds a notify.Client from cfg.
func notifyClient(cfg *config.Config) *notify.Client {
	return notify.New(notify.Config{
		WebhookURL:       cfg.Notify.WebhookURL,
		NotifyInProgress: cfg.Notify.NotifyInProgress,
	})
}

// registerPushedBranchWatch scans open PRs for one whose source branch
// matches the freshly pushed branch and registers it. Best-effort and silent
// on failure — it must never break a `push`.
func registerPushedBranchWatch(ctx context.Context, cfg *config.Config, branch string) {
	if branch == "" {
		return
	}
	h, remoteURL, err := host.DetectFromCWD(cfg.DefaultRemote, cfg)
	if err != nil {
		return
	}
	prs, err := h.ListPRs(ctx, "OPEN", false)
	if err != nil {
		return
	}
	for _, pr := range prs {
		if pr.SourceBranch == branch {
			registerWatch(ctx, cfg, h, remoteURL, pr.ID, "push", pr)
			return
		}
	}
}

// registerWatch is the shared entry-point used by `pr create`, `push`, and
// `pr watch`. It loads or fetches the PR, captures its current head SHA and
// metadata, and writes an Entry to the registry. Errors are logged but never
// returned to the caller — registration is best-effort.
//
// If pr is nil, it is fetched from the host using prID.
func registerWatch(ctx context.Context, cfg *config.Config, h host.Host, remoteURL, prID, source string, pr *host.PR) {
	store, err := openWatchStore(cfg)
	if err != nil {
		fmt.Fprintf(stderr(), "watch: cannot open registry: %v\n", err)
		return
	}
	if pr == nil {
		got, err := h.GetPR(ctx, prID)
		if err != nil {
			fmt.Fprintf(stderr(), "watch: cannot fetch PR %s: %v\n", prID, err)
			return
		}
		pr = got
	}
	hostName := hostFromRemoteURL(remoteURL)
	projectKey, repoSlug := gitutil.ParseProjectSlugFromURL(remoteURL)
	key := watchstore.MakeKey(hostName, projectKey, repoSlug, pr.ID)

	e := watchstore.Entry{
		Key:          key,
		Host:         hostName,
		ProjectKey:   projectKey,
		RepoSlug:     repoSlug,
		PRID:         pr.ID,
		URL:          pr.URL,
		Title:        pr.Title,
		SourceBranch: pr.SourceBranch,
		TargetBranch: pr.TargetBranch,
		HeadSHA:      pr.HeadSHA,
		Source:       source,
	}
	if err := store.Add(e); err != nil {
		fmt.Fprintf(stderr(), "watch: cannot register: %v\n", err)
		return
	}
}

// stderr is a small indirection so tests can swap it.
var stderr = func() *cobraStderr { return &cobraStderr{} }

type cobraStderr struct{}

func (c *cobraStderr) Write(p []byte) (int, error) { return fmt.Print(string(p)) }

// ---------------------------------------------------------------------------
// pr watch <id>     — manual register
// pr watch list     — show registry
// pr watch unregister <id>
// pr poll           — drain registry, notify on terminal states
// ---------------------------------------------------------------------------

func newPRWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Track PR build status and notify when it resolves",
	}
	cmd.AddCommand(
		newPRWatchAddCmd(),
		newPRWatchListCmd(),
		newPRWatchUnregisterCmd(),
	)
	return cmd
}

func newPRWatchAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id>",
		Short: "Manually register a PR for build-status polling",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			h, remoteURL, err := host.DetectFromCWD(cfg.DefaultRemote, cfg)
			if err != nil {
				return err
			}
			pr, err := h.GetPR(ctx, args[0])
			if err != nil {
				return fmt.Errorf("get PR: %w", err)
			}
			registerWatch(ctx, cfg, h, remoteURL, pr.ID, "manual", pr)
			fmt.Fprintf(c.OutOrStdout(), "watching PR #%s (%s)\n", pr.ID, pr.URL)
			return nil
		},
	}
}

func newPRWatchListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List PRs currently being watched for build status",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s, err := openWatchStore(cfg)
			if err != nil {
				return err
			}
			entries, err := s.List()
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(entries)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tSHA\tLAST\tPOLLS\tSOURCE")
			for _, e := range entries {
				sha := e.HeadSHA
				if len(sha) > 12 {
					sha = sha[:12]
				}
				last := e.LastState
				if last == "" {
					last = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
					e.Key, sha, last, e.PollCount, e.Source)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newPRWatchUnregisterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unregister <id>",
		Short: "Stop watching a PR (by id, derived from current repo's remote)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_, remoteURL, err := host.DetectFromCWD(cfg.DefaultRemote, cfg)
			if err != nil {
				return err
			}
			projectKey, repoSlug := gitutil.ParseProjectSlugFromURL(remoteURL)
			key := watchstore.MakeKey(hostFromRemoteURL(remoteURL), projectKey, repoSlug, args[0])
			s, err := openWatchStore(cfg)
			if err != nil {
				return err
			}
			if err := s.Remove(key); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "unregistered %s\n", key)
			return nil
		},
	}
}

// newPRPollCmd drains the registry. Each call:
//
//   - Lists all entries.
//   - For each, re-resolves the PR (so head_sha tracks fresh pushes), queries
//     build status, and:
//   - SUCCESSFUL/FAILED → notify + remove from registry.
//   - INPROGRESS → notify only when state changed and notify_inprogress
//     is enabled; record state.
//   - UNKNOWN → record state, leave in registry.
//
// Designed to be safe to run on a 15-minute cron: when the registry is empty
// it returns immediately with no host calls.
func newPRPollCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "poll",
		Short: "Poll the watch registry once and notify on resolved PRs",
		Long: `Iterate the PR watch registry once: for each entry, query build
status from the host. Terminal states (SUCCESSFUL, FAILED) emit a notification
and drop the entry from the registry. Non-terminal states stay queued.

If the registry is empty this exits immediately with no host calls — safe to
run on a tight schedule (e.g. every 15 minutes from cron) without busy-looping.`,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			s, err := openWatchStore(cfg)
			if err != nil {
				return err
			}
			entries, err := s.List()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				if asJSON {
					return json.NewEncoder(c.OutOrStdout()).Encode([]any{})
				}
				fmt.Fprintln(c.OutOrStdout(), "watch registry empty")
				return nil
			}

			results, err := pollEntries(ctx, cfg, entries)
			if err != nil {
				return err
			}

			// Apply registry mutations + send notifications.
			n := notifyClient(cfg)
			for _, r := range results {
				if r.Err != "" {
					fmt.Fprintf(stderr(), "poll %s: %s\n", r.Entry.Key, r.Err)
					continue
				}
				if r.NewHeadSHA != "" && r.NewHeadSHA != r.Entry.HeadSHA {
					_ = s.UpdateHeadSHA(r.Entry.Key, r.NewHeadSHA)
				}
				switch r.State {
				case "SUCCESSFUL", "FAILED":
					_ = s.UpdatePollResult(r.Entry.Key, r.State, true)
					if err := n.Send(ctx, formatNotification(r.Entry, r.State)); err != nil {
						fmt.Fprintf(stderr(), "notify: %v\n", err)
					}
				case "INPROGRESS":
					prevState := r.Entry.LastState
					_ = s.UpdatePollResult(r.Entry.Key, r.State, false)
					if cfg.Notify.NotifyInProgress && prevState != "INPROGRESS" {
						_ = n.Send(ctx, formatNotification(r.Entry, r.State))
					}
				default: // UNKNOWN or anything else
					_ = s.UpdatePollResult(r.Entry.Key, r.State, false)
				}
			}

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(results)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PR\tSTATE\tACTION")
			for _, r := range results {
				action := "kept"
				if r.State == "SUCCESSFUL" || r.State == "FAILED" {
					action = "resolved+notified"
				}
				if r.Err != "" {
					action = "error: " + r.Err
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Entry.Key, r.State, action)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

// PollResult is one entry's outcome from a poll cycle.
type PollResult struct {
	Entry      watchstore.Entry `json:"entry"`
	State      string           `json:"state"`
	NewHeadSHA string           `json:"new_head_sha,omitempty"`
	Err        string           `json:"error,omitempty"`
}

// pollEntries queries each entry's host. It groups entries by host hostname
// + project + repo so multiple entries from the same repo share one Host
// adapter where possible. For now it constructs a host adapter from the
// recorded URL each time (simple, correct).
func pollEntries(ctx context.Context, cfg *config.Config, entries []watchstore.Entry) ([]PollResult, error) {
	out := make([]PollResult, 0, len(entries))
	// Cache hosts by (host+projectKey+repoSlug) — building a Host requires a
	// remote URL. We reconstruct an https URL from the entry, which is enough
	// for Detect() to route to the right adapter.
	type hk struct{ host, proj, slug string }
	cache := map[hk]host.Host{}
	timeout := 30 * time.Second
	for _, e := range entries {
		ctx2, cancel := context.WithTimeout(ctx, timeout)
		k := hk{e.Host, e.ProjectKey, e.RepoSlug}
		h, ok := cache[k]
		if !ok {
			fakeRemote := fmt.Sprintf("https://%s/scm/%s/%s.git", e.Host, e.ProjectKey, e.RepoSlug)
			built, err := host.Detect(fakeRemote, cfg)
			if err != nil {
				out = append(out, PollResult{Entry: e, Err: err.Error()})
				cancel()
				continue
			}
			cache[k] = built
			h = built
		}
		// Re-fetch PR to track HeadSHA + current state without depending only
		// on what we cached at registration time.
		pr, err := h.GetPR(ctx2, e.PRID)
		if err != nil {
			out = append(out, PollResult{Entry: e, Err: err.Error()})
			cancel()
			continue
		}
		state := pr.CIState
		if state == "" {
			state = "UNKNOWN"
		}
		out = append(out, PollResult{
			Entry:      e,
			State:      state,
			NewHeadSHA: pr.HeadSHA,
		})
		cancel()
	}
	return out, nil
}

func formatNotification(e watchstore.Entry, state string) string {
	icon := "❓"
	switch state {
	case "SUCCESSFUL":
		icon = "✅"
	case "FAILED":
		icon = "❌"
	case "INPROGRESS":
		icon = "⏳"
	}
	title := e.Title
	if title == "" {
		title = "(no title)"
	}
	return fmt.Sprintf("%s PR #%s **%s** — %s\n%s", icon, e.PRID, state, title, e.URL)
}
