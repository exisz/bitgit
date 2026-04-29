// Package config loads and exposes the ~/.bitgit/config.toml configuration.
//
// Usage:
//
//	cfg, err := config.Load()         // reads $BITGIT_HOME/config.toml or ~/.bitgit/config.toml
//	cfg := config.MustLoad()          // panics on error
//
// The singleton is safe for concurrent read after Load() returns.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
)

// HostEntry is a per-host credential block in [[hosts]].
type HostEntry struct {
	URL       string `toml:"url"`
	Type      string `toml:"type"`       // "github" | "bitbucket-dc"
	TokenFile string `toml:"token_file"` // path to token file
}

// PluginsConfig holds the [plugins] section overrides.
type PluginsConfig struct {
	Disabled []string `toml:"disabled"`
}

// ReviewersConfig holds the [reviewers] section.
type ReviewersConfig struct {
	// Team is a fixed list of reviewer usernames always added to every PR.
	Team []string `toml:"team"`
	// IncludeRecent, when true, also pulls reviewers from recent merged PRs.
	IncludeRecent bool `toml:"include_recent"`
	// RecentLimit is the number of recent merged PRs to scan (default 1).
	RecentLimit int `toml:"recent_limit"`
}

// Config is the parsed ~/.bitgit/config.toml.
type Config struct {
	DefaultRemote string          `toml:"default_remote"`
	Hosts         []HostEntry     `toml:"hosts"`
	Plugins       PluginsConfig   `toml:"plugins"`
	Reviewers     ReviewersConfig `toml:"reviewers"`

	// InsecureSkipVerify enables skipping TLS cert validation for all hosts.
	// WARNING: exposes you to MITM attacks. Use only in controlled environments.
	InsecureSkipVerify bool `toml:"insecure_skip_verify"`

	// dir is the resolved config directory (not serialised).
	dir string
}

// Dir returns the config directory (e.g. ~/.bitgit).
func (c *Config) Dir() string { return c.dir }

// SecretsDir returns the secrets subdirectory.
func (c *Config) SecretsDir() string { return filepath.Join(c.dir, "secrets") }

// PluginsDir returns the plugins subdirectory.
func (c *Config) PluginsDir() string { return filepath.Join(c.dir, "plugins") }

// HostForURL finds the first [[hosts]] entry matching the given URL prefix.
// Returns nil if not found.
func (c *Config) HostForURL(rawURL string) *HostEntry {
	for i, h := range c.Hosts {
		if h.URL != "" && len(rawURL) >= len(h.URL) && rawURL[:len(h.URL)] == h.URL {
			return &c.Hosts[i]
		}
	}
	return nil
}

var (
	once      sync.Once
	singleton *Config
	loadErr   error
)

// Load loads the config exactly once and returns the singleton.
// Subsequent calls return the cached result.
func Load() (*Config, error) {
	once.Do(func() {
		singleton, loadErr = load()
	})
	return singleton, loadErr
}

// MustLoad is like Load but panics on error.
func MustLoad() *Config {
	c, err := Load()
	if err != nil {
		panic("bitgit config: " + err.Error())
	}
	return c
}

// Reset clears the singleton — for use in tests only.
func Reset() {
	once = sync.Once{}
	singleton = nil
	loadErr = nil
}

func load() (*Config, error) {
	dir, err := resolveDir()
	if err != nil {
		return nil, err
	}

	cfg := defaults(dir)

	path := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// No config file — return defaults, don't crash.
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.dir = dir
	return cfg, nil
}

// resolveDir returns the bitgit config directory. Respects $BITGIT_HOME.
func resolveDir() (string, error) {
	if v := os.Getenv("BITGIT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".bitgit"), nil
}

func defaults(dir string) *Config {
	return &Config{
		DefaultRemote: "origin",
		Reviewers: ReviewersConfig{
			RecentLimit: 1,
		},
		dir: dir,
	}
}
