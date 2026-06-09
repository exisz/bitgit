package cli

import (
	"context"
	"testing"
	"time"

	"github.com/exisz/bitgit/internal/config"
	"github.com/exisz/bitgit/internal/watchstore"
)

// TestPollUntilDrainedExitsWhenEmpty verifies the inline loop returns
// immediately when the registry is empty (no host calls, no sleep).
func TestPollUntilDrainedExitsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()
	defer config.Reset()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Override sleep to fail loudly if invoked — empty registry must not sleep.
	prev := pollSleep
	pollSleep = func(_ context.Context, _ time.Duration) {
		t.Fatal("pollSleep called on empty registry")
	}
	defer func() { pollSleep = prev }()

	done := make(chan struct{})
	go func() {
		pollUntilDrained(context.Background(), cfg)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollUntilDrained hung on empty registry")
	}
}

// TestPollUntilDrainedRespectsContextCancel verifies a cancelled context
// breaks the loop even when entries are present (so SIGINT exits cleanly
// without draining).
func TestPollUntilDrainedRespectsContextCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()
	defer config.Reset()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Seed an entry so the registry is non-empty. Use a bogus host so any
	// poll attempt fails fast (pollEntries records the error and continues).
	s, err := watchstore.New(watchstore.DefaultPath(cfg.Dir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(watchstore.Entry{
		Key:        watchstore.MakeKey("nope.invalid", "PROJ", "repo", "1"),
		Host:       "nope.invalid",
		ProjectKey: "PROJ",
		RepoSlug:   "repo",
		PRID:       "1",
	}); err != nil {
		t.Fatal(err)
	}

	// Make sleep cancellable but slow enough that ctx cancel wins.
	prev := pollSleep
	pollSleep = func(ctx context.Context, _ time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	defer func() { pollSleep = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pollUntilDrained(ctx, cfg)
		close(done)
	}()

	// Give one cycle a chance, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollUntilDrained did not honor context cancel")
	}
}
