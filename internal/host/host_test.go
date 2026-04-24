package host_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/host"
)

// bbPRFixture returns a minimal Bitbucket DC PR JSON body.
func bbPRFixture(id int, state string) map[string]any {
	return map[string]any{
		"id":          id,
		"version":     0,
		"title":       "test PR",
		"description": "desc",
		"state":       state,
		"draft":       false,
		"links": map[string]any{
			"self": []map[string]any{{"href": "https://bb.test/pr/" + strconv.Itoa(id)}},
		},
		"fromRef": map[string]any{
			"displayId":    "feature/x",
			"latestCommit": "abc1234567890",
			"repository": map[string]any{
				"slug":    "api",
				"project": map[string]any{"key": "PLAT"},
			},
		},
		"toRef": map[string]any{
			"displayId": "main",
		},
		"reviewers": []any{},
	}
}

func TestBitbucketDC_GetPR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pull-requests/42") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(bbPRFixture(42, "OPEN"))
		case strings.Contains(r.URL.Path, "/activities"):
			json.NewEncoder(w).Encode(map[string]any{"isLastPage": true, "values": []any{}})
		case strings.Contains(r.URL.Path, "build-status"):
			json.NewEncoder(w).Encode(map[string]any{"isLastPage": true, "values": []any{
				map[string]any{"state": "SUCCESSFUL"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	pr, err := h.GetPR(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if pr.ID != "42" {
		t.Errorf("expected ID=42, got %s", pr.ID)
	}
	if pr.State != "OPEN" {
		t.Errorf("expected state=OPEN, got %s", pr.State)
	}
	if pr.CIState != "SUCCESSFUL" {
		t.Errorf("expected CIState=SUCCESSFUL, got %s", pr.CIState)
	}
}

func TestBitbucketDC_ListPRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pull-requests") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/42") {
			json.NewEncoder(w).Encode(map[string]any{
				"isLastPage": true,
				"values":     []any{bbPRFixture(1, "OPEN"), bbPRFixture(2, "OPEN")},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	prs, err := h.ListPRs(context.Background(), "OPEN", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Errorf("expected 2 PRs, got %d", len(prs))
	}
}

func TestBitbucketDC_CreatePR(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pull-requests") && r.Method == http.MethodPost {
			json.NewDecoder(r.Body).Decode(&gotBody)
			json.NewEncoder(w).Encode(bbPRFixture(99, "OPEN"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	pr, err := h.CreatePR(context.Background(), host.CreatePRInput{
		Title:        "feat: new thing",
		Description:  "does stuff",
		SourceBranch: "feature/x",
		TargetBranch: "main",
		Reviewers:    []string{"alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.ID != "99" {
		t.Errorf("expected ID=99, got %s", pr.ID)
	}
	if gotBody["title"] != "feat: new thing" {
		t.Errorf("unexpected title in request body: %v", gotBody["title"])
	}
}

func TestBitbucketDC_BlockerComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pull-requests/10") && r.Method == http.MethodGet && !strings.Contains(r.URL.RawQuery, "activities"):
			json.NewEncoder(w).Encode(bbPRFixture(10, "OPEN"))
		case strings.Contains(r.URL.Path, "/activities"):
			json.NewEncoder(w).Encode(map[string]any{
				"isLastPage": true,
				"values": []any{
					map[string]any{
						"action": "COMMENTED",
						"comment": map[string]any{
							"id": 101, "text": "fix this", "severity": "BLOCKER", "state": "OPEN",
							"author":      map[string]any{"slug": "reviewer1", "displayName": "Reviewer One"},
							"createdDate": 1700000000000,
						},
					},
				},
			})
		case strings.Contains(r.URL.Path, "build-status"):
			json.NewEncoder(w).Encode(map[string]any{"isLastPage": true, "values": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	pr, err := h.GetPR(context.Background(), "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.Blockers) != 1 || pr.Blockers[0] != "101" {
		t.Errorf("expected 1 blocker with id=101, got %v", pr.Blockers)
	}
	if pr.LastComment == nil {
		t.Error("expected last comment, got nil")
	} else if pr.LastComment.Author != "Reviewer One" {
		t.Errorf("expected author=Reviewer One, got %s", pr.LastComment.Author)
	}
}

func TestBitbucketDC_GetBuildStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "build-status") {
			json.NewEncoder(w).Encode(map[string]any{
				"isLastPage": true,
				"values":     []any{map[string]any{"state": "FAILED"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	state, err := h.GetBuildStatus(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if state != "FAILED" {
		t.Errorf("expected FAILED, got %s", state)
	}
}

// hostFromTestServer creates a Bitbucket DC host pointing at the test server.
func hostFromTestServer(t *testing.T, serverURL string) host.Host {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	t.Setenv("BITBUCKET_TOKEN", "test-token")
	config.Reset()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Patch the remote URL so Detect picks up the test server
	remoteURL := serverURL + "/scm/PLAT/api.git"
	h, err := host.NewBitbucketDCForTest(remoteURL, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Ensure we can read token from env without a file.
func TestReadTokenFromEnv_Bitbucket(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	t.Setenv("BITBUCKET_TOKEN", "bb-token-123")
	config.Reset()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := cfg.ReadToken("bitbucket-dc")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "bb-token-123" {
		t.Errorf("expected bb-token-123, got %q", tok)
	}
	os.Unsetenv("BITBUCKET_TOKEN")
}
