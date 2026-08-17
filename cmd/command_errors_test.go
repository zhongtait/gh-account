package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/utils"
)

func addAlice(t *testing.T, store *config.Store) config.Account {
	t.Helper()
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	return account
}

func TestAddAndLoginFailures(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	gh.oauthErr = errors.New("oauth")
	add := newAddCmd()
	if err := add.RunE(add, nil); err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatal(err)
	}
	gh.oauthErr = nil
	gh.loginErr = errors.New("login")
	if err := add.RunE(add, nil); err == nil || !strings.Contains(err.Error(), "get login") {
		t.Fatal(err)
	}
	gh.loginErr = nil
	deps.Stdin = strings.NewReader("")
	manual := newAddCmd()
	_ = manual.Flags().Set("manual", "true")
	if err := manual.RunE(manual, nil); err == nil {
		t.Fatal("manual EOF accepted")
	}

	gh.login = ""
	login := newLoginCmd()
	_ = login.Flags().Set("git-name", "Name")
	_ = login.Flags().Set("email", "a@example.com")
	if err := login.RunE(login, nil); err == nil || !strings.Contains(err.Error(), "user is empty") {
		t.Fatal(err)
	}
	gh.login = "alice"
	deps.Stdin = strings.NewReader("")
	login = newLoginCmd()
	if err := login.RunE(login, nil); err == nil {
		t.Fatal("missing email accepted")
	}
	login = newLoginCmd()
	_ = login.Flags().Set("git-name", "Name")
	_ = login.Flags().Set("email", "a@example.com")
	_ = login.Flags().Set("protocol", "ftp")
	if err := login.RunE(login, nil); err == nil {
		t.Fatal("invalid protocol accepted")
	}

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(filepath.Join(blocked, "child"))
	if err := newInitCmd().RunE(newInitCmd(), nil); err == nil {
		t.Fatal("init error ignored")
	}
	deps.Store = store
	// Exercise each interactive field's EOF path and protocol default.
	for _, input := range []string{"alias\n", "alias\nlogin\n", "alias\nlogin\nName\n", "alias\nlogin\nName\ne@example.com\n"} {
		deps.Stdin = strings.NewReader(input)
		manual = newAddCmd()
		_ = manual.Flags().Set("manual", "true")
		if err := manual.RunE(manual, nil); err == nil {
			t.Fatal("interactive EOF accepted")
		}
	}
	manual = newAddCmd()
	_ = manual.Flags().Set("manual", "true")
	deps.Stdin = strings.NewReader("alias\nlogin\nName\ne@example.com\n\n")
	if err := manual.RunE(manual, nil); err != nil {
		t.Fatal(err)
	}
	invalid := newAddCmd()
	for key, value := range map[string]string{"manual": "true", "alias": "bad", "hostname": "github.com", "git-name": "Name", "email": "bad", "protocol": "https"} {
		_ = invalid.Flags().Set(key, value)
	}
	deps.Stdin = strings.NewReader("login\n")
	if err := invalid.RunE(invalid, nil); err == nil {
		t.Fatal("invalid account accepted")
	}
}

func TestAutoCommandBranches(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	g.remote = ""
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil || !strings.Contains(err.Error(), "read origin") {
		t.Fatal(err)
	}
	g.remote = "bad"
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil || !strings.Contains(err.Error(), "supported GitHub remote") {
		t.Fatal(err)
	}
	g.remote = "https://github.com/owner/repo.git"
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("malformed accounts accepted")
	}
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("owner", config.Account{Login: "owner", Hostname: "github.com", GitName: "Owner", Email: "owner@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = loginOnlyGitHub{login: "owner"}
	deps.Stdin = strings.NewReader("2\nowner\nowner\nOwner\nhttps\n")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = gh
	gh.hasCredential = false

	deps.Stdin = strings.NewReader("3\n")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil || !strings.Contains(err.Error(), "invalid choice") {
		t.Fatal(err)
	}
	manual := newAutoCmd()
	for name, value := range map[string]string{"alias": "owner", "login": "owner", "git-name": "Owner", "email": "owner@example.com", "protocol": "https"} {
		_ = manual.Flags().Set(name, value)
	}
	deps.Stdin = strings.NewReader("2\n")
	if err := manual.RunE(manual, nil); err != nil {
		t.Fatal(err)
	}

	gh.credentialErr = errors.New("credential")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil || !strings.Contains(err.Error(), "check local") {
		t.Fatal(err)
	}
	gh.credentialErr = nil
	gh.hasCredential = false
	deps.Stdin = strings.NewReader("1\nowner@example.com\n")
	if err := store.RemoveAccount("owner"); err != nil {
		t.Fatal(err)
	}
	gh.login = "owner"
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestSyncUseRemoteAndSummaryErrors(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	account := addAlice(t, store)

	gh.loginErr = errors.New("login")
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("sync login error ignored")
	}
	gh.loginErr = nil
	gh.login = "nobody"
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil || !strings.Contains(err.Error(), "no configured account") {
		t.Fatal(err)
	}
	gh.login = "alice"
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "bad"}); err != nil {
		t.Fatal(err)
	}
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("bad scope accepted")
	}
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "local"}); err != nil {
		t.Fatal(err)
	}
	g.isRepoErr = errors.New("repo")
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("repo error ignored")
	}
	g.isRepoErr = nil
	g.isRepo = false
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("nonrepo sync accepted")
	}
	g.isRepo = true
	g.setIdentityErr = errors.New("identity")
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("identity error ignored")
	}
	g.setIdentityErr = nil
	// GitHub clients without the host extension use SwitchUser, and Git clients
	// without credential integration continue with a warning.
	deps.GitHub = loginOnlyGitHub{login: "alice"}
	deps.Git = noHelperGit{base: g}
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "global"}); err != nil {
		t.Fatal(err)
	}
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	deps.GitHub = gh
	deps.Git = g
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "global", UpdateRemote: true}); err != nil {
		t.Fatal(err)
	}
	g.remote = "https://github.com/alice/repo.git"
	g.setRemoteErr = errors.New("set")
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	g.setRemoteErr = nil

	g.isRepoErr = errors.New("repo")
	if err := updateOriginRemote(context.Background(), account); err == nil {
		t.Fatal("remote repo error ignored")
	}
	g.isRepoErr = nil
	g.isRepo = true
	g.remoteErr = errors.New("remote")
	if err := updateOriginRemote(context.Background(), account); err == nil {
		t.Fatal("remote read error ignored")
	}
	g.remoteErr = nil
	g.remote = "bad"
	if err := updateOriginRemote(context.Background(), account); err == nil {
		t.Fatal("bad remote accepted")
	}
	g.remote = "https://github.com/alice/repo.git"
	if err := updateOriginRemote(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	g.remote = "https://github.com/other/repo.git"
	g.setRemoteErr = errors.New("set")
	if err := updateOriginRemote(context.Background(), account); err == nil {
		t.Fatal("set remote error ignored")
	}
	g.setRemoteErr = nil

	// Exercise summary fallbacks, unknown repository, malformed and missing remotes.
	gh.login = "alice"
	g.identity = git.Identity{}
	if err := printCurrentSummary(nil, "", config.Account{}, git.ScopeLocal); err != nil {
		t.Fatal(err)
	}
	g.topErr = errors.New("top")
	g.branch = ""
	g.remote = "bad"
	if err := printCurrentSummary(nil, "alice", account, git.ScopeLocal); err != nil {
		t.Fatal(err)
	}
	g.topErr = nil
	g.remote = ""
	g.remoteErr = errors.New("remote")
	if err := printCurrentSummary(nil, "alice", account, git.ScopeLocal); err != nil {
		t.Fatal(err)
	}
}

func TestEditCredentialListAndRequiredInitializationErrors(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	addAlice(t, store)
	originalEdit := editFile
	t.Cleanup(func() { editFile = originalEdit })
	t.Setenv("EDITOR", "true")
	editFile = func(string, string) error { return errors.New("editor") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("editor error ignored")
	}
	editFile = func(_ string, path string) error { return os.WriteFile(path, []byte("invalid: ["), 0o600) }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil || !strings.Contains(err.Error(), "invalid account yaml") {
		t.Fatal(err)
	}
	editFile = func(_ string, path string) error { return os.WriteFile(path, []byte("login: alice\n"), 0o600) }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatal(err)
	}

	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newListCmd().RunE(newListCmd(), nil); err == nil {
		t.Fatal("list malformed accounts accepted")
	}
	deps.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	helper := newCredentialHelperCmd()
	_ = helper.Flags().Set("account-key", "github.com|alice")
	if err := helper.RunE(helper, []string{"get"}); err == nil {
		t.Fatal("credential malformed auth expected")
	}

	missing := config.NewStore(t.TempDir())
	deps.Store = missing
	for _, command := range []func() error{
		func() error { return newListCmd().RunE(newListCmd(), nil) },
		func() error { return newRemoveCmd().RunE(newRemoveCmd(), []string{"alice"}) },
		func() error { return newUseCmd().RunE(newUseCmd(), []string{"alice"}) },
		func() error { return newSyncCmd().RunE(newSyncCmd(), nil) },
	} {
		if err := command(); err == nil {
			t.Fatal("uninitialized command succeeded")
		}
	}
}
