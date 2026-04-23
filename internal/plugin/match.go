package plugin

import (
	"net/url"
	"regexp"
	"strings"
)

// Context describes the operation a hook is being evaluated for.
// Plugins receive this; the matcher uses it to decide attachment.
type Context struct {
	RemoteURL  string // e.g. "https://bitbucket.example.com/scm/PLAT/api.git"
	ProjectKey string // e.g. "PLAT" — caller-resolved when known
	RepoSlug   string // e.g. "api"
	Branch     string // current branch
	Hook       string // e.g. "pre-pr-create"
}

// Matches reports whether m attaches to ctx.
//
// Rules:
//   - Empty Match → always attaches (universal plugin).
//   - Otherwise the union of any non-empty rule set must contain a match.
//   - Multiple rule kinds (host + project + branch) are OR-ed: any hit attaches.
//
// This is the Empire convention; plugins MAY layer their own ANDs internally.
func (m Manifest) Matches(ctx Context) bool {
	r := m.Match
	if isEmpty(r) {
		return true
	}

	host, _ := remoteHost(ctx.RemoteURL)

	for _, h := range r.RemoteHost {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	for _, pat := range r.RemoteRegex {
		if rx, err := regexp.Compile(pat); err == nil && rx.MatchString(ctx.RemoteURL) {
			return true
		}
	}
	for _, k := range r.ProjectKey {
		if strings.EqualFold(k, ctx.ProjectKey) {
			return true
		}
	}
	for _, s := range r.RepoSlug {
		if strings.EqualFold(s, ctx.RepoSlug) {
			return true
		}
	}
	for _, p := range r.BranchPrefix {
		if strings.HasPrefix(ctx.Branch, p) {
			return true
		}
	}
	return false
}

func isEmpty(m Match) bool {
	return len(m.RemoteHost) == 0 && len(m.RemoteRegex) == 0 &&
		len(m.ProjectKey) == 0 && len(m.RepoSlug) == 0 && len(m.BranchPrefix) == 0
}

// remoteHost extracts the host from common git remote URL forms.
//   - https://host/path        → host
//   - ssh://git@host:22/path   → host
//   - git@host:owner/repo.git  → host
func remoteHost(remote string) (string, error) {
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", err
		}
		return u.Hostname(), nil
	}
	// scp-like form: git@host:path
	if at := strings.Index(remote, "@"); at >= 0 {
		rest := remote[at+1:]
		if colon := strings.Index(rest, ":"); colon > 0 {
			return rest[:colon], nil
		}
	}
	return "", nil
}
