// Package gitutil provides helpers for running git commands and parsing output.
package gitutil

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Run executes a git command and returns trimmed stdout.
func Run(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the current branch name.
func CurrentBranch() (string, error) {
	return Run("rev-parse", "--abbrev-ref", "HEAD")
}

// HeadSHA returns the full HEAD SHA.
func HeadSHA() (string, error) {
	return Run("rev-parse", "HEAD")
}

// HeadSHAShort returns a 12-char abbreviated HEAD SHA.
func HeadSHAShort() (string, error) {
	sha, err := HeadSHA()
	if err != nil {
		return "", err
	}
	if len(sha) > 12 {
		return sha[:12], nil
	}
	return sha, nil
}

// HeadParents returns abbreviated SHAs of HEAD's parent commits.
func HeadParents() ([]string, error) {
	out, err := Run("log", "--pretty=%P", "-1")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	parts := strings.Fields(out)
	shorts := make([]string, len(parts))
	for i, p := range parts {
		if len(p) > 12 {
			shorts[i] = p[:12]
		} else {
			shorts[i] = p
		}
	}
	return shorts, nil
}

// RemoteURL returns the URL for the given remote.
func RemoteURL(remote string) (string, error) {
	return Run("remote", "get-url", remote)
}

// StagedFiles returns the list of files in the index (staged for commit).
func StagedFiles() ([]string, error) {
	out, err := Run("diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// DiffStats returns insertions/deletions for staged changes.
type DiffStat struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// StagedDiffStats parses `git diff --cached --numstat` output.
func StagedDiffStats() (DiffStat, error) {
	out, err := Run("diff", "--cached", "--numstat")
	if err != nil {
		return DiffStat{}, err
	}
	var s DiffStat
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		s.FilesChanged++
		if parts[0] != "-" {
			n, _ := strconv.Atoi(parts[0])
			s.Insertions += n
		}
		if parts[1] != "-" {
			n, _ := strconv.Atoi(parts[1])
			s.Deletions += n
		}
	}
	return s, nil
}

// CommitsAhead returns the number of commits ahead of upstream for the given branch.
func CommitsAhead(branch, remote string) (int, error) {
	upstream := remote + "/" + branch
	out, err := Run("rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		// No upstream tracking — return 0 without error
		return 0, nil
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// IsGitRepo returns true if the current directory is inside a git repo.
func IsGitRepo() bool {
	_, err := Run("rev-parse", "--git-dir")
	return err == nil
}

// IsShallowRepo returns true if the repo is shallow.
func IsShallowRepo() bool {
	out, err := Run("rev-list", "--count", "HEAD", "--max-count=51")
	if err != nil {
		return false
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n <= 50
}

// Refspecs builds the default push refspec for a branch.
func Refspecs(branch string) []string {
	return []string{fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)}
}

// ParseProjectSlugFromURL extracts Bitbucket DC project key and repo slug.
// Input: https://bb.example.com/scm/PLAT/api.git
// Returns: ("PLAT", "api")
func ParseProjectSlugFromURL(remoteURL string) (projectKey, repoSlug string) {
	// Normalise
	u := remoteURL
	// Remove .git suffix
	u = strings.TrimSuffix(u, ".git")

	// Handle https://host/scm/PROJECT/REPO
	if idx := strings.Index(u, "/scm/"); idx >= 0 {
		rest := u[idx+5:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return strings.ToUpper(parts[0]), parts[1]
		}
	}

	// Handle git@host:PROJECT/REPO or https://host/PROJECT/REPO (GitHub style)
	if idx := strings.Index(u, ":"); idx >= 0 && !strings.Contains(u[:idx], "/") {
		rest := u[idx+1:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}

	// GitHub-style https
	if idx := strings.LastIndex(u, "/"); idx >= 0 {
		repo := u[idx+1:]
		before := u[:idx]
		if idx2 := strings.LastIndex(before, "/"); idx2 >= 0 {
			owner := before[idx2+1:]
			return owner, repo
		}
	}
	return "", ""
}
