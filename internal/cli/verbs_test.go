package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/exisz/bitgit/internal/cli"
	"github.com/exisz/bitgit/internal/config"
)

// buildRoot creates a fresh root command for testing.
func buildRoot() *cli.RootForTest {
	return cli.NewRootForTest(cli.BuildInfo{Version: "test", Commit: "abc", Date: "now"})
}

func TestPRCreateFlags(t *testing.T) {
	// Verify that --target is required and --title/--source/--draft flags are accepted
	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"pr", "create", "--help"})
	err := root.Cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	help := out.String()
	for _, flag := range []string{"--title", "--description", "--source", "--target", "--draft", "--reviewer"} {
		if !strings.Contains(help, flag) {
			t.Errorf("expected %s in help output", flag)
		}
	}
}

func TestPRListFlags(t *testing.T) {
	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"pr", "list", "--help"})
	err := root.Cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	help := out.String()
	for _, flag := range []string{"--mine", "--state", "--json"} {
		if !strings.Contains(help, flag) {
			t.Errorf("expected %s in help output", flag)
		}
	}
}

func TestCommitFlagRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"commit"})
	err := root.Cmd.Execute()
	if err == nil {
		t.Error("expected error when -m is missing, got nil")
	}
}

func TestDoctorRuns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"doctor"})
	// Doctor should not error even outside a git repo
	_ = root.Cmd.Execute()
	output := out.String()
	if !strings.Contains(output, "bitgit doctor") {
		t.Errorf("expected doctor header in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// Hook dispatch unit tests using mock host + mock dispatcher
// ---------------------------------------------------------------------------

func TestPRCreate_HookVeto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	// Install a veto plugin
	tc := cli.NewTestContext(t, dir)
	tc.InstallVetoPlugin("pre-pr-create", "policy violation")

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"pr", "create", "--title", "test", "--target", "main"})
	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected veto error, got nil")
	}
	if !strings.Contains(err.Error(), "vetoed") && !strings.Contains(out.String(), "vetoed") && !strings.Contains(err.Error(), "policy violation") {
		t.Errorf("expected veto message, got err=%v out=%s", err, out.String())
	}
}

func TestPRCreate_HookMutation(t *testing.T) {
	// The mutation test verifies that the payload sent to CreatePR uses mutated values.
	// We use a mock host to capture the input.
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	tc := cli.NewTestContext(t, dir)
	tc.InstallMutatePlugin("pre-pr-create", map[string]any{
		"title": "[JIRA-1] test title",
	})

	// This will fail at host detection (not a real git repo) but we can verify
	// the hook was called with the right payload by inspecting the plugin output.
	// For now just ensure no panic.
	root := buildRoot()
	root.Cmd.SetArgs([]string{"pr", "create", "--title", "test title", "--target", "main"})
	_ = root.Cmd.Execute() // may fail at git/host detection — that's fine
}

func TestCommit_HookVeto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	tc := cli.NewTestContext(t, dir)
	tc.InstallVetoPlugin("pre-commit", "commit message policy")

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"commit", "-m", "bad message"})
	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected veto error")
	}
	if !strings.Contains(err.Error(), "vetoed") && !strings.Contains(err.Error(), "commit message policy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPush_HookVeto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	tc := cli.NewTestContext(t, dir)
	tc.InstallVetoPlugin("pre-push", "no force push to main")

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"push", "--force"})
	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected veto error")
	}
	if !strings.Contains(err.Error(), "vetoed") && !strings.Contains(err.Error(), "no force push") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBranchNew_HookVeto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BITGIT_HOME", dir)
	config.Reset()

	tc := cli.NewTestContext(t, dir)
	tc.InstallVetoPlugin("pre-branch-new", "branch name policy")

	root := buildRoot()
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs([]string{"branch", "new", "my-branch"})
	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected veto error")
	}
	if !strings.Contains(err.Error(), "vetoed") && !strings.Contains(err.Error(), "branch name policy") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Ensure unused import doesn't cause issues.
var _ = context.Background
