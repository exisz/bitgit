package reviewer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/host"
	"github.com/exisz/bitgit/internal/reviewer"
)

// ---------------------------------------------------------------------------
// fakeHost
// ---------------------------------------------------------------------------

type fakeHost struct {
	mergedPRs []*host.PR
	listErr   error
}

func (f *fakeHost) CreatePR(_ context.Context, _ host.CreatePRInput) (*host.PR, error) {
	return nil, nil
}
func (f *fakeHost) GetPR(_ context.Context, _ string) (*host.PR, error) { return nil, nil }
func (f *fakeHost) ListPRs(_ context.Context, state string, _ bool) ([]*host.PR, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if state == "MERGED" {
		return f.mergedPRs, nil
	}
	return nil, nil
}
func (f *fakeHost) MergePR(_ context.Context, _ string) (string, error)    { return "", nil }
func (f *fakeHost) CommentPR(_ context.Context, _, _, _ string) error       { return nil }
func (f *fakeHost) ListComments(_ context.Context, _ string) ([]*host.Comment, error) {
	return nil, nil
}
func (f *fakeHost) GetBuildStatus(_ context.Context, _ string) (string, error) { return "", nil }
func (f *fakeHost) GetReviewers(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (f *fakeHost) UpdatePR(_ context.Context, _, _, _ string, _ []string) error {
	return nil
}
func (f *fakeHost) CurrentUser(_ context.Context) (string, error) { return "", nil }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func cfg(team []string, includeRecent bool, recentLimit int) *config.Config {
	config.Reset()
	c := &config.Config{}
	c.Reviewers.Team = team
	c.Reviewers.IncludeRecent = includeRecent
	c.Reviewers.RecentLimit = recentLimit
	return c
}

func merged(reviewers ...string) *host.PR {
	return &host.PR{State: "MERGED", Reviewers: reviewers}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestResolveReviewers_TeamOnly(t *testing.T) {
	c := cfg([]string{"alice", "bob"}, false, 0)
	h := &fakeHost{}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "bob"}
	assertEqual(t, want, got)
}

func TestResolveReviewers_ExplicitOnly(t *testing.T) {
	c := cfg(nil, false, 0)
	h := &fakeHost{}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, []string{"charlie", "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "charlie"} // sorted
	assertEqual(t, want, got)
}

func TestResolveReviewers_TeamPlusExplicit(t *testing.T) {
	c := cfg([]string{"alice", "bob"}, false, 0)
	h := &fakeHost{}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, []string{"charlie"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "bob", "charlie"}
	assertEqual(t, want, got)
}

func TestResolveReviewers_Deduplication(t *testing.T) {
	c := cfg([]string{"alice", "bob"}, false, 0)
	h := &fakeHost{}

	// "alice" appears in both team and explicit
	got, err := reviewer.ResolveReviewers(context.Background(), c, h, []string{"alice", "charlie"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "bob", "charlie"}
	assertEqual(t, want, got)
}

func TestResolveReviewers_IncludeRecent(t *testing.T) {
	c := cfg([]string{"alice"}, true, 1)
	h := &fakeHost{
		mergedPRs: []*host.PR{
			merged("bob", "charlie"),
			merged("dave"), // should be ignored — limit=1
		},
	}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "bob", "charlie"}
	assertEqual(t, want, got)
}

func TestResolveReviewers_IncludeRecentMultiple(t *testing.T) {
	c := cfg(nil, true, 2)
	h := &fakeHost{
		mergedPRs: []*host.PR{
			merged("alice", "bob"),
			merged("charlie"),
			merged("dave"), // ignored — limit=2
		},
	}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice", "bob", "charlie"}
	assertEqual(t, want, got)
}

func TestResolveReviewers_EmptyConfig(t *testing.T) {
	c := cfg(nil, false, 0)
	h := &fakeHost{}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestResolveReviewers_ListPRsError(t *testing.T) {
	c := cfg(nil, true, 1)
	h := &fakeHost{listErr: errors.New("network error")}

	_, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveReviewers_RecentLimitZeroDefaultsToOne(t *testing.T) {
	c := cfg(nil, true, 0) // 0 → should default to 1
	h := &fakeHost{
		mergedPRs: []*host.PR{
			merged("alice"),
			merged("bob"), // should be excluded
		},
	}

	got, err := reviewer.ResolveReviewers(context.Background(), c, h, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alice"}
	assertEqual(t, want, got)
}

// ---------------------------------------------------------------------------
// assert helper
// ---------------------------------------------------------------------------

func assertEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("index %d: want %q, got %q (full: want %v, got %v)", i, want[i], got[i], want, got)
		}
	}
}
