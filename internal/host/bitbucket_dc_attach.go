// Bitbucket Data Center attachment implementation.
//
// Bitbucket DC has no public attachments REST API for PR comments. The only
// reliable way to surface binary files (screenshots, logs, PDFs) in a PR
// thread is to push them to a git ref the user can render and reference the
// raw URL from a comment.
//
// Strategy:
//  1. Create a one-commit orphan branch `attachments/pr-<id>` containing only
//     the uploaded files under `.attachments/`. Force-push every time so the
//     branch always reflects the latest uploads.
//  2. Return raw URLs of the form
//     {base}/projects/{P}/repos/{R}/raw/.attachments/{file}?at=refs/heads/attachments/pr-{id}
//     which Bitbucket renders inline in a comment when embedded as
//     ![alt](url). The user must be logged in to Bitbucket to see them; that
//     is the normal viewing case.
//
// We talk to the git server over HTTPS using the same Bearer token as the REST
// adapter via `git -c http.extraHeader=Authorization: Bearer <token>`.

package host

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var jiraKeyRE = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

func firstJiraKey(s string) string {
	return jiraKeyRE.FindString(s)
}

// UploadAttachments implements AttachmentUploader for Bitbucket DC.
func (b *bitbucketDCHost) UploadAttachments(ctx context.Context, prID string, paths []string) ([]Attachment, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%s is a directory; only files are supported", p)
		}
	}

	// Try to lift a Jira key (e.g. PORTAL-59788) off the PR for repos with a
	// pre-receive hook that requires it. Failure here is not fatal.
	jiraKey := ""
	if pr, err := b.GetPR(ctx, prID); err == nil && pr != nil {
		jiraKey = firstJiraKey(pr.Title + " " + pr.SourceBranch)
	}

	tmp, err := os.MkdirTemp("", "bitgit-attach-")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmp)

	branch := "attachments/pr-" + prID
	cloneURL := fmt.Sprintf("%s/scm/%s/%s.git", b.baseURL, strings.ToLower(b.projectKey), b.repoSlug)

	gitArgs := func(args ...string) []string {
		base := []string{}
		if b.token != "" {
			base = append(base, "-c", "http.extraHeader=Authorization: Bearer "+b.token)
		}
		return append(base, args...)
	}

	// Init a clean repo; do not clone (avoids dragging history).
	if out, err := runGit(ctx, tmp, gitArgs("init", "-q", "-b", branch)...); err != nil {
		return nil, fmt.Errorf("git init: %w (%s)", err, out)
	}
	if out, err := runGit(ctx, tmp, gitArgs("remote", "add", "origin", cloneURL)...); err != nil {
		return nil, fmt.Errorf("git remote add: %w (%s)", err, out)
	}

	attachDir := filepath.Join(tmp, ".attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		return nil, err
	}

	out := make([]Attachment, 0, len(paths))
	for _, p := range paths {
		name := filepath.Base(p)
		dst := filepath.Join(attachDir, name)
		if err := copyFile(p, dst); err != nil {
			return nil, fmt.Errorf("copy %s: %w", p, err)
		}
		raw := fmt.Sprintf("%s/projects/%s/repos/%s/raw/.attachments/%s?at=%s",
			b.baseURL,
			b.projectKey,
			b.repoSlug,
			url.PathEscape(name),
			url.QueryEscape("refs/heads/"+branch),
		)
		out = append(out, Attachment{Name: name, URL: raw})
	}

	if o, err := runGit(ctx, tmp, gitArgs("add", ".attachments")...); err != nil {
		return nil, fmt.Errorf("git add: %w (%s)", err, o)
	}
	commitMsg := "bitgit attachments for PR #" + prID
	if jiraKey != "" {
		commitMsg = jiraKey + " " + commitMsg
	}
	commitArgs := gitArgs(
		"-c", "user.name=bitgit",
		"-c", "user.email=bitgit@local",
		"commit", "-q", "--no-verify",
		"-m", commitMsg,
	)
	if o, err := runGit(ctx, tmp, commitArgs...); err != nil {
		return nil, fmt.Errorf("git commit: %w (%s)", err, o)
	}
	if o, err := runGit(ctx, tmp, gitArgs("push", "--force", "--no-verify", "origin", "HEAD:refs/heads/"+branch)...); err != nil {
		return nil, fmt.Errorf("git push: %w (%s)", err, o)
	}

	return out, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	b, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
