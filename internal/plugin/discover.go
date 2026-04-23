// Package plugin owns the bitgit plugin protocol: discovery on disk,
// match-rule evaluation, JSON-RPC stdio transport, and hook dispatch.
//
// Plugins are subprocesses, language-agnostic. The protocol is JSON-RPC 2.0
// over stdin/stdout. See docs/plugin-protocol.md for the wire spec.
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// Manifest describes a plugin loaded from <dir>/plugin.toml.
type Manifest struct {
	Name       string   `toml:"name"`
	Version    string   `toml:"version"`
	Entrypoint string   `toml:"entrypoint"` // path or command, resolved against Dir
	Hooks      []string `toml:"hooks"`      // e.g. ["pre-pr-create", "pre-commit"]
	Match      Match    `toml:"match"`
	Dir        string   `toml:"-"`
}

// Match is the auto-attach rule set. A plugin attaches when ANY rule fires.
// All fields are optional; an empty Match means "always attach".
type Match struct {
	RemoteHost   []string `toml:"remote_host"`   // exact host, e.g. "bitbucket.example.com"
	RemoteRegex  []string `toml:"remote_regex"`  // RE2 against full remote URL
	ProjectKey   []string `toml:"project_key"`   // Bitbucket DC project key, e.g. "PLATFORM"
	RepoSlug     []string `toml:"repo_slug"`     // Bitbucket repo slug
	BranchPrefix []string `toml:"branch_prefix"` // current branch starts with one of these
}

// Discover scans pluginsDir for <name>/plugin.toml manifests.
// Missing dir returns (nil, nil) — not an error.
func Discover(pluginsDir string) ([]Manifest, error) {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir %q: %w", pluginsDir, err)
	}

	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, e.Name())
		manifestPath := filepath.Join(dir, "plugin.toml")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}
		var m Manifest
		if _, err := toml.DecodeFile(manifestPath, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
		}
		if m.Name == "" {
			m.Name = e.Name()
		}
		m.Dir = dir
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
