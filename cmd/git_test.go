package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/remote"
)

func TestGitCmdCloneForwardsDirFlag(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	oldRun := cloneRun
	oldDest := cloneDestinationFor
	oldCustom := cloneCustomDest
	t.Cleanup(func() {
		cloneRun = oldRun
		cloneDestinationFor = oldDest
		cloneCustomDest = oldCustom
	})

	if err := store.UpsertAccount("personal", config.Account{
		Login:    "personal",
		Hostname: "github.com",
		GitName:  "Personal User",
		Email:    "personal@example.com",
		Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}

	var usedDestination string
	cloneRun = func(context.Context, string, remote.Info, config.Account) error { return nil }
	destination := t.TempDir()
	if err := os.MkdirAll(filepath.Join(destination, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, ".git", "config"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cloneDestinationFor = func(_ string, _ remote.Info) (string, error) {
		usedDestination = cloneCustomDest
		return destination, nil
	}

	for _, args := range [][]string{
		{"clone", "https://github.com/personal/repo.git", "-d", "custom-dir"},
		{"clone", "https://github.com/personal/repo.git", "--dir", "custom-dir"},
		{"clone", "https://github.com/personal/repo.git", "--dir=custom-dir"},
		{"clone", "-d=custom-dir", "https://github.com/personal/repo.git"},
		{"clone", "https://github.com/personal/repo.git", "-a", "personal", "-d", "custom-dir"},
		{"clone", "--account=personal", "https://github.com/personal/repo.git", "--dir=custom-dir"},
	} {
		usedDestination = ""
		if err := newGitCmd().RunE(newGitCmd(), args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if usedDestination != "custom-dir" {
			t.Fatalf("%v: destination = %q", args, usedDestination)
		}
	}

	for _, flag := range []string{"-d", "--dir", "-a", "--account"} {
		if err := newGitCmd().RunE(newGitCmd(), []string{"clone", "repo", flag}); err == nil {
			t.Fatalf("missing %s value accepted", flag)
		}
	}
}

func TestGitPassthroughPreservesNativeFlagsAndValues(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	addAlice(t, store)
	originalExec := gitExec
	t.Cleanup(func() { gitExec = originalExec })
	var executed []string
	gitExec = func(_ context.Context, _ io.Reader, _, _ io.Writer, args ...string) error {
		executed = args
		return nil
	}
	for _, tc := range []struct {
		name    string
		args    []string
		want    []string
		account bool
	}{
		{"native short flag", []string{"git", "commit", "-a", "--dry-run"}, []string{"commit", "-a", "--dry-run"}, false},
		{"shorthand", []string{"commit", "-a", "--dry-run"}, []string{"commit", "-a", "--dry-run"}, false},
		{"native trailing flag", []string{"git", "branch", "-a"}, []string{"branch", "-a"}, false},
		{"leading account", []string{"git", "-a", "alice", "commit", "-a", "--dry-run"}, []string{"commit", "-a", "--dry-run"}, true},
		{"long account", []string{"--account=alice", "commit", "-a", "--dry-run"}, []string{"commit", "-a", "--dry-run"}, true},
		{"message is a flag", []string{"git", "commit", "-m", "--account", "--dry-run"}, []string{"commit", "-m", "--account", "--dry-run"}, false},
		{"path separator", []string{"git", "diff", "--", "-a", "--account=alice"}, []string{"diff", "--", "-a", "--account=alice"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executed = nil
			args := append([]string{"--config-dir", store.Dir}, tc.args...)
			if err := ExecuteArgs(args); err != nil {
				t.Fatal(err)
			}
			forwarded := executed
			if tc.account {
				if len(executed) <= len(tc.want) || executed[0] != "-c" || !strings.HasPrefix(executed[1], "user.name=") {
					t.Fatalf("account was not applied: %v", executed)
				}
				forwarded = executed[len(executed)-len(tc.want):]
			}
			if !reflect.DeepEqual(forwarded, tc.want) {
				t.Fatalf("forwarded %q, want %q", forwarded, tc.want)
			}
		})
	}
}

func TestRunSystemGit(t *testing.T) {
	// Tests the real runSystemGit function using a safe flag
	_ = runSystemGit(context.Background(), strings.NewReader(""), io.Discard, io.Discard, "--version")
}

func TestGitCmdHelpAndNoArgs(t *testing.T) {
	commandSetup(t)
	oldGitExec := gitExec
	defer func() { gitExec = oldGitExec }()

	called := false
	var capturedArgs []string
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		called = true
		capturedArgs = args
		return nil
	}

	gitCmd := newGitCmd()
	// No args: runs git directly
	if err := gitCmd.RunE(gitCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !called || len(capturedArgs) != 0 {
		t.Fatalf("expected gitExec with 0 args, got called=%v args=%v", called, capturedArgs)
	}

	// Help flags
	if err := gitCmd.RunE(gitCmd, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd.RunE(gitCmd, []string{"-h"}); err != nil {
		t.Fatal(err)
	}
}

func TestGitCmdExtractAccountFlagErrors(t *testing.T) {
	commandSetup(t)
	gitCmd := newGitCmd()

	// Missing arg after --account
	if err := gitCmd.RunE(gitCmd, []string{"--account"}); err == nil || !strings.Contains(err.Error(), "--account requires an argument") {
		t.Fatalf("expected missing arg error, got %v", err)
	}

	// Missing arg after -a
	if err := gitCmd.RunE(gitCmd, []string{"-a"}); err == nil || !strings.Contains(err.Error(), "-a requires an argument") {
		t.Fatalf("expected missing arg error, got %v", err)
	}
}

func TestGitCmdWithAccountFlagOnly(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	oldGitExec := gitExec
	defer func() { gitExec = oldGitExec }()

	if err := store.UpsertAccount("work", config.Account{
		Login:    "workuser",
		Hostname: "github.com",
		GitName:  "Work User",
		Email:    "work@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	var capturedArgs []string
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		capturedArgs = args
		return nil
	}

	gitCmd := newGitCmd()
	// --account work with no git args
	if err := gitCmd.RunE(gitCmd, []string{"--account", "work"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedArgs) == 0 || !strings.Contains(capturedArgs[1], "Work User") {
		t.Fatalf("expected account args injected, got: %v", capturedArgs)
	}

	// -a=work with no git args
	capturedArgs = nil
	if err := gitCmd.RunE(gitCmd, []string{"-a=work"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedArgs) == 0 {
		t.Fatalf("expected account args injected, got: %v", capturedArgs)
	}

	// --account=work with no git args
	capturedArgs = nil
	if err := gitCmd.RunE(gitCmd, []string{"--account=work"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedArgs) == 0 {
		t.Fatalf("expected account args injected, got: %v", capturedArgs)
	}
}

func TestGitCmdPassthroughExecution(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	oldGitExec := gitExec
	defer func() { gitExec = oldGitExec }()

	var capturedArgs []string
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		capturedArgs = args
		return nil
	}

	gitCmd := newGitCmd()
	// Plain git status
	if err := gitCmd.RunE(gitCmd, []string{"status", "-s"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "status" || capturedArgs[1] != "-s" {
		t.Fatalf("unexpected git args: %v", capturedArgs)
	}

	// Account not found error
	if err := gitCmd.RunE(gitCmd, []string{"--account", "nonexistent", "push"}); err == nil || !strings.Contains(err.Error(), "account \"nonexistent\" not found") {
		t.Fatalf("expected nonexistent account error, got: %v", err)
	}

	// Execution error from git
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		return errors.New("git failure")
	}
	if err := gitCmd.RunE(gitCmd, []string{"pull"}); err == nil || !strings.Contains(err.Error(), "git failure") {
		t.Fatalf("expected git failure error, got: %v", err)
	}

	// With initialized account
	if err := store.UpsertAccount("minimal", config.Account{
		Login:    "min",
		Hostname: "github.com",
		GitName:  "Min",
		Email:    "min@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		capturedArgs = args
		return nil
	}
	if err := gitCmd.RunE(gitCmd, []string{"-a", "minimal", "status"}); err != nil {
		t.Fatal(err)
	}
	// Check helper args present and status at the end
	if len(capturedArgs) < 3 || capturedArgs[len(capturedArgs)-1] != "status" {
		t.Fatalf("unexpected args: %v", capturedArgs)
	}

	// Uninitialized store error
	storeUninit := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = storeUninit
	if err := gitCmd.RunE(gitCmd, []string{"--account", "work", "status"}); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	deps.Store = oldStore
}

func TestGitCmdCloneIntegration(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	oldRun := cloneRun
	oldDest := cloneDestinationFor
	defer func() { cloneRun = oldRun; cloneDestinationFor = oldDest }()

	tempDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(tempDir, ".git", "config"), nil, 0o644)
	cloneDestinationFor = func(string, remote.Info) (string, error) {
		return tempDir, nil
	}

	if err := store.UpsertAccount("personal", config.Account{
		Login:    "personal",
		Hostname: "github.com",
		GitName:  "Personal User",
		Email:    "personal@example.com",
		Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}

	cloneRun = func(ctx context.Context, repository string, info remote.Info, account config.Account) error {
		return nil
	}

	gitCmd := newGitCmd()

	// Clone help
	if err := gitCmd.RunE(gitCmd, []string{"clone", "--help"}); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd.RunE(gitCmd, []string{"clone", "-h"}); err != nil {
		t.Fatal(err)
	}

	// Clone with invalid arguments
	if err := gitCmd.RunE(gitCmd, []string{"clone"}); err == nil {
		t.Fatal("expected clone usage error")
	}

	// Clone with repository
	if err := gitCmd.RunE(gitCmd, []string{"clone", "https://github.com/personal/repo.git", "use", "personal"}); err != nil {
		t.Fatal(err)
	}

	// Clone with --account work (which appends "use work")
	if err := gitCmd.RunE(gitCmd, []string{"--account", "personal", "clone", "https://github.com/personal/repo.git"}); err != nil {
		t.Fatal(err)
	}

	// Clone with --account work where "use" is already present
	if err := gitCmd.RunE(gitCmd, []string{"-a", "personal", "clone", "https://github.com/personal/repo.git", "use", "personal"}); err != nil {
		t.Fatal(err)
	}
}

func TestGitCmdDispatchToGHASubcommands(t *testing.T) {
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
	oldGitExec := gitExec
	defer func() { gitExec = oldGitExec }()

	gitCmd := newGitCmd()
	root := NewRootCommand()
	root.AddCommand(gitCmd)

	// 1. gha git list -> dispatches to gha list
	if err := gitCmd.RunE(gitCmd, []string{"list"}); err != nil {
		t.Fatal(err)
	}

	// 2. gha git use alice -> dispatches to gha use alice
	if err := gitCmd.RunE(gitCmd, []string{"use", "alice"}); err != nil {
		t.Fatal(err)
	}

	// 3. gha git switch alice -> dispatches to gha use alice via switch alias
	if err := gitCmd.RunE(gitCmd, []string{"switch", "alice"}); err != nil {
		t.Fatal(err)
	}

	// 4. gha git whoami -> dispatches to gha current via whoami alias
	if err := gitCmd.RunE(gitCmd, []string{"whoami"}); err != nil {
		t.Fatal(err)
	}

	// 5. gha git version -> dispatches to gha version (target.Run)
	if err := gitCmd.RunE(gitCmd, []string{"version"}); err != nil {
		t.Fatal(err)
	}

	// 6. gha git use --invalid-flag -> returns flag error
	if err := gitCmd.RunE(gitCmd, []string{"use", "--invalid-flag"}); err == nil {
		t.Fatal("expected flag parse error")
	}

	// 7. gha git use a b -> returns args error (accepts at most 1)
	if err := gitCmd.RunE(gitCmd, []string{"use", "a", "b"}); err == nil {
		t.Fatal("expected args validation error")
	}

	// 8. Reserved native git commands should not be routed to GHA:
	// "add", "init", "remote", "git"
	var passedGitCommand string
	gitExec = func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		if len(args) > 0 {
			passedGitCommand = args[0]
		}
		return nil
	}
	for _, name := range []string{"add", "init", "remote", "git", "status"} {
		if err := gitCmd.RunE(gitCmd, []string{name}); err != nil {
			t.Fatal(err)
		}
		if passedGitCommand != name {
			t.Fatalf("expected native git invocation for %q, got %q", name, passedGitCommand)
		}
	}

	// 9. findGHASubcommand on standalone command without root returns nil
	standaloneCmd := newGitCmd()
	if c := findGHASubcommand(standaloneCmd, "use"); c != nil {
		t.Fatal("expected nil when no root")
	}

	// 10. findGHASubcommand on command attached to root but name not found
	if c := findGHASubcommand(gitCmd, "nonexistent"); c != nil {
		t.Fatal("expected nil when subcommand not in root")
	}

	// 11. gha git use --help -> triggers pflag.ErrHelp and displays help
	if err := gitCmd.RunE(gitCmd, []string{"use", "--help"}); err != nil {
		t.Fatal(err)
	}
}
