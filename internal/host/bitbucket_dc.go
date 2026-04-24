// Bitbucket Server / Data Center REST 1.0 adapter.
//
// QUIRKS documented here for future maintainers:
//
// 1. PAGINATION: Bitbucket DC uses offset-based pagination via `start` + `limit`
//    query parameters. Responses include `isLastPage` (bool) and `nextPageStart`
//    (int). Do NOT use cursor/token pagination.
//
// 2. SELF-SIGNED CERTS: On-prem instances frequently have self-signed TLS certs.
//    Set `insecure_skip_verify = true` in config.toml to bypass. We warn loudly
//    when this is enabled because it exposes you to MITM attacks.
//
// 3. BUILD STATUS: Build status is on a SEPARATE API endpoint:
//    /rest/build-status/1.0/commits/{sha} — NOT under /rest/api/1.0/...
//
// 4. BLOCKER COMMENTS: Fetched via the PR activities endpoint, filtered by
//    comment.severity == "BLOCKER" and state == "OPEN". These gate merges.
//
// 5. DECLINE + REOPEN: Bitbucket DC has a decline+reopen pattern sometimes used
//    to force a PR ref rescan. It is NOT reliable and is NOT implemented here.
//    See: https://jira.atlassian.com/browse/BSERV-10328
//
// 6. AUTH: Bearer token in Authorization header. File: ~/.bitgit/secrets/bitbucket.token
//    or env BITBUCKET_TOKEN.

package host

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/gitutil"
)

type bitbucketDCHost struct {
	baseURL    string // e.g. https://bb.example.com
	token      string
	projectKey string
	repoSlug   string
	httpClient *http.Client
}

func newBitbucketDC(remoteURL string, cfg *config.Config) (Host, error) {
	token, err := cfg.ReadToken("bitbucket-dc")
	if err != nil {
		return nil, fmt.Errorf("bitbucket-dc: read token: %w", err)
	}

	baseURL := extractBaseURL(remoteURL)
	projectKey, repoSlug := gitutil.ParseProjectSlugFromURL(remoteURL)

	transport := http.DefaultTransport
	if cfg.InsecureSkipVerify {
		fmt.Println("WARNING: insecure_skip_verify is enabled. TLS certificate validation is disabled. " +
			"This exposes you to man-in-the-middle attacks. Only use in controlled environments.")
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return &bitbucketDCHost{
		baseURL:    baseURL,
		token:      token,
		projectKey: projectKey,
		repoSlug:   repoSlug,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}, nil
}

// extractBaseURL extracts the scheme+host from a git remote URL.
func extractBaseURL(remoteURL string) string {
	if strings.Contains(remoteURL, "://") {
		u, err := url.Parse(remoteURL)
		if err != nil {
			return remoteURL
		}
		return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}
	// scp-like: git@host:path
	if at := strings.Index(remoteURL, "@"); at >= 0 {
		rest := remoteURL[at+1:]
		if colon := strings.Index(rest, ":"); colon > 0 {
			return "https://" + rest[:colon]
		}
	}
	return remoteURL
}

func (b *bitbucketDCHost) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	u := b.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return b.httpClient.Do(req)
}

func (b *bitbucketDCHost) decode(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket-dc: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// --- Bitbucket DC wire types ---

type bbPR struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"` // OPEN | MERGED | DECLINED | SUPERSEDED
	Draft       bool   `json:"draft"`
	Links       struct {
		Self []struct{ Href string } `json:"self"`
	} `json:"links"`
	FromRef struct {
		DisplayID    string `json:"displayId"`
		LatestCommit string `json:"latestCommit"`
		Repository   struct {
			Slug    string               `json:"slug"`
			Project struct{ Key string } `json:"project"`
		} `json:"repository"`
	} `json:"fromRef"`
	ToRef struct {
		DisplayID string `json:"displayId"`
	} `json:"toRef"`
	Reviewers []struct {
		User struct{ Slug string } `json:"user"`
		Role string                `json:"role"`
	} `json:"reviewers"`
}

type bbPageResponse[T any] struct {
	IsLastPage    bool `json:"isLastPage"`
	NextPageStart int  `json:"nextPageStart"`
	Values        []T  `json:"values"`
}

type bbActivity struct {
	Action  string `json:"action"` // COMMENTED, REVIEWED, etc.
	Comment *struct {
		ID       int    `json:"id"`
		Text     string `json:"text"`
		Severity string `json:"severity"` // NORMAL | BLOCKER
		State    string `json:"state"`    // OPEN | RESOLVED
		Author   struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"displayName"`
		} `json:"author"`
		CreatedDate int64 `json:"createdDate"` // epoch ms
	} `json:"comment,omitempty"`
}

type bbBuildStatus struct {
	State string `json:"state"` // SUCCESSFUL | FAILED | INPROGRESS
}

func (b *bitbucketDCHost) CreatePR(ctx context.Context, in CreatePRInput) (*PR, error) {
	reviewers := make([]map[string]any, len(in.Reviewers))
	for i, r := range in.Reviewers {
		reviewers[i] = map[string]any{"user": map[string]any{"name": r}}
	}
	payload := map[string]any{
		"title":       in.Title,
		"description": in.Description,
		"draft":       in.Draft,
		"fromRef": map[string]any{
			"id": "refs/heads/" + in.SourceBranch,
			"repository": map[string]any{
				"slug":    b.repoSlug,
				"project": map[string]any{"key": b.projectKey},
			},
		},
		"toRef": map[string]any{
			"id": "refs/heads/" + in.TargetBranch,
			"repository": map[string]any{
				"slug":    b.repoSlug,
				"project": map[string]any{"key": b.projectKey},
			},
		},
		"reviewers": reviewers,
	}
	body, err := jsonReader(payload)
	if err != nil {
		return nil, err
	}
	resp, err := b.do(ctx, http.MethodPost,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", b.projectKey, b.repoSlug),
		body)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-dc CreatePR: %w", err)
	}
	var pr bbPR
	if err := b.decode(resp, &pr); err != nil {
		return nil, err
	}
	return bbPRToHost(&pr), nil
}

func (b *bitbucketDCHost) GetPR(ctx context.Context, id string) (*PR, error) {
	resp, err := b.do(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s", b.projectKey, b.repoSlug, id),
		nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-dc GetPR: %w", err)
	}
	var pr bbPR
	if err := b.decode(resp, &pr); err != nil {
		return nil, err
	}
	out := bbPRToHost(&pr)

	// Approvals from reviewers with status APPROVED
	out.Approvals, err = b.getApprovals(ctx, id)
	if err != nil {
		out.Approvals = nil
	}

	// Blocker comments
	out.Blockers, out.LastComment, err = b.getBlockersAndLastComment(ctx, id)
	if err != nil {
		out.Blockers = nil
	}

	// CI state from build status
	if out.HeadSHA != "" {
		out.CIState, _ = b.GetBuildStatus(ctx, out.HeadSHA)
	}

	return out, nil
}

func (b *bitbucketDCHost) getApprovals(ctx context.Context, prID string) ([]string, error) {
	resp, err := b.do(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s", b.projectKey, b.repoSlug, prID),
		nil)
	if err != nil {
		return nil, err
	}
	var pr bbPR
	if err := b.decode(resp, &pr); err != nil {
		return nil, err
	}
	var approvals []string
	for _, r := range pr.Reviewers {
		if r.Role == "APPROVED" {
			approvals = append(approvals, r.User.Slug)
		}
	}
	return approvals, nil
}

// getBlockersAndLastComment fetches activities for blocker comments and last comment.
func (b *bitbucketDCHost) getBlockersAndLastComment(ctx context.Context, prID string) ([]string, *Comment, error) {
	start := 0
	var blockers []string
	var lastComment *Comment
	for {
		path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s/activities?start=%d&limit=100",
			b.projectKey, b.repoSlug, prID, start)
		resp, err := b.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, nil, err
		}
		var page bbPageResponse[bbActivity]
		if err := b.decode(resp, &page); err != nil {
			return nil, nil, err
		}
		for _, act := range page.Values {
			if act.Action == "COMMENTED" && act.Comment != nil {
				c := act.Comment
				// Track last comment
				if lastComment == nil || c.CreatedDate > parseCommentTime(lastComment.Timestamp) {
					lastComment = &Comment{
						ID:        strconv.Itoa(c.ID),
						Author:    c.Author.DisplayName,
						Body:      c.Text,
						Timestamp: msToRFC3339(c.CreatedDate),
					}
				}
				// Collect blocker comments
				if c.Severity == "BLOCKER" && c.State == "OPEN" {
					blockers = append(blockers, strconv.Itoa(c.ID))
				}
			}
		}
		if page.IsLastPage {
			break
		}
		start = page.NextPageStart
	}
	return blockers, lastComment, nil
}

func (b *bitbucketDCHost) ListPRs(ctx context.Context, state string, mine bool) ([]*PR, error) {
	var all []*PR
	start := 0
	for {
		path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=%s&start=%d&limit=100",
			b.projectKey, b.repoSlug, strings.ToUpper(state), start)
		resp, err := b.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-dc ListPRs: %w", err)
		}
		var page bbPageResponse[bbPR]
		if err := b.decode(resp, &page); err != nil {
			return nil, err
		}
		for i := range page.Values {
			pr := bbPRToHost(&page.Values[i])
			_ = mine // TODO: filter by current user when CurrentUser() is reliable
			all = append(all, pr)
		}
		if page.IsLastPage {
			break
		}
		start = page.NextPageStart
	}
	return all, nil
}

func (b *bitbucketDCHost) MergePR(ctx context.Context, id string) (string, error) {
	// Bitbucket DC merge requires the PR version in the URL
	prResp, err := b.do(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s", b.projectKey, b.repoSlug, id),
		nil)
	if err != nil {
		return "", fmt.Errorf("bitbucket-dc MergePR get version: %w", err)
	}
	var prData struct {
		Version int `json:"version"`
	}
	if err := b.decode(prResp, &prData); err != nil {
		return "", err
	}
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s/merge?version=%d",
		b.projectKey, b.repoSlug, id, prData.Version)
	resp, err := b.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return "", fmt.Errorf("bitbucket-dc MergePR: %w", err)
	}
	var result struct {
		Properties struct {
			MergeCommit struct {
				ID string `json:"id"`
			} `json:"mergeCommit"`
		} `json:"properties"`
	}
	if err := b.decode(resp, &result); err != nil {
		return "", err
	}
	sha := result.Properties.MergeCommit.ID
	if len(sha) > 12 {
		return sha[:12], nil
	}
	return sha, nil
}

func (b *bitbucketDCHost) CommentPR(ctx context.Context, id, text, replyTo string) error {
	payload := map[string]any{"text": text}
	if replyTo != "" {
		replyID, _ := strconv.Atoi(replyTo)
		payload["parent"] = map[string]any{"id": replyID}
	}
	body, err := jsonReader(payload)
	if err != nil {
		return err
	}
	resp, err := b.do(ctx, http.MethodPost,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s/comments", b.projectKey, b.repoSlug, id),
		body)
	if err != nil {
		return fmt.Errorf("bitbucket-dc CommentPR: %w", err)
	}
	return b.decode(resp, nil)
}

func (b *bitbucketDCHost) ListComments(ctx context.Context, prID string) ([]*Comment, error) {
	start := 0
	var out []*Comment
	for {
		path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s/activities?start=%d&limit=100",
			b.projectKey, b.repoSlug, prID, start)
		resp, err := b.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-dc ListComments: %w", err)
		}
		var page bbPageResponse[bbActivity]
		if err := b.decode(resp, &page); err != nil {
			return nil, err
		}
		for _, act := range page.Values {
			if act.Action == "COMMENTED" && act.Comment != nil {
				c := act.Comment
				out = append(out, &Comment{
					ID:        strconv.Itoa(c.ID),
					Author:    c.Author.DisplayName,
					Body:      c.Text,
					Timestamp: msToRFC3339(c.CreatedDate),
				})
			}
		}
		if page.IsLastPage {
			break
		}
		start = page.NextPageStart
	}
	return out, nil
}

func (b *bitbucketDCHost) GetBuildStatus(ctx context.Context, sha string) (string, error) {
	// Build status API is at /rest/build-status/1.0/commits/{sha} — separate from /rest/api/1.0
	resp, err := b.do(ctx, http.MethodGet,
		fmt.Sprintf("/rest/build-status/1.0/commits/%s", sha),
		nil)
	if err != nil {
		return "UNKNOWN", nil
	}
	var page bbPageResponse[bbBuildStatus]
	if err := b.decode(resp, &page); err != nil {
		return "UNKNOWN", nil
	}
	// Aggregate: any FAILED → FAILED, all SUCCESSFUL → SUCCESSFUL, else INPROGRESS
	if len(page.Values) == 0 {
		return "UNKNOWN", nil
	}
	overall := "SUCCESSFUL"
	for _, s := range page.Values {
		switch s.State {
		case "FAILED":
			return "FAILED", nil
		case "INPROGRESS":
			overall = "INPROGRESS"
		}
	}
	return overall, nil
}

func (b *bitbucketDCHost) GetReviewers(ctx context.Context, id string) ([]string, error) {
	pr, err := b.GetPR(ctx, id)
	if err != nil {
		return nil, err
	}
	return pr.Reviewers, nil
}

func (b *bitbucketDCHost) UpdatePR(ctx context.Context, id, title, description string, addReviewers []string) error {
	// First, get current PR to obtain version + reviewers
	resp, err := b.do(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s", b.projectKey, b.repoSlug, id),
		nil)
	if err != nil {
		return fmt.Errorf("bitbucket-dc UpdatePR get: %w", err)
	}
	var prData struct {
		ID          int    `json:"id"`
		Version     int    `json:"version"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Reviewers   []struct {
			User struct{ Slug string } `json:"user"`
		} `json:"reviewers"`
		FromRef map[string]any `json:"fromRef"`
		ToRef   map[string]any `json:"toRef"`
	}
	if err := b.decode(resp, &prData); err != nil {
		return err
	}

	// Merge reviewers
	reviewerSlugs := map[string]bool{}
	for _, r := range prData.Reviewers {
		reviewerSlugs[r.User.Slug] = true
	}
	for _, r := range addReviewers {
		reviewerSlugs[r] = true
	}
	reviewers := make([]map[string]any, 0, len(reviewerSlugs))
	for slug := range reviewerSlugs {
		reviewers = append(reviewers, map[string]any{"user": map[string]any{"name": slug}})
	}

	if title == "" {
		title = prData.Title
	}
	if description == "" {
		description = prData.Description
	}

	payload := map[string]any{
		"id":          prData.ID,
		"version":     prData.Version,
		"title":       title,
		"description": description,
		"reviewers":   reviewers,
		"fromRef":     prData.FromRef,
		"toRef":       prData.ToRef,
	}
	body, err := jsonReader(payload)
	if err != nil {
		return err
	}
	putResp, err := b.do(ctx, http.MethodPut,
		fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%s", b.projectKey, b.repoSlug, id),
		body)
	if err != nil {
		return fmt.Errorf("bitbucket-dc UpdatePR: %w", err)
	}
	return b.decode(putResp, nil)
}

func (b *bitbucketDCHost) CurrentUser(ctx context.Context) (string, error) {
	resp, err := b.do(ctx, http.MethodGet, "/rest/api/1.0/application-properties", nil)
	if err != nil {
		return "", fmt.Errorf("bitbucket-dc connectivity check: %w", err)
	}
	defer resp.Body.Close()
	// We don't get user from app-properties; use a different endpoint
	resp2, err := b.do(ctx, http.MethodGet, "/rest/api/1.0/users?filter=", nil)
	if err != nil {
		return "", nil // non-fatal
	}
	var page bbPageResponse[struct {
		Slug string `json:"slug"`
	}]
	if err := b.decode(resp2, &page); err != nil {
		return "", nil
	}
	// Can't reliably determine "me" without /api/1.0/application-properties + session
	// Best effort: return empty and let callers handle
	_ = resp
	return "", nil
}

func bbPRToHost(pr *bbPR) *PR {
	var url string
	if len(pr.Links.Self) > 0 {
		url = pr.Links.Self[0].Href
	}
	var reviewers []string
	for _, r := range pr.Reviewers {
		reviewers = append(reviewers, r.User.Slug)
	}
	sha := pr.FromRef.LatestCommit
	if len(sha) > 12 {
		sha = sha[:12]
	}
	return &PR{
		ID:           strconv.Itoa(pr.ID),
		URL:          url,
		Title:        pr.Title,
		Description:  pr.Description,
		State:        pr.State,
		Draft:        pr.Draft,
		SourceBranch: pr.FromRef.DisplayID,
		TargetBranch: pr.ToRef.DisplayID,
		HeadSHA:      sha,
		Reviewers:    reviewers,
	}
}

func msToRFC3339(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func parseCommentTime(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func jsonReader(v any) (io.Reader, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(string(data)), nil
}
