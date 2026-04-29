// Package reviewer provides reviewer resolution for pull requests.
// It merges the configured team list, recent-PR reviewers (optional),
// and any explicitly supplied reviewer names into a deduplicated,
// alphabetically sorted list.
package reviewer

import (
	"context"
	"sort"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/host"
)

// ResolveReviewers merges team + recent + explicit into a deduplicated,
// alphabetically sorted list.
//
//   - cfg.Reviewers.Team is always included.
//   - If cfg.Reviewers.IncludeRecent is true, the reviewers from the most
//     recent cfg.Reviewers.RecentLimit merged PRs are also included.
//   - explicit contains any reviewers supplied via --reviewer flags.
//
// If [reviewers] is absent from config.toml the function is a no-op and
// returns only the explicit list (deduplicated).
func ResolveReviewers(ctx context.Context, cfg *config.Config, h host.Host, explicit []string) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string

	add := func(username string) {
		if username == "" {
			return
		}
		if _, ok := seen[username]; !ok {
			seen[username] = struct{}{}
			result = append(result, username)
		}
	}

	// 1. Always-add team members.
	for _, u := range cfg.Reviewers.Team {
		add(u)
	}

	// 2. Pull reviewers from recent merged PRs when requested.
	if cfg.Reviewers.IncludeRecent {
		limit := cfg.Reviewers.RecentLimit
		if limit <= 0 {
			limit = 1
		}
		prs, err := h.ListPRs(ctx, "MERGED", false)
		if err != nil {
			return nil, err
		}
		// ListPRs returns PRs in descending order (most recent first).
		for i, pr := range prs {
			if i >= limit {
				break
			}
			for _, u := range pr.Reviewers {
				add(u)
			}
		}
	}

	// 3. Explicit reviewers (from --reviewer flags).
	for _, u := range explicit {
		add(u)
	}

	sort.Strings(result)
	return result, nil
}
