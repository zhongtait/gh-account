package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
)

func TestExecuteAndExecuteArgs(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	if err := store.UpsertAccount("alice", config.Account{
		Login:    "alice",
		Hostname: "github.com",
		GitName:  "Alice",
		Email:    "alice@example.com",
		Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}
	oldConfigDir := commandConfigDir
	defer func() { commandConfigDir = oldConfigDir }()
	commandConfigDir = func() (string, error) {
		return store.Dir, nil
	}
	flagConfigDir = store.Dir
	defer func() { flagConfigDir = "" }()
	oldGitExec := gitExec
	defer func() { gitExec = oldGitExec }()

	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		return nil
	}

	// 1. Execute runs with empty args or version
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"gha", "version"}
	if err := Execute(); err != nil {
		t.Fatal(err)
	}

	// 2. ExecuteArgs with empty args (prints help)
	if err := ExecuteArgs(nil); err != nil {
		t.Fatal(err)
	}

	// 3. ExecuteArgs with known command
	if err := ExecuteArgs([]string{"list"}); err != nil {
		t.Fatal(err)
	}

	// 4. ExecuteArgs with known command and config-dir flag
	tempDir := t.TempDir()
	if err := ExecuteArgs([]string{"--config-dir", tempDir, "init"}); err != nil {
		t.Fatal(err)
	}

	// 5. ExecuteArgs with unknown command -> routes to git
	var executedGitArgs []string
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		executedGitArgs = args
		return nil
	}
	flagConfigDir = store.Dir
	if err := ExecuteArgs([]string{"diff", "--stat"}); err != nil {
		t.Fatal(err)
	}
	if len(executedGitArgs) != 2 || executedGitArgs[0] != "diff" {
		t.Fatalf("expected git diff, got: %v", executedGitArgs)
	}

	// 6. ExecuteArgs with unknown command and --account flag and --config-dir
	executedGitArgs = nil
	if err := ExecuteArgs([]string{"--config-dir", store.Dir, "--account", "alice", "push", "origin", "main"}); err != nil {
		t.Fatal(err)
	}
	if len(executedGitArgs) == 0 || executedGitArgs[len(executedGitArgs)-1] != "main" {
		t.Fatalf("expected git push with account, got: %v", executedGitArgs)
	}

	// 7. setupDeps failure in ExecuteContextArgs
	commandConfigDir = func() (string, error) {
		return "", errors.New("cannot determine config dir")
	}
	flagConfigDir = ""
	if err := ExecuteArgs([]string{"push"}); err == nil || !strings.Contains(err.Error(), "cannot determine config dir") {
		t.Fatalf("expected config dir error, got: %v", err)
	}
}

func TestFindFirstNonFlagAndIsKnownRootCommand(t *testing.T) {
	root := NewRootCommand()

	// Flags with values
	args := []string{"--client-id", "123", "-a", "alice", "status"}
	cmdName, idx := findFirstNonFlag(args)
	if cmdName != "status" || idx != 4 {
		t.Fatalf("expected status at index 4, got %s at %d", cmdName, idx)
	}

	// Flags with =
	args = []string{"--config-dir=/tmp", "--client-id=123", "--account=alice", "-a=alice", "pull"}
	cmdName, idx = findFirstNonFlag(args)
	if cmdName != "pull" || idx != 4 {
		t.Fatalf("expected pull at index 4, got %s at %d", cmdName, idx)
	}

	// Only flags
	args = []string{"--no-browser", "-h", "--help"}
	cmdName, idx = findFirstNonFlag(args)
	if cmdName != "" || idx != -1 {
		t.Fatalf("expected empty non-flag, got %s at %d", cmdName, idx)
	}
	if !isKnownRootCommand(root, args) {
		t.Fatal("expected true when only flags are given")
	}

	// Subcommand alias match (e.g. "switch" for "use")
	if !isKnownRootCommand(root, []string{"switch", "alice"}) {
		t.Fatal("expected switch to be recognized as known root command")
	}

	// Unknown command (e.g. "commit")
	if isKnownRootCommand(root, []string{"commit", "-m", "fix"}) {
		t.Fatal("expected commit to be recognized as unknown (git) command")
	}
}

func TestExtractLeadingRootFlags(t *testing.T) {
	flagConfigDir = ""
	flagClientID = ""
	flagNoBrowser = false

	args := []string{"--config-dir", "/custom/dir", "--client-id", "my-client", "--no-browser", "status"}
	rem := extractLeadingRootFlags(args)
	if len(rem) != 1 || rem[0] != "status" {
		t.Fatalf("expected [status], got: %v", rem)
	}
	if flagConfigDir != "/custom/dir" || flagClientID != "my-client" || !flagNoBrowser {
		t.Fatalf("unexpected flag values: dir=%s id=%s noBrowser=%v", flagConfigDir, flagClientID, flagNoBrowser)
	}

	// Using = syntax and only flags
	flagConfigDir = ""
	flagClientID = ""
	flagNoBrowser = false
	args = []string{"--config-dir=/eq/dir", "--client-id=eq-id", "--no-browser"}
	rem = extractLeadingRootFlags(args)
	if len(rem) != 0 {
		t.Fatalf("expected empty rem, got: %v", rem)
	}
	if flagConfigDir != "/eq/dir" || flagClientID != "eq-id" || !flagNoBrowser {
		t.Fatalf("unexpected flag values: dir=%s id=%s noBrowser=%v", flagConfigDir, flagClientID, flagNoBrowser)
	}
}

func TestIsKnownGHCommandAndExecution(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	oldConfigDir := commandConfigDir
	defer func() { commandConfigDir = oldConfigDir }()
	commandConfigDir = func() (string, error) {
		return store.Dir, nil
	}
	oldLookPath := ghLookPath
	oldGHExec := ghExec
	defer func() {
		ghLookPath = oldLookPath
		ghExec = oldGHExec
	}()

	ghLookPath = func(string) (string, error) {
		return "/usr/bin/gh", nil
	}
	var executedGHArgs []string
	ghExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
		executedGHArgs = args
		return nil
	}

	for _, cmd := range []string{"pr", "issue", "issues", "api", "run", "gist", "release", "search", "workflow", "codespace"} {
		if !isKnownGHCommand(cmd) {
			t.Fatalf("expected true for %s", cmd)
		}
	}
	if isKnownGHCommand("status") || isKnownGHCommand("unknown") {
		t.Fatal("expected false for git/unknown commands")
	}

	flagConfigDir = store.Dir
	if err := ExecuteArgs([]string{"pr", "list"}); err != nil {
		t.Fatal(err)
	}
	if len(executedGHArgs) != 2 || executedGHArgs[0] != "pr" || executedGHArgs[1] != "list" {
		t.Fatalf("expected gh pr list, got: %v", executedGHArgs)
	}
}
