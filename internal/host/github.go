package host

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/gitutil"
)

// gitHubHost implements Host against github.com via go-github.
type gitHubHost struct {
	client *gogithub.Client
	owner  string
	repo   string
}

func newGitHub(cfg *config.Config) (Host, error) {
	token, err := cfg.ReadToken("github")
	if err != nil {
		return nil, fmt.Errorf("github: read token: %w", err)
	}

	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	}

	client := gogithub.NewClient(httpClient)

	// Resolve owner/repo from current remote
	remoteURL, _ := gitutil.RemoteURL("origin")
	owner, repo := gitutil.ParseProjectSlugFromURL(remoteURL)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("github: cannot parse owner/repo from remote URL %q", remoteURL)
	}

	return &gitHubHost{client: client, owner: owner, repo: repo}, nil
}

func (g *gitHubHost) CreatePR(ctx context.Context, in CreatePRInput) (*PR, error) {
	draft := in.Draft
	req := &gogithub.NewPullRequest{
		Title: gogithub.String(in.Title),
		Body:  gogithub.String(in.Description),
		Head:  gogithub.String(in.SourceBranch),
		Base:  gogithub.String(in.TargetBranch),
		Draft: &draft,
	}
	pr, _, err := g.client.PullRequests.Create(ctx, g.owner, g.repo, req)
	if err != nil {
		return nil, fmt.Errorf("github CreatePR: %w", err)
	}
	// Request reviewers
	if len(in.Reviewers) > 0 {
		_, _, err = g.client.PullRequests.RequestReviewers(ctx, g.owner, g.repo, pr.GetNumber(), gogithub.ReviewersRequest{
			Reviewers: in.Reviewers,
		})
		if err != nil {
			// Non-fatal; log but continue
			fmt.Printf("warning: could not request reviewers: %v\n", err)
		}
	}
	return githubPRToHost(pr), nil
}

func (g *gitHubHost) GetPR(ctx context.Context, id string) (*PR, error) {
	num, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid PR id %q: %w", id, err)
	}
	pr, _, err := g.client.PullRequests.Get(ctx, g.owner, g.repo, num)
	if err != nil {
		return nil, fmt.Errorf("github GetPR: %w", err)
	}
	out := githubPRToHost(pr)

	// Enrich: reviews
	reviews, _, err := g.client.PullRequests.ListReviews(ctx, g.owner, g.repo, num, nil)
	if err == nil {
		seen := map[string]bool{}
		for _, r := range reviews {
			if r.GetState() == "APPROVED" {
				user := r.GetUser().GetLogin()
				if !seen[user] {
					out.Approvals = append(out.Approvals, user)
					seen[user] = true
				}
			}
		}
	}

	// CI state from commit status
	ciState, _ := g.GetBuildStatus(ctx, pr.GetHead().GetSHA())
	out.CIState = ciState

	// Last comment
	comments, _, err := g.client.Issues.ListComments(ctx, g.owner, g.repo, num, &gogithub.IssueListCommentsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	})
	if err == nil && len(comments) > 0 {
		last := comments[len(comments)-1]
		out.LastComment = &Comment{
			ID:        strconv.FormatInt(last.GetID(), 10),
			Author:    last.GetUser().GetLogin(),
			Body:      last.GetBody(),
			Timestamp: last.GetCreatedAt().Format(time.RFC3339),
		}
	}

	return out, nil
}

func (g *gitHubHost) ListPRs(ctx context.Context, state string, mine bool) ([]*PR, error) {
	opts := &gogithub.PullRequestListOptions{
		State:       strings.ToLower(state),
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}
	if mine {
		user, _, err := g.client.Users.Get(ctx, "")
		if err == nil {
			opts.Head = g.owner + ":" + user.GetLogin()
		}
	}
	prs, _, err := g.client.PullRequests.List(ctx, g.owner, g.repo, opts)
	if err != nil {
		return nil, fmt.Errorf("github ListPRs: %w", err)
	}
	out := make([]*PR, len(prs))
	for i, pr := range prs {
		out[i] = githubPRToHost(pr)
	}
	return out, nil
}

func (g *gitHubHost) MergePR(ctx context.Context, id string) (string, error) {
	num, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("invalid PR id %q: %w", id, err)
	}
	result, _, err := g.client.PullRequests.Merge(ctx, g.owner, g.repo, num, "", &gogithub.PullRequestOptions{
		MergeMethod: "merge",
	})
	if err != nil {
		return "", fmt.Errorf("github MergePR: %w", err)
	}
	return result.GetSHA(), nil
}

func (g *gitHubHost) CommentPR(ctx context.Context, id, text, _ string) error {
	num, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid PR id %q: %w", id, err)
	}
	_, _, err = g.client.Issues.CreateComment(ctx, g.owner, g.repo, num, &gogithub.IssueComment{
		Body: gogithub.String(text),
	})
	if err != nil {
		return fmt.Errorf("github CommentPR: %w", err)
	}
	return nil
}

func (g *gitHubHost) GetBuildStatus(ctx context.Context, sha string) (string, error) {
	status, _, err := g.client.Repositories.GetCombinedStatus(ctx, g.owner, g.repo, sha, nil)
	if err != nil {
		return "UNKNOWN", nil
	}
	switch status.GetState() {
	case "success":
		return "SUCCESSFUL", nil
	case "failure", "error":
		return "FAILED", nil
	case "pending":
		return "INPROGRESS", nil
	default:
		return "UNKNOWN", nil
	}
}

func (g *gitHubHost) GetReviewers(ctx context.Context, id string) ([]string, error) {
	num, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid PR id %q: %w", id, err)
	}
	pr, _, err := g.client.PullRequests.Get(ctx, g.owner, g.repo, num)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range pr.RequestedReviewers {
		out = append(out, r.GetLogin())
	}
	return out, nil
}

func (g *gitHubHost) UpdatePR(ctx context.Context, id, title, description string, addReviewers []string) error {
	num, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid PR id %q: %w", id, err)
	}
	update := &gogithub.PullRequest{}
	if title != "" {
		update.Title = gogithub.String(title)
	}
	if description != "" {
		update.Body = gogithub.String(description)
	}
	if title != "" || description != "" {
		if _, _, err := g.client.PullRequests.Edit(ctx, g.owner, g.repo, num, update); err != nil {
			return fmt.Errorf("github UpdatePR: %w", err)
		}
	}
	if len(addReviewers) > 0 {
		if _, _, err := g.client.PullRequests.RequestReviewers(ctx, g.owner, g.repo, num, gogithub.ReviewersRequest{
			Reviewers: addReviewers,
		}); err != nil {
			return fmt.Errorf("github RequestReviewers: %w", err)
		}
	}
	return nil
}

func (g *gitHubHost) CurrentUser(ctx context.Context) (string, error) {
	u, _, err := g.client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("github CurrentUser: %w", err)
	}
	return u.GetLogin(), nil
}

func githubPRToHost(pr *gogithub.PullRequest) *PR {
	state := "OPEN"
	switch pr.GetState() {
	case "closed":
		if pr.GetMerged() {
			state = "MERGED"
		} else {
			state = "DECLINED"
		}
	}
	var reviewers []string
	for _, r := range pr.RequestedReviewers {
		reviewers = append(reviewers, r.GetLogin())
	}
	return &PR{
		ID:           strconv.Itoa(pr.GetNumber()),
		URL:          pr.GetHTMLURL(),
		Title:        pr.GetTitle(),
		Description:  pr.GetBody(),
		State:        state,
		Draft:        pr.GetDraft(),
		SourceBranch: pr.GetHead().GetRef(),
		TargetBranch: pr.GetBase().GetRef(),
		HeadSHA:      pr.GetHead().GetSHA(),
		Reviewers:    reviewers,
	}
}
