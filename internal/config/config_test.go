package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exisz/bitgit/internal/config"
)

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRemote != "origin" {
		t.Errorf("expected default_remote=origin, got %q", cfg.DefaultRemote)
	}
	if cfg.Dir() != dir {
		t.Errorf("expected dir=%s, got %s", dir, cfg.Dir())
	}
}

func TestParseValid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	toml := `
default_remote = "upstream"

[[hosts]]
url = "https://github.com"
type = "github"
token_file = "~/.bitgit/secrets/github.token"

[[hosts]]
url = "https://bb.example.com"
type = "bitbucket-dc"
token_file = "~/.bitgit/secrets/bitbucket.token"

[plugins]
disabled = ["old-plugin"]
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRemote != "upstream" {
		t.Errorf("expected default_remote=upstream, got %q", cfg.DefaultRemote)
	}
	if len(cfg.Hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(cfg.Hosts))
	}
	if cfg.Hosts[0].Type != "github" {
		t.Errorf("expected hosts[0].type=github")
	}
	if len(cfg.Plugins.Disabled) != 1 || cfg.Plugins.Disabled[0] != "old-plugin" {
		t.Errorf("expected plugins.disabled=[old-plugin]")
	}
}

func TestRejectMalformed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("default_remote = [broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
}

func TestEnvOverride(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Write config in dir2
	toml := `default_remote = "from-env"`
	if err := os.WriteFile(filepath.Join(dir2, "config.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BITGIT_HOME", dir1) // would normally be used
	config.Reset()

	// Override via env
	t.Setenv("BITGIT_HOME", dir2)
	config.Reset()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRemote != "from-env" {
		t.Errorf("env override failed, got %q", cfg.DefaultRemote)
	}
}

func TestReadTokenFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	t.Setenv("GITHUB_TOKEN", "ghp_testtoken")
	config.Reset()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := cfg.ReadToken("github")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "ghp_testtoken" {
		t.Errorf("expected ghp_testtoken, got %q", tok)
	}
}

func TestReadTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	t.Setenv("GITHUB_TOKEN", "") // clear env
	config.Reset()

	secretsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(secretsDir, "github.token")
	if err := os.WriteFile(tokenPath, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := cfg.ReadToken("github")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "file-token" {
		t.Errorf("expected file-token, got %q", tok)
	}
}
