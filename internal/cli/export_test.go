package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// RootForTest wraps the cobra root command for testing.
type RootForTest struct {
	Cmd *cobra.Command
}

// NewRootForTest builds a root command for tests.
func NewRootForTest(b BuildInfo) *RootForTest {
	return &RootForTest{Cmd: NewRootCmd(b)}
}

// TestContext provides helpers for installing test plugins.
type TestContext struct {
	t          *testing.T
	pluginsDir string
}

// NewTestContext creates a TestContext that writes plugins to dir.
func NewTestContext(t *testing.T, bitgitHome string) *TestContext {
	t.Helper()
	dir := filepath.Join(bitgitHome, "plugins")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return &TestContext{t: t, pluginsDir: dir}
}

// InstallVetoPlugin installs a plugin that vetoes the given hook with reason.
func (tc *TestContext) InstallVetoPlugin(hook, reason string) {
	tc.t.Helper()
	tc.installPlugin("veto-"+hook, hook, map[string]any{
		"allow":  false,
		"reason": reason,
	})
}

// InstallMutatePlugin installs a plugin that mutates the given hook payload.
func (tc *TestContext) InstallMutatePlugin(hook string, mutate map[string]any) {
	tc.t.Helper()
	tc.installPlugin("mutate-"+hook, hook, map[string]any{
		"allow":  true,
		"mutate": mutate,
	})
}

// installPlugin writes a plugin.toml + a Go-compiled plugin executable.
// For simplicity, we write a shell script as the entrypoint.
func (tc *TestContext) installPlugin(name, hook string, result map[string]any) {
	tc.t.Helper()
	dir := filepath.Join(tc.pluginsDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tc.t.Fatal(err)
	}

	// Write plugin.toml
	manifest := fmt.Sprintf(`name = %q
version = "0.0.1"
entrypoint = "./plugin.sh"
hooks = [%q]
`, name, hook)
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(manifest), 0o600); err != nil {
		tc.t.Fatal(err)
	}

	// Encode the result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		tc.t.Fatal(err)
	}

	// Shell script: reads one JSON-RPC request and responds with allow/mutate result.
	// Always uses id=1 (works for single-call plugins).
	script := fmt.Sprintf(`#!/bin/sh
read line
printf '{"jsonrpc":"2.0","id":1,"result":%s}\n'
`, string(resultJSON))

	pluginPath := filepath.Join(dir, "plugin.sh")
	if err := os.WriteFile(pluginPath, []byte(script), 0o700); err != nil {
		tc.t.Fatal(err)
	}
}
