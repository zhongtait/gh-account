package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestSelectCloneAccountVariants(t *testing.T) {
	ctx := context.Background()
	info, _ := remote.Parse("https://github.com/org/repo.git")

	store, _, _, _ := commandSetup(t)
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "", false); err == nil {
		t.Fatal("malformed accounts accepted")
	}

	// Explicit organization clone without a checker is rejected.
	store, _, _, _ = commandSetup(t)
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = loginOnlyGitHub{login: "alice"}
	if _, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "alice", true); err == nil || !strings.Contains(err.Error(), "checking is unavailable") {
		t.Fatal(err)
	}
	deps.GitHub = &commandGitHub{login: "alice", host: "github.com", hasCredential: true, credentialErr: errors.New("check")}
	if _, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "alice", true); err == nil || !strings.Contains(err.Error(), "check account credential") {
		t.Fatal(err)
	}

	// Without a checker, an owner match wins immediately.
	ownerInfo, _ := remote.Parse("https://github.com/alice/repo.git")
	deps.GitHub = loginOnlyGitHub{login: "alice"}
	if alias, _, err := selectCloneAccount(ctx, newCloneCmd(), ownerInfo, "", false); err != nil || alias != "alice" {
		t.Fatalf("owner=%q %v", alias, err)
	}

	// Active account wins among multiple usable accounts; a single candidate is
	// selected, while multiple non-active candidates require disambiguation.
	store, _, gh, _ := commandSetup(t)
	for alias, login := range map[string]string{"alice": "alice", "bob": "bob"} {
		if err := store.UpsertAccount(alias, config.Account{Login: login, Hostname: "github.com", GitName: login, Email: login + "@example.com", Protocol: "https"}); err != nil {
			t.Fatal(err)
		}
	}
	gh.login, gh.hasCredential = "alice", true
	if alias, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "", false); err != nil || alias != "alice" {
		t.Fatalf("active=%q %v", alias, err)
	}
	gh.login = "nobody"
	if _, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "", false); err == nil || !strings.Contains(err.Error(), "multiple logged-in") {
		t.Fatal(err)
	}
	if err := store.RemoveAccount("bob"); err != nil {
		t.Fatal(err)
	}
	if alias, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "", false); err != nil || alias != "alice" {
		t.Fatalf("single=%q %v", alias, err)
	}

	// Accounts on another host are skipped. With no usable credential the login
	// flow runs, then verifies the newly stored account.
	store, _, gh, _ = commandSetup(t)
	if err := store.UpsertAccount("enterprise", config.Account{Login: "org", Hostname: "ghe.example", GitName: "Org", Email: "org@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	gh.login, gh.hasCredential = "newuser", true
	deps.Stdin = strings.NewReader("new@example.com\n")
	if alias, _, err := selectCloneAccount(ctx, newCloneCmd(), info, "", false); err != nil || alias != "newuser" {
		t.Fatalf("login selection=%q %v", alias, err)
	}
}

func TestCloneCommandFailureAndRetryPaths(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	clone := newCloneCmd()
	if values, directive := clone.ValidArgsFunction(clone, nil, ""); values != nil || directive != cobra.ShellCompDirectiveDefault {
		t.Fatal("unexpected default completion")
	}
	originalSelect, originalRun, originalDestination, originalLogin, originalLocal := cloneSelectAccount, cloneRun, cloneDestinationFor, cloneLogin, cloneLocalGit
	t.Cleanup(func() {
		cloneSelectAccount, cloneRun, cloneDestinationFor, cloneLogin, cloneLocalGit = originalSelect, originalRun, originalDestination, originalLogin, originalLocal
	})
	cloneSelectAccount = func(context.Context, *cobra.Command, remote.Info, string, bool) (string, config.Account, error) {
		return "", config.Account{}, errors.New("select")
	}
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git"}); err == nil {
		t.Fatal("select error ignored")
	}
	cloneSelectAccount = originalSelect
	if err := clone.RunE(clone, []string{"bad"}); err == nil || !strings.Contains(err.Error(), "supported GitHub remote") {
		t.Fatal(err)
	}
	cloneSelectAccount = func(context.Context, *cobra.Command, remote.Info, string, bool) (string, config.Account, error) {
		return "alice", account, nil
	}
	cloneRun = func(context.Context, string, remote.Info, config.Account) error {
		return errors.New("Authentication failed")
	}
	cloneLogin = func(*cobra.Command, remote.Info) (string, error) { return "", errors.New("login") }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git"}); err == nil || !strings.Contains(err.Error(), "automatic login failed") {
		t.Fatal(err)
	}
	cloneLogin = func(*cobra.Command, remote.Info) (string, error) { return "missing", nil }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git"}); err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatal(err)
	}
	cloneLogin = originalLogin
	cloneRun = originalRun
	cloneSelectAccount = func(context.Context, *cobra.Command, remote.Info, string, bool) (string, config.Account, error) {
		return "alice", account, nil
	}
	cloneRun = func(context.Context, string, remote.Info, config.Account) error { return nil }
	cloneDestinationFor = func(string, remote.Info) (string, error) { return "", errors.New("destination") }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git", "use", "alice"}); err == nil {
		t.Fatal("destination error ignored")
	}
	cloneDestinationFor = originalDestination
	cloneRun = originalRun
	cloneSelectAccount = originalSelect

	// Retry succeeds, then configures the prepared repository.
	destination := t.TempDir()
	if err := os.MkdirAll(filepath.Join(destination, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, ".git", "config"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cloneSelectAccount = func(context.Context, *cobra.Command, remote.Info, string, bool) (string, config.Account, error) {
		return "alice", account, nil
	}
	calls := 0
	cloneRun = func(context.Context, string, remote.Info, config.Account) error {
		calls++
		if calls == 1 {
			return errors.New("Authentication failed")
		}
		return nil
	}
	cloneLogin = func(*cobra.Command, remote.Info) (string, error) { return "alice", nil }
	cloneDestinationFor = func(string, remote.Info) (string, error) { return destination, nil }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git"}); err != nil {
		t.Fatal(err)
	}
	cloneRun = func(context.Context, string, remote.Info, config.Account) error { return errors.New("network") }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git", "use", "alice"}); err == nil {
		t.Fatal("explicit clone error ignored")
	}
	cloneSelectAccount, cloneRun, cloneDestinationFor, cloneLogin = originalSelect, originalRun, originalDestination, originalLogin

	oldClone := cloneGit
	t.Cleanup(func() { cloneGit = oldClone })
	t.Chdir(t.TempDir())
	cloneLocalGit = func(string) cloneConfigurator {
		return failingCloneConfig{credentialErr: errors.New("credential config")}
	}
	cloneGit = func(context.Context, string, string, io.Writer, io.Writer) error { return nil }
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git", "use", "alice"}); err == nil || !strings.Contains(err.Error(), "configure cloned") {
		t.Fatal(err)
	}

	// Auto retry combines the original auth failure with a failed login.
	cloneGit = func(context.Context, string, string, io.Writer, io.Writer) error {
		return errors.New("Authentication failed")
	}
	gh.oauthErr = errors.New("oauth")
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git"}); err == nil || !strings.Contains(err.Error(), "automatic login failed") {
		t.Fatal(err)
	}
}

func TestCloneCredentialAndLoginErrorBranches(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	info, _ := remote.Parse("https://github.com/alice/repo.git")
	gh.hasCredential = false
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), info, "alice", true); err == nil || !strings.Contains(err.Error(), "cannot clone") {
		t.Fatal(err)
	}
	deps.GitHub = loginOnlyGitHub{login: "alice"}
	if err := requireAccountCredential(context.Background(), account); err == nil {
		t.Fatal("missing checker accepted")
	}
	deps.GitHub = &commandGitHub{credentialErr: errors.New("check")}
	if err := requireAccountCredential(context.Background(), account); err == nil || !strings.Contains(err.Error(), "check") {
		t.Fatal(err)
	}

	cloneGitOld := cloneGit
	t.Cleanup(func() { cloneGit = cloneGitOld })
	cloneGit = func(_ context.Context, _ string, _ string, _ io.Writer, stderr io.Writer) error {
		_, _ = io.WriteString(stderr, "server denied")
		return errors.New("clone")
	}
	if err := cloneWithAccount(context.Background(), info.Raw, info, account); err == nil || !strings.Contains(err.Error(), "server denied") {
		t.Fatal(err)
	}
}
