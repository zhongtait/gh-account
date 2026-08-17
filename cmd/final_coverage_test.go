package cmd

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/utils"
)

type failingCloneConfig struct{ credentialErr error }

func (f failingCloneConfig) SetIdentity(context.Context, git.Scope, git.Identity) error { return nil }
func (f failingCloneConfig) SetCredentialHelper(context.Context, git.Scope, string, string) error {
	return f.credentialErr
}

func TestAutoHelperFinalBranches(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	g.remote = "https://github.com/owner/repo.git"
	manual := newAutoCmd()
	for key, value := range map[string]string{"alias": "bad", "login": "owner", "git-name": "Owner", "email": "bad", "protocol": "https"} {
		_ = manual.Flags().Set(key, value)
	}
	deps.Stdin = strings.NewReader("2\n")
	if err := manual.RunE(manual, nil); err == nil {
		t.Fatal("auto invalid account accepted")
	}

	info, _ := remote.Parse(g.remote)
	if err := store.UpsertAccount("other", config.Account{Login: "other", Hostname: "github.com", GitName: "Other", Email: "other@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := accountForRemote(info, "missing"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := promptAutoAccount(bufio.NewReader(strings.NewReader("\n")), remote.Info{Host: "github.com", Owner: "owner"}, "", config.Account{}, "owner", "owner", "Owner", "owner@example.com", ""); err != nil {
		t.Fatal(err)
	}

	// CurrentIdentity runs after login; hooks let the tests exercise post-login
	// identity and accounts-file failures independently.
	base := &commandGitHub{login: "owner", host: "github.com", hasCredential: true, clientID: "client"}
	deps.GitHub = identityHookGitHub{commandGitHub: base, identityErr: errors.New("identity")}
	deps.Stdin = strings.NewReader("owner@example.com\n")
	if _, err := runLoginForAuto(newAutoCmd(), info, bufio.NewReader(deps.Stdin)); err == nil {
		t.Fatal("post-login identity error ignored")
	}
	deps.GitHub = identityHookGitHub{commandGitHub: base, hook: func() { _ = os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600) }}
	deps.Stdin = strings.NewReader("owner@example.com\n")
	if _, err := runLoginForAuto(newAutoCmd(), info, bufio.NewReader(deps.Stdin)); err == nil {
		t.Fatal("post-login accounts error ignored")
	}
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = identityHookGitHub{commandGitHub: base, hook: func() { _ = store.SaveAccounts(config.DefaultAccounts()) }}
	deps.Stdin = strings.NewReader("owner@example.com\n")
	if _, err := runLoginForAuto(newAutoCmd(), info, bufio.NewReader(deps.Stdin)); err == nil || !strings.Contains(err.Error(), "no account profile") {
		t.Fatal(err)
	}
	if _, _, err := promptAutoAccount(bufio.NewReader(strings.NewReader("")), info, "", config.Account{}, " ", "owner", "Owner", "owner@example.com", "https"); err == nil {
		t.Fatal("blank alias accepted")
	}
	deps.GitHub = gh
}

func TestCloneFinalBranches(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(filepath.Join(blocked, "child"))
	if err := newCloneCmd().RunE(newCloneCmd(), []string{"https://github.com/alice/repo.git"}); err == nil {
		t.Fatal("clone initialization error ignored")
	}
	deps.Store = store

	info, _ := remote.Parse("https://github.com/org/repo.git")
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	gh.oauthErr = errors.New("oauth")
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), info, "", false); err == nil {
		t.Fatal("select login error ignored")
	}
	gh.oauthErr = nil
	gh.login = "new"
	gh.hasCredential = false
	deps.Stdin = strings.NewReader("new@example.com\n")
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), info, "", false); err == nil {
		t.Fatal("post-login credential error ignored")
	}

	base := &commandGitHub{login: "new", host: "github.com", hasCredential: true, clientID: "client"}
	deps.GitHub = identityHookGitHub{commandGitHub: base, identityErr: errors.New("identity")}
	deps.Stdin = strings.NewReader("new@example.com\n")
	if _, err := runLoginForClone(newCloneCmd(), info); err == nil {
		t.Fatal("clone identity error ignored")
	}
	deps.GitHub = identityHookGitHub{commandGitHub: base, hook: func() { _ = os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600) }}
	deps.Stdin = strings.NewReader("new@example.com\n")
	if _, err := runLoginForClone(newCloneCmd(), info); err == nil {
		t.Fatal("clone accounts error ignored")
	}
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = identityHookGitHub{commandGitHub: base, hook: func() { _ = store.SaveAccounts(config.DefaultAccounts()) }}
	deps.Stdin = strings.NewReader("new@example.com\n")
	if _, err := runLoginForClone(newCloneCmd(), info); err == nil || !strings.Contains(err.Error(), "no account profile") {
		t.Fatal(err)
	}
}

func TestCompletionEditLogoutAndUseFinalBranches(t *testing.T) {
	store, g, _, _ := commandSetup(t)
	addAlice(t, store)
	deps = Dependencies{}
	originalConfigDir := commandConfigDir
	t.Cleanup(func() { commandConfigDir = originalConfigDir })
	commandConfigDir = func() (string, error) { return "", errors.New("config") }
	if _, directive := completeAccountAliases(NewRootCommand(), nil, ""); directive != cobra.ShellCompDirectiveError {
		t.Fatal(directive)
	}
	deps = Dependencies{Store: config.NewStore(t.TempDir())}
	if _, directive := completeAccountAliases(NewRootCommand(), nil, ""); directive != cobra.ShellCompDirectiveError {
		t.Fatal(directive)
	}

	store, g, _, _ = commandSetup(t)
	addAlice(t, store)
	if err := newLogoutCmd().RunE(newLogoutCmd(), []string{"missing"}); err == nil {
		t.Fatal("missing logout alias accepted")
	}
	ghLogout := deps.GitHub.(*commandGitHub)
	ghLogout.logoutErr = errors.New("logout")
	if err := newLogoutCmd().RunE(newLogoutCmd(), []string{"alice"}); err == nil || !strings.Contains(err.Error(), "alice") {
		t.Fatal(err)
	}
	ghLogout.logoutErr = nil

	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "bad"}); err != nil {
		t.Fatal(err)
	}
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err == nil {
		t.Fatal("use bad scope accepted")
	}
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "global", UpdateRemote: true}); err != nil {
		t.Fatal(err)
	}
	g.remote = "https://github.com/other/repo.git"
	g.setRemoteErr = errors.New("set")
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	g.setRemoteErr = nil
	if err := updateOriginRemote(context.Background(), config.Account{Login: "alice", Protocol: "bad"}); err == nil {
		t.Fatal("bad protocol built")
	}
	if err := updateOriginRemote(context.Background(), config.Account{Login: "alice", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
}

func TestCloneInjectedAccountAndCredentialErrors(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	account := config.Account{Login: "new", Hostname: "github.com", GitName: "New", Email: "new@example.com", Protocol: "https"}
	oldAccount, oldLocal := cloneAccountByAlias, cloneLocalGit
	oldSelect, oldRun, oldDestination, oldLogin := cloneSelectAccount, cloneRun, cloneDestinationFor, cloneLogin
	t.Cleanup(func() {
		cloneAccountByAlias, cloneLocalGit = oldAccount, oldLocal
		cloneSelectAccount, cloneRun, cloneDestinationFor = oldSelect, oldRun, oldDestination
		cloneLogin = oldLogin
	})
	cloneAccountByAlias = func(*config.Store, string) (config.Account, error) {
		return config.Account{}, errors.New("load account")
	}
	cloneSelectAccount = func(context.Context, *cobra.Command, remote.Info, string, bool) (string, config.Account, error) {
		return "new", account, nil
	}
	cloneRun = func(context.Context, string, remote.Info, config.Account) error {
		return errors.New("Authentication failed")
	}
	cloneLogin = func(*cobra.Command, remote.Info) (string, error) { return "new", nil }
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), remote.Info{Host: "github.com", Owner: "org", Protocol: "https", Raw: "https://github.com/org/repo.git"}, "", false); err == nil {
		t.Fatal("clone account load error ignored")
	}

	cloneRun = func(context.Context, string, remote.Info, config.Account) error { return nil }
	cloneLocalGit = func(string) cloneConfigurator {
		return failingCloneConfig{credentialErr: errors.New("credential config")}
	}
	cloneDestinationFor = func(string, remote.Info) (string, error) { return t.TempDir(), nil }
	if err := newCloneCmd().RunE(newCloneCmd(), []string{"https://github.com/new/repo.git"}); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatal(err)
	}
	_ = store
}
