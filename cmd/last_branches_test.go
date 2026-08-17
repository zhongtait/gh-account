package cmd

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestRemainingAddAutoAndEditBranches(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	blocked := t.TempDir() + "/blocked"
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(blocked + "/child")
	if err := newAddCmd().RunE(newAddCmd(), nil); err == nil {
		t.Fatal("add initialization error ignored")
	}
	deps.Store = store
	gh.clientID = ""
	deps.Stdin = strings.NewReader("")
	if err := newAddCmd().RunE(newAddCmd(), nil); err == nil {
		t.Fatal("add client ID EOF ignored")
	}
	gh.clientID = "client"
	gh.login = "alice"
	for _, input := range []string{"alias\n", "alias\nName\n", "alias\nName\ne@example.com\n"} {
		deps.Stdin = strings.NewReader(input)
		add := newAddCmd()
		if err := add.RunE(add, nil); err == nil {
			t.Fatal("noninteractive prompt EOF ignored")
		}
	}
	deps.Stdin = strings.NewReader("alias\nName\ne@example.com\n\n")
	if err := newAddCmd().RunE(newAddCmd(), nil); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(blocked + "/login")
	if err := newLoginCmd().RunE(newLoginCmd(), nil); err == nil {
		t.Fatal("login initialization error ignored")
	}
	deps.Store = store
	gh.clientID = ""
	deps.Stdin = strings.NewReader("")
	if err := newLoginCmd().RunE(newLoginCmd(), nil); err == nil {
		t.Fatal("login client ID error ignored")
	}
	gh.clientID = "client"
	deps.Stdin = strings.NewReader("\n")
	login := newLoginCmd()
	_ = login.Flags().Set("git-name", "Name")
	if err := login.RunE(login, nil); err == nil || !strings.Contains(err.Error(), "email is required") {
		t.Fatal(err)
	}

	// Auto requires initialization and propagates prompt, login and persistence errors.
	deps.Store = config.NewStore(blocked + "/auto")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("auto initialization error ignored")
	}
	deps.Store = store
	g.remote = "https://github.com/owner/repo.git"
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	deps.Stdin = strings.NewReader("")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("auto prompt EOF ignored")
	}
	deps.Stdin = strings.NewReader("1\n")
	gh.oauthErr = errors.New("oauth")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("auto login error ignored")
	}
	gh.oauthErr = nil
	deps.Stdin = strings.NewReader("2\n")
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("auto account prompt EOF ignored")
	}

	// Edit initialization, editor, missing account, protocol default and save errors.
	deps.Store = config.NewStore(blocked + "/edit")
	edit := newEditCmd()
	if err := edit.RunE(edit, []string{"alice"}); err == nil {
		t.Fatal("edit initialization error ignored")
	}
	deps.Store = store
	oldEdit := editFile
	t.Cleanup(func() { editFile = oldEdit })
	t.Setenv("EDITOR", "")
	findEditor = func(string) (string, error) { return "", errors.New("missing") }
	if err := newEditCmd().RunE(newEditCmd(), nil); err == nil {
		t.Fatal("missing editor accepted")
	}
	findEditor = func(string) (string, error) { return "/bin/true", nil }
	if err := newEditCmd().RunE(newEditCmd(), []string{"missing"}); err == nil {
		t.Fatal("missing account edit accepted")
	}
	addAlice(t, store)
	editFile = func(_ string, path string) error {
		return os.WriteFile(path, []byte("login: alice\ngit_name: Alice\nemail: a@example.com\n"), 0o600)
	}
	t.Setenv("EDITOR", "true")
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	originalSaveAccount := editSaveAccount
	t.Cleanup(func() { editSaveAccount = originalSaveAccount })
	editSaveAccount = func(*config.Store, string, config.Account) error { return errors.New("save") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("edit save error ignored")
	}
	editFile = func(string, string) error { return errors.New("all editor") }
	all := newEditCmd()
	_ = all.Flags().Set("all", "true")
	if err := all.RunE(all, nil); err == nil {
		t.Fatal("all editor error ignored")
	}

	_ = bufio.NewReader
	_ = git.ScopeLocal
	_ = utils.AuthPath
	_ = context.Background
}

func TestRemainingSyncUseAndCompletionBranches(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	addAlice(t, store)
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("sync malformed accounts accepted")
	}
	accounts := config.AccountsFile{Accounts: map[string]config.Account{"alice": {Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "a@example.com", Protocol: "https"}}}
	if err := store.SaveAccounts(accounts); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utils.ConfigPath(store.Dir), []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newSyncCmd().RunE(newSyncCmd(), nil); err == nil {
		t.Fatal("sync malformed config accepted")
	}
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err == nil {
		t.Fatal("use malformed config accepted")
	}
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "local"}); err != nil {
		t.Fatal(err)
	}
	g.isRepoErr = errors.New("repo")
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err == nil {
		t.Fatal("use repo error ignored")
	}
	g.isRepoErr = nil
	g.isRepo = true
	gh.switchErr = errors.New("switch")
	if err := newUseCmd().RunE(newUseCmd(), []string{"alice"}); err != nil {
		t.Fatal(err)
	}

	deps = Dependencies{}
	root := NewRootCommand()
	flagConfigDir = store.Dir
	aliases, directive := completeAccountAliases(root, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp || len(aliases) != 1 {
		t.Fatalf("aliases=%v directive=%v", aliases, directive)
	}
}

func TestRemainingCredentialDoctorRemoteLogout(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	addAlice(t, store)
	deps.Stdin = strings.NewReader("protocol=ssh\nhost=github.com\n\n")
	if err := newCredentialHelperCmd().RunE(newCredentialHelperCmd(), []string{"get"}); err != nil {
		t.Fatal(err)
	}
	deps.Stdin = failingReader{}
	if err := newCredentialHelperCmd().RunE(newCredentialHelperCmd(), []string{"get"}); err == nil {
		t.Fatal("credential reader error ignored")
	}
	deps.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	for _, key := range []string{"|alice", "github.com|", "github.com|missing", "ghe.example|alice"} {
		helper := newCredentialHelperCmd()
		_ = helper.Flags().Set("account-key", key)
		if err := helper.RunE(helper, []string{"get"}); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable, originalAbs := credentialExecutable, credentialAbs
	t.Cleanup(func() { credentialExecutable, credentialAbs = originalExecutable, originalAbs })
	credentialExecutable = func() (string, error) { return "", errors.New("exe") }
	credentialAbs = func(string) (string, error) { return "", errors.New("abs") }
	_ = credentialHelperCommand(store, config.Account{})

	g.isRepoErr = errors.New("repo")
	if err := newRemoteCmd().RunE(newRemoteCmd(), nil); err == nil {
		t.Fatal("remote repo error ignored")
	}
	g.isRepoErr = nil
	g.isRepo = false
	if err := newRemoteCmd().RunE(newRemoteCmd(), nil); err == nil {
		t.Fatal("remote outside repo accepted")
	}
	g.isRepo = true
	gh.logoutErr = errors.New("logout")
	if err := newLogoutCmd().RunE(newLogoutCmd(), nil); err == nil {
		t.Fatal("logout error ignored")
	}
	gh.logoutErr = nil
	blocked := t.TempDir() + "/blocked"
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(blocked + "/child")
	if err := newLogoutCmd().RunE(newLogoutCmd(), []string{"alice"}); err == nil {
		t.Fatal("logout initialization error ignored")
	}

	deps.Store = store
	// Doctor renders both healthy and unhealthy reports.
	if err := newDoctorCmd().RunE(newDoctorCmd(), nil); err != nil {
		t.Fatal(err)
	}
	deps.Store = config.NewStore(t.TempDir())
	if err := newDoctorCmd().RunE(newDoctorCmd(), nil); err != nil {
		t.Fatal(err)
	}
	deps.Store = store
	gh.logoutErr = nil
	if err := newLogoutCmd().RunE(newLogoutCmd(), nil); err != nil {
		t.Fatal(err)
	}
	deps.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	helper := newCredentialHelperCmd()
	_ = helper.Flags().Set("account-key", "github.com|missing")
	if err := helper.RunE(helper, []string{"get"}); err != nil {
		t.Fatal(err)
	}
}
