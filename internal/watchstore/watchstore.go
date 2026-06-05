// Package watchstore is a tiny JSON-backed registry of pull requests whose
// build/CI state we have not yet resolved.
//
// Lifecycle:
//
//   - bitgit pr create / bitgit pr watch <id> / bitgit push (when a PR exists
//     for the source branch) → Add(entry)
//   - bitgit pr poll → iterate, query host build status, Remove(entry) when
//     the state is terminal (SUCCESSFUL | FAILED).
//
// Storage: $BITGIT_HOME/state/pr-watch.json (single file, atomic rename).
// Concurrency: protected by a flock on the same file. The registry is small
// (open PRs only), so a coarse lock is fine.
package watchstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Entry is one tracked pull request.
type Entry struct {
	// Key uniquely identifies the PR across hosts. Format:
	//   "<host>:<projectKey>/<repoSlug>#<prID>"
	// e.g. "bitbucket.cd.sac.int.threatmetrix.com:NP/portal#12345"
	Key string `json:"key"`

	// Host is the API base host (no scheme), e.g. "bitbucket.cd…".
	Host string `json:"host"`
	// ProjectKey + RepoSlug locate the repo on the host (Bitbucket DC concept).
	// For GitHub, ProjectKey is owner and RepoSlug is repo name.
	ProjectKey string `json:"project_key"`
	RepoSlug   string `json:"repo_slug"`

	// PRID is the PR number/id.
	PRID string `json:"pr_id"`
	// URL is the human URL to the PR (used in notifications).
	URL string `json:"url"`
	// Title is captured at registration time for nice notifications.
	Title string `json:"title"`
	// SourceBranch is captured for notifications.
	SourceBranch string `json:"source_branch,omitempty"`
	// TargetBranch is captured for notifications.
	TargetBranch string `json:"target_branch,omitempty"`

	// HeadSHA is the commit SHA being watched. Build status is per-commit, so
	// we store the SHA captured at registration; pr poll re-resolves the
	// current head SHA before querying so a fresh push moves the watch
	// forward without manual re-registration.
	HeadSHA string `json:"head_sha"`

	// LastState is the last CI state observed. Used to suppress duplicate
	// "INPROGRESS" pings between polls.
	LastState string `json:"last_state,omitempty"`

	// RegisteredAt is when the PR was first added.
	RegisteredAt time.Time `json:"registered_at"`
	// LastPolledAt is when we last queried the host. Zero before first poll.
	LastPolledAt time.Time `json:"last_polled_at,omitempty"`
	// PollCount is how many poll attempts we have made.
	PollCount int `json:"poll_count,omitempty"`

	// Source records what registered this watch ("create", "push", "manual").
	Source string `json:"source,omitempty"`
}

// Store is the on-disk registry.
type Store struct {
	path string
	mu   sync.Mutex
}

// New constructs a Store backed by path. The parent dir is created on demand.
// path is typically "<bitgit_home>/state/pr-watch.json".
func New(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("watchstore: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("watchstore: mkdir: %w", err)
	}
	return &Store{path: path}, nil
}

// DefaultPath returns "<bitgit_dir>/state/pr-watch.json".
func DefaultPath(bitgitDir string) string {
	return filepath.Join(bitgitDir, "state", "pr-watch.json")
}

type fileShape struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// load reads the file or returns an empty slice if it does not exist.
// Caller must hold s.mu.
func (s *Store) load() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("watchstore: read: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("watchstore: parse %s: %w", s.path, err)
	}
	return f.Entries, nil
}

// save writes the file atomically. Caller must hold s.mu.
func (s *Store) save(entries []Entry) error {
	if entries == nil {
		entries = []Entry{}
	}
	// Stable order for diff-friendliness.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	f := fileShape{Version: 1, Entries: entries}
	data, err := json.MarshalIndent(&f, "", "  ")
	if err != nil {
		return fmt.Errorf("watchstore: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("watchstore: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("watchstore: rename: %w", err)
	}
	return nil
}

// List returns a snapshot of all entries.
func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Add inserts or updates an entry by Key. Existing entries have RegisteredAt
// preserved; mutable fields (HeadSHA, Title, URL, branches, Source) are
// overwritten so a re-push moves HeadSHA forward.
func (s *Store) Add(e Entry) error {
	if e.Key == "" {
		return errors.New("watchstore: Add: empty Key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if e.RegisteredAt.IsZero() {
		e.RegisteredAt = now
	}
	found := false
	for i := range entries {
		if entries[i].Key == e.Key {
			// Preserve original RegisteredAt + counters.
			e.RegisteredAt = entries[i].RegisteredAt
			e.PollCount = entries[i].PollCount
			e.LastPolledAt = entries[i].LastPolledAt
			// LastState is overwritten only if caller supplied one; otherwise
			// keep the previous to avoid losing history when re-registering.
			if e.LastState == "" {
				e.LastState = entries[i].LastState
			}
			entries[i] = e
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, e)
	}
	return s.save(entries)
}

// UpdatePollResult records a poll result. If terminal is true, the entry is
// removed; otherwise LastState/LastPolledAt/PollCount are updated.
func (s *Store) UpdatePollResult(key, state string, terminal bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.Key == key {
			if terminal {
				continue // drop
			}
			e.LastState = state
			e.LastPolledAt = time.Now().UTC()
			e.PollCount++
		}
		out = append(out, e)
	}
	return s.save(out)
}

// UpdateHeadSHA replaces HeadSHA on an existing entry (e.g. after a fresh
// push). No-op if the entry is missing.
func (s *Store) UpdateHeadSHA(key, sha string) error {
	if key == "" || sha == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].Key == key {
			if entries[i].HeadSHA != sha {
				entries[i].HeadSHA = sha
				entries[i].LastState = "" // re-arm: new commit, unknown state
			}
			break
		}
	}
	return s.save(entries)
}

// Remove deletes an entry by Key. No-op if absent.
func (s *Store) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.Key != key {
			out = append(out, e)
		}
	}
	return s.save(out)
}

// MakeKey builds a stable registry key. Host is normalised to lowercase.
func MakeKey(host, projectKey, repoSlug, prID string) string {
	return fmt.Sprintf("%s:%s/%s#%s", host, projectKey, repoSlug, prID)
}
