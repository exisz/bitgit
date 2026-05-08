// Package host defines the Host interface and auto-detection logic.
package host

import (
	"context"
	"fmt"
	"strings"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/gitutil"
)

// PR represents a pull request in a normalised form.
type PR struct {
	ID           string
	URL          string
	Title        string
	Description  string
	State        string // OPEN | MERGED | DECLINED | SUPERSEDED
	Draft        bool
	SourceBranch string
	TargetBranch string
	HeadSHA      string
	Reviewers    []string
	Approvals    []string // users who approved
	Blockers     []string // blocker comment IDs
	CIState      string   // SUCCESSFUL | FAILED | INPROGRESS | UNKNOWN
	LastComment  *Comment
}

// Comment represents a PR comment.
type Comment struct {
	ID     string
	Author string
	Body   string
	// Timestamp is RFC3339 or similar human-readable string.
	Timestamp string
}

// CreatePRInput carries all fields for a new pull request.
type CreatePRInput struct {
	Title        string
	Description  string
	SourceBranch string
	TargetBranch string
	Draft        bool
	Reviewers    []string
}

// Host is the interface every git-hosting adapter must implement.
type Host interface {
	// CreatePR creates a new pull request and returns the created PR.
	CreatePR(ctx context.Context, in CreatePRInput) (*PR, error)
	// GetPR fetches a pull request by ID.
	GetPR(ctx context.Context, id string) (*PR, error)
	// ListPRs lists pull requests. If mine is true, filter to the authed user.
	ListPRs(ctx context.Context, state string, mine bool) ([]*PR, error)
	// MergePR merges a pull request by ID. Returns the merge commit SHA.
	MergePR(ctx context.Context, id string) (string, error)
	// CommentPR posts a comment on a PR. replyTo is a comment ID (empty = top-level).
	CommentPR(ctx context.Context, id, text, replyTo string) error
	// ListComments returns all comments on a PR in chronological order.
	ListComments(ctx context.Context, id string) ([]*Comment, error)
	// GetBuildStatus returns the CI state for a commit SHA.
	GetBuildStatus(ctx context.Context, sha string) (string, error)
	// GetReviewers returns the list of reviewer usernames for a PR.
	GetReviewers(ctx context.Context, id string) ([]string, error)
	// DeclinePR declines (closes without merging) a pull request by ID.
	DeclinePR(ctx context.Context, id string) error
	// UpdatePR updates title, description, and adds reviewers to an existing PR.
	UpdatePR(ctx context.Context, id, title, description string, addReviewers []string) error
	// CurrentUser returns the authenticated username.
	CurrentUser(ctx context.Context) (string, error)
}

// Detect sniffs the remote URL and returns the appropriate Host adapter.
func Detect(remoteURL string, cfg *config.Config) (Host, error) {
	switch {
	case remoteURL == "":
		return nil, fmt.Errorf("could not determine remote URL; are you in a git repo?")
	case strings.Contains(remoteURL, "github.com"):
		return newGitHubHost(cfg)
	case strings.Contains(remoteURL, "bitbucket.org"):
		return nil, fmt.Errorf("bitbucket.org (cloud) is not supported in v0.1; use bitbucket-dc or github")
	default:
		// Heuristic: treat as Bitbucket Data Center
		return newBitbucketDCHost(remoteURL, cfg)
	}
}

// DetectFromCWD detects the host from the current working directory's git remote.
func DetectFromCWD(remoteName string, cfg *config.Config) (Host, string, error) {
	if remoteName == "" {
		remoteName = cfg.DefaultRemote
		if remoteName == "" {
			remoteName = "origin"
		}
	}
	remoteURL, err := gitutil.RemoteURL(remoteName)
	if err != nil {
		return nil, "", fmt.Errorf("get remote %s: %w", remoteName, err)
	}
	h, err := Detect(remoteURL, cfg)
	return h, remoteURL, err
}

// newGitHubHost is a forward declaration; actual impl in internal/host/github.
func newGitHubHost(cfg *config.Config) (Host, error) {
	return newGitHub(cfg)
}

// newBitbucketDCHost is a forward declaration; actual impl in internal/host/bitbucket_dc.
func newBitbucketDCHost(remoteURL string, cfg *config.Config) (Host, error) {
	return newBitbucketDC(remoteURL, cfg)
}
