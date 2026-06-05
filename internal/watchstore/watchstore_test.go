package watchstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "state", "pr-watch.json"))
	if err != nil {
		t.Fatal(err)
	}

	// empty list
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}

	e := Entry{
		Key:          "h:P/r#1",
		Host:         "h",
		ProjectKey:   "P",
		RepoSlug:     "r",
		PRID:         "1",
		URL:          "https://h/pr/1",
		Title:        "first",
		HeadSHA:      "abc123",
		Source:       "manual",
		RegisteredAt: time.Now().UTC(),
	}
	if err := s.Add(e); err != nil {
		t.Fatal(err)
	}
	got, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != e.Key || got[0].HeadSHA != "abc123" {
		t.Fatalf("unexpected: %+v", got)
	}

	// Add same key with new SHA — RegisteredAt preserved, HeadSHA updated.
	original := got[0].RegisteredAt
	e2 := e
	e2.HeadSHA = "def456"
	e2.Title = "second"
	e2.RegisteredAt = time.Time{}
	if err := s.Add(e2); err != nil {
		t.Fatal(err)
	}
	got, _ = s.List()
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if got[0].HeadSHA != "def456" || got[0].Title != "second" {
		t.Fatalf("update lost: %+v", got[0])
	}
	if !got[0].RegisteredAt.Equal(original) {
		t.Fatalf("RegisteredAt should be preserved")
	}

	// UpdateHeadSHA on existing key arms LastState to ""
	if err := s.UpdatePollResult(e.Key, "INPROGRESS", false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.List()
	if got[0].LastState != "INPROGRESS" || got[0].PollCount != 1 {
		t.Fatalf("UpdatePollResult: %+v", got[0])
	}
	if err := s.UpdateHeadSHA(e.Key, "newsha"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.List()
	if got[0].HeadSHA != "newsha" || got[0].LastState != "" {
		t.Fatalf("UpdateHeadSHA: %+v", got[0])
	}

	// Terminal poll removes
	if err := s.UpdatePollResult(e.Key, "SUCCESSFUL", true); err != nil {
		t.Fatal(err)
	}
	got, _ = s.List()
	if len(got) != 0 {
		t.Fatalf("terminal should drop, got %+v", got)
	}
}

func TestMakeKey(t *testing.T) {
	got := MakeKey("h.example.com", "PROJ", "repo", "42")
	want := "h.example.com:PROJ/repo#42"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
