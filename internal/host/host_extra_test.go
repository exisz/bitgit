package host_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/host"
)

func TestBitbucketDC_MergePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pull-requests/5") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/merge"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 5, "version": 3, "title": "test",
				"fromRef":   map[string]any{"displayId": "feature/x", "latestCommit": "abc", "repository": map[string]any{"slug": "api", "project": map[string]any{"key": "PLAT"}}},
				"toRef":     map[string]any{"displayId": "main"},
				"reviewers": []any{},
				"links":     map[string]any{"self": []any{}},
			})
		case strings.Contains(r.URL.Path, "/merge") && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"properties": map[string]any{
					"mergeCommit": map[string]any{"id": "deadbeef123456789"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	sha, err := h.MergePR(context.Background(), "5")
	if err != nil {
		t.Fatal(err)
	}
	if sha != "deadbeef1234" {
		t.Errorf("expected deadbeef1234, got %s", sha)
	}
}

func TestBitbucketDC_CommentPR(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/comments") && r.Method == http.MethodPost {
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": 50})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	err := h.CommentPR(context.Background(), "7", "looks good", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["text"] != "looks good" {
		t.Errorf("expected text=looks good, got %v", gotBody["text"])
	}
}

func TestBitbucketDC_CommentPR_WithReply(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/comments") && r.Method == http.MethodPost {
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": 51})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	err := h.CommentPR(context.Background(), "7", "reply text", "200")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["text"] != "reply text" {
		t.Errorf("expected text=reply text")
	}
	parent, ok := gotBody["parent"].(map[string]any)
	if !ok || parent["id"] == nil {
		t.Errorf("expected parent.id in body, got %v", gotBody)
	}
}

func TestBitbucketDC_GetReviewers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pull-requests/3") && !strings.Contains(r.URL.Path, "/activities") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"id": 3, "version": 0, "title": "t",
				"fromRef": map[string]any{"displayId": "f", "latestCommit": "abc", "repository": map[string]any{"slug": "api", "project": map[string]any{"key": "P"}}},
				"toRef":   map[string]any{"displayId": "m"},
				"reviewers": []any{
					map[string]any{"user": map[string]any{"slug": "alice"}, "role": "REVIEWER"},
					map[string]any{"user": map[string]any{"slug": "bob"}, "role": "REVIEWER"},
				},
				"links": map[string]any{"self": []any{}},
			})
		case strings.Contains(r.URL.Path, "/activities"):
			json.NewEncoder(w).Encode(map[string]any{"isLastPage": true, "values": []any{}})
		case strings.Contains(r.URL.Path, "build-status"):
			json.NewEncoder(w).Encode(map[string]any{"isLastPage": true, "values": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	reviewers, err := h.GetReviewers(context.Background(), "3")
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewers) != 2 {
		t.Errorf("expected 2 reviewers, got %d: %v", len(reviewers), reviewers)
	}
}

func TestBitbucketDC_UpdatePR(t *testing.T) {
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/pull-requests/8") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"id": 8, "version": 2, "title": "old title", "description": "old desc",
				"fromRef":   map[string]any{"displayId": "f", "latestCommit": "abc", "repository": map[string]any{"slug": "api", "project": map[string]any{"key": "P"}}},
				"toRef":     map[string]any{"displayId": "m"},
				"reviewers": []any{map[string]any{"user": map[string]any{"slug": "alice"}}},
				"links":     map[string]any{"self": []any{}},
			})
		case strings.HasSuffix(path, "/pull-requests/8") && r.Method == http.MethodPut:
			json.NewDecoder(r.Body).Decode(&putBody)
			json.NewEncoder(w).Encode(map[string]any{"id": 8})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := hostFromTestServer(t, srv.URL)
	err := h.UpdatePR(context.Background(), "8", "new title", "new desc", []string{"bob"})
	if err != nil {
		t.Fatal(err)
	}
	if putBody["title"] != "new title" {
		t.Errorf("expected title=new title, got %v", putBody["title"])
	}
	// Verify alice + bob both in reviewers
	reviewers, _ := putBody["reviewers"].([]any)
	if len(reviewers) < 2 {
		t.Errorf("expected 2 reviewers (alice + bob), got %d", len(reviewers))
	}
}

func TestBitbucketDC_Detect(t *testing.T) {
	// Test Detect() routing
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	t.Setenv("BITBUCKET_TOKEN", "tok")
	config.Reset()

	cfg, _ := config.Load()

	// github.com → github host (will fail on owner/repo parsing without a real remote)
	// Skip this test to avoid hanging git subprocess
	// _, err := host.Detect("https://github.com/owner/repo.git", cfg)
	// _ = err

	// bitbucket.org → not supported
	_, err := host.Detect("https://bitbucket.org/owner/repo.git", cfg)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected not-supported error for bitbucket.org, got %v", err)
	}

	// unknown host → bitbucket_dc
	_, err = host.Detect("https://bb.internal.example.com/scm/PROJ/repo.git", cfg)
	if err != nil {
		t.Errorf("unexpected error for DC host: %v", err)
	}

	// empty URL
	_, err = host.Detect("", cfg)
	if err == nil {
		t.Error("expected error for empty remote URL")
	}
}
