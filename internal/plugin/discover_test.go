package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverEmpty(t *testing.T) {
	dir := t.TempDir()
	plugins, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscoverMissingDir(t *testing.T) {
	plugins, err := Discover(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if plugins != nil {
		t.Fatalf("expected nil plugins, got %v", plugins)
	}
}

func TestDiscoverManifest(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `name = "demo"
version = "0.1.0"
entrypoint = "./run"
hooks = ["pre-commit", "pre-push"]

[match]
remote_host = ["bitbucket.example.com"]
project_key = ["PLAT"]
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	p := plugins[0]
	if p.Name != "demo" || p.Version != "0.1.0" {
		t.Errorf("unexpected manifest: %+v", p)
	}
	if len(p.Hooks) != 2 {
		t.Errorf("expected 2 hooks, got %v", p.Hooks)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		name string
		m    Match
		ctx  Context
		want bool
	}{
		{"empty match attaches", Match{}, Context{RemoteURL: "https://anywhere/x.git"}, true},
		{"host match", Match{RemoteHost: []string{"bb.example.com"}}, Context{RemoteURL: "https://bb.example.com/scm/p/r.git"}, true},
		{"host miss", Match{RemoteHost: []string{"other.com"}}, Context{RemoteURL: "https://bb.example.com/x.git"}, false},
		{"scp host", Match{RemoteHost: []string{"bb.example.com"}}, Context{RemoteURL: "git@bb.example.com:p/r.git"}, true},
		{"project key", Match{ProjectKey: []string{"PLAT"}}, Context{ProjectKey: "PLAT"}, true},
		{"branch prefix", Match{BranchPrefix: []string{"feature/"}}, Context{Branch: "feature/foo"}, true},
		{"regex match", Match{RemoteRegex: []string{`bb\.example\.com`}}, Context{RemoteURL: "https://bb.example.com/x"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Manifest{Match: c.m}
			if got := m.Matches(c.ctx); got != c.want {
				t.Errorf("Matches=%v want %v", got, c.want)
			}
		})
	}
}
