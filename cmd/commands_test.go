package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/remote"
)

type commandGit struct {
	isRepo                                                  bool
	identity                                                git.Identity
	remote                                                  string
	branch                                                  string
	top                                                     string
	setHelper, setRemoteErr, setIdentityErr                 error
	isRepoErr, topErr, getIdentityErr, remoteErr, branchErr error
}

type noHelperGit struct{ base *commandGit }

func (g noHelperGit) IsRepo(c context.Context) (bool, error)     { return g.base.IsRepo(c) }
func (g noHelperGit) TopLevel(c context.Context) (string, error) { return g.base.TopLevel(c) }
func (g noHelperGit) GetIdentity(c context.Context, s git.Scope) (git.Identity, error) {
	return g.base.GetIdentity(c, s)
}
func (g noHelperGit) SetIdentity(c context.Context, s git.Scope, i git.Identity) error {
	return g.base.SetIdentity(c, s, i)
}
func (g noHelperGit) GetRemoteURL(c context.Context, n string) (string, error) {
	return g.base.GetRemoteURL(c, n)
}
func (g noHelperGit) SetRemoteURL(c context.Context, n, u string) error {
	return g.base.SetRemoteURL(c, n, u)
}
func (g noHelperGit) CurrentBranch(c context.Context) (string, error) { return g.base.CurrentBranch(c) }

func (g *commandGit) IsRepo(context.Context) (bool, error)     { return g.isRepo, g.isRepoErr }
func (g *commandGit) TopLevel(context.Context) (string, error) { return g.top, g.topErr }
func (g *commandGit) GetIdentity(context.Context, git.Scope) (git.Identity, error) {
	return g.identity, g.getIdentityErr
}
func (g *commandGit) SetIdentity(_ context.Context, _ git.Scope, i git.Identity) error {
	if g.setIdentityErr == nil {
		g.identity = i
	}
	return g.setIdentityErr
}
func (g *commandGit) GetRemoteURL(context.Context, string) (string, error) {
	if g.remoteErr != nil {
		return "", g.remoteErr
	}
	if g.remote == "" {
		return "", errors.New("no remote")
	}
	return g.remote, nil
}
func (g *commandGit) SetRemoteURL(_ context.Context, _ string, u string) error {
	if g.setRemoteErr == nil {
		g.remote = u
	}
	return g.setRemoteErr
}
func (g *commandGit) CurrentBranch(context.Context) (string, error) { return g.branch, g.branchErr }
func (g *commandGit) SetCredentialHelper(context.Context, git.Scope, string, string) error {
	return g.setHelper
}

type commandGitHub struct {
	login, host                                             string
	hasCredential                                           bool
	clientID                                                string
	loginErr, switchErr, oauthErr, logoutErr, credentialErr error
}

type loginOnlyGitHub struct {
	login string
	err   error
}

type identityHookGitHub struct {
	*commandGitHub
	hook        func()
	identityErr error
}

func (g identityHookGitHub) CurrentIdentity(context.Context) (string, string, error) {
	if g.hook != nil {
		g.hook()
	}
	return g.login, g.host, g.identityErr
}

func (g loginOnlyGitHub) CurrentLogin(context.Context) (string, error) { return g.login, g.err }
func (loginOnlyGitHub) SwitchUser(context.Context, string) error       { return nil }
func (loginOnlyGitHub) Status(context.Context) (string, error)         { return "", nil }
func (loginOnlyGitHub) Login(context.Context, string, string) error    { return nil }
func (loginOnlyGitHub) Logout(context.Context, string, string) error   { return nil }

func (g *commandGitHub) CurrentLogin(context.Context) (string, error) { return g.login, g.loginErr }
func (g *commandGitHub) CurrentIdentity(context.Context) (string, string, error) {
	return g.login, g.host, g.loginErr
}
func (g *commandGitHub) SwitchUser(context.Context, string) error               { return g.switchErr }
func (g *commandGitHub) SwitchUserAtHost(context.Context, string, string) error { return g.switchErr }
func (g *commandGitHub) Status(context.Context) (string, error)                 { return "ok", nil }
func (g *commandGitHub) Login(context.Context, string, string) error            { return g.oauthErr }
func (g *commandGitHub) Logout(context.Context, string, string) error           { return g.logoutErr }
func (g *commandGitHub) HasCredential(context.Context, string, string) (bool, error) {
	return g.hasCredential, g.credentialErr
}
func (g *commandGitHub) ConfiguredClientID() string { return g.clientID }
func (g *commandGitHub) SetClientID(id string)      { g.clientID = id }

func commandSetup(t *testing.T) (*config.Store, *commandGit, *commandGitHub, *bytes.Buffer) {
	t.Helper()
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "global", UpdateRemote: true}); err != nil {
		t.Fatal(err)
	}
	out := new(bytes.Buffer)
	g := &commandGit{isRepo: true, top: "/repo", branch: "main", remote: "https://github.com/alice/repo.git"}
	gh := &commandGitHub{login: "alice", host: "github.com", hasCredential: true, clientID: "client"}
	deps = Dependencies{Store: store, Git: g, GitHub: gh, Stdout: out, Stderr: out, Stdin: strings.NewReader("")}
	flagGlobal, flagUpdateRemote, flagNoBrowser, flagClientID, flagConfigDir = false, false, false, "", ""
	return store, g, gh, out
}

func runCommand(t *testing.T, c *cobra.Command, args ...string) error {
	t.Helper()
	c.SetArgs(args)
	c.SetIn(deps.Stdin)
	c.SetOut(deps.Stdout)
	c.SetErr(deps.Stderr)
	if err := c.ParseFlags(args); err != nil {
		return err
	}
	return c.RunE(c, c.Flags().Args())
}

func TestCommandLifecycle(t *testing.T) {
	store, gitClient, gh, out := commandSetup(t)
	if err := runCommand(t, newInitCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newListCmd()); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("alice", config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newListCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newUseCmd(), "alice"); err != nil {
		t.Fatal(err)
	}
	if gitClient.identity.Name != "Alice" || !strings.Contains(gitClient.remote, "alice") {
		t.Fatalf("use did not sync: %+v", gitClient)
	}
	if err := runCommand(t, newCurrentCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newRemoteCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newSyncCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newDoctorCmd()); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newLogoutCmd(), "alice"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t, newRemoveCmd(), "alice"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Current") {
		t.Fatal(out.String())
	}
	_ = gh
}

func TestCommandAddLoginAutoAndErrors(t *testing.T) {
	store, _, gh, out := commandSetup(t)
	deps.Stdin = strings.NewReader("alice\nalice\nAlice\nalice@example.com\nssh\n")
	if err := runCommand(t, newAddCmd(), "--manual"); err != nil {
		t.Fatal(err)
	}
	account, err := store.GetAccount("alice")
	if err != nil || account.Protocol != "ssh" {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	deps.Stdin = strings.NewReader("Bob\nbob@example.com\n")
	if err := runCommand(t, newLoginCmd(), "--git-name", "Bob", "--email", "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAccount("alice"); err != nil {
		t.Fatal(err)
	}
	gh.login = "alice"
	if err := runCommand(t, newAutoCmd()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Saved account") || !strings.Contains(out.String(), "Updated git identity") {
		t.Fatal(out.String())
	}

	gh.oauthErr = errors.New("oauth")
	if err := runCommand(t, newLoginCmd(), "--git-name", "X", "--email", "x@y"); err == nil {
		t.Fatal("login error ignored")
	}
	gh.oauthErr = nil
	gh.loginErr = errors.New("identity")
	if err := runCommand(t, newLoginCmd(), "--git-name", "X", "--email", "x@y"); err == nil {
		t.Fatal("identity error ignored")
	}
	gh.loginErr = nil
	if err := runCommand(t, newUseCmd(), "missing"); err == nil {
		t.Fatal("missing use accepted")
	}
	if err := runCommand(t, newRemoveCmd(), "missing"); err == nil {
		t.Fatal("missing remove accepted")
	}
}

func TestRootHelpersAndCompletion(t *testing.T) {
	_, g, gh, out := commandSetup(t)
	if commandContext(nil) == nil {
		t.Fatal("nil context")
	}
	if err := requireInitialized(); err != nil {
		t.Fatal(err)
	}
	gh.SetClientID("")
	if err := ensureOAuthClientID(bufio.NewReader(bytes.NewBufferString("\n"))); err == nil {
		t.Fatal("empty client id accepted")
	}
	deps.GitHub = gh
	if err := ensureOAuthClientID(bufio.NewReader(bytes.NewBufferString("new-client\n"))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Client ID") {
		t.Fatal(out.String())
	}
	_, _ = completeAccountAliases(nil, nil, "")
	versionCmd := newVersionCmd()
	var versionOut bytes.Buffer
	versionCmd.SetOut(&versionOut)
	versionCmd.Run(versionCmd, nil)
	if !strings.Contains(versionOut.String(), "gha") {
		t.Fatal(versionOut.String())
	}
	root := NewRootCommand()
	if root == nil || len(root.Commands()) == 0 {
		t.Fatal("root command tree empty")
	}
	if _, err := resolveScope(config.ConfigFile{DefaultScope: "bad"}); err == nil {
		t.Fatal("bad scope accepted")
	}
	flagGlobal = true
	if scope, err := resolveScope(config.ConfigFile{}); err != nil || scope != git.ScopeGlobal {
		t.Fatalf("scope=%v err=%v", scope, err)
	}
	flagGlobal = false
	if err := updateOriginRemote(context.Background(), config.Account{Login: "alice", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	g.isRepo = false
	if err := updateOriginRemote(context.Background(), config.Account{}); err == nil {
		t.Fatal("nonrepo accepted")
	}
}

func TestPromptAndCloneHelpers(t *testing.T) {
	_, _, _, _ = commandSetup(t)
	if got, err := readValue(bufio.NewReader(bytes.NewBufferString(" value\n")), "x", ""); err != nil || got != "value" {
		t.Fatalf("readValue=%q %v", got, err)
	}
	if got, err := readValue(bufio.NewReader(bytes.NewBuffer(nil)), "x", ""); err == nil || got != "" {
		t.Fatalf("EOF=%q %v", got, err)
	}
	if got := suggestAlias(" Alice "); got != "alice" {
		t.Fatal(got)
	}
	if got := suggestAlias(" "); got != "" {
		t.Fatal(got)
	}
	if got := promptWithDefault(bufio.NewReader(strings.NewReader("\n")), "x", "default"); got != "default" {
		t.Fatal(got)
	}
	if got := promptWithDefault(bufio.NewReader(strings.NewReader("value\n")), "x", ""); got != "value" {
		t.Fatal(got)
	}
	if _, err := cloneDestination("x", remote.Info{}); err == nil {
		t.Fatal("empty destination accepted")
	}
	if got, err := cloneDestination("x", remote.Info{Repo: "repo"}); err != nil || !strings.HasSuffix(got, "repo") {
		t.Fatalf("destination=%q %v", got, err)
	}
	if hasCredential(context.Background(), &commandGitHub{hasCredential: true}, config.Account{}) != true {
		t.Fatal("hasCredential false")
	}
	deps.GitHub = &commandGitHub{hasCredential: false}
	if err := requireAccountCredential(context.Background(), config.Account{Login: "a"}); err == nil {
		t.Fatal("missing checker accepted")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write") }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read") }

func TestCommandErrorBranchesAndSetup(t *testing.T) {
	store, g, gh, out := commandSetup(t)
	// Root setup and Execute exercise dependency construction and the version path.
	oldArgs := os.Args
	os.Args = []string{"gha", "version", "--config-dir", store.Dir, "--client-id", "client"}
	if err := Execute(); err != nil {
		t.Fatal(err)
	}
	os.Args = oldArgs
	cmd := NewRootCommand()
	flagConfigDir = "~/.config/gha"
	flagClientID, flagNoBrowser = "client", true
	if err := setupDeps(cmd); err != nil {
		t.Fatal(err)
	}
	store, g, gh, out = commandSetup(t)
	flagConfigDir, flagNoBrowser = "", false

	// Current summary tolerates unavailable GitHub, git and repository details.
	gh.loginErr, g.getIdentityErr, g.isRepoErr, g.topErr, g.branchErr, g.remoteErr = errors.New("login"), errors.New("identity"), errors.New("repo"), errors.New("top"), errors.New("branch"), errors.New("remote")
	if err := printCurrentSummary(cmd, "", config.Account{}, git.ScopeLocal); err != nil {
		t.Fatal(err)
	}
	gh.loginErr, g.getIdentityErr, g.isRepoErr, g.topErr, g.branchErr, g.remoteErr = nil, nil, nil, nil, nil, nil
	if err := store.UpsertAccount("alice", config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}

	gh.switchErr = errors.New("switch")
	if err := runCommand(t, newUseCmd(), "alice"); err != nil {
		t.Fatal(err)
	}
	gh.switchErr = nil
	if err := store.SaveConfig(config.ConfigFile{DefaultScope: "local"}); err != nil {
		t.Fatal(err)
	}
	g.isRepo = false
	if err := runCommand(t, newUseCmd(), "alice"); err == nil {
		t.Fatal("local use outside repo accepted")
	}
	g.isRepo = true
	g.setHelper = errors.New("helper")
	if err := runCommand(t, newUseCmd(), "alice"); err == nil {
		t.Fatal("helper error ignored")
	}
	g.setHelper = nil
	g.setIdentityErr = errors.New("identity")
	if err := runCommand(t, newUseCmd(), "alice"); err == nil {
		t.Fatal("identity error ignored")
	}
	g.setIdentityErr = nil

	gh.logoutErr = errors.New("logout")
	if err := runCommand(t, newLogoutCmd()); err == nil {
		t.Fatal("logout error ignored")
	}
	gh.logoutErr = nil
	if err := runCommand(t, newLogoutCmd(), "missing"); err == nil {
		t.Fatal("missing logout accepted")
	}

	// Remote command handles malformed and missing origins.
	g.remote = "not-a-remote"
	if err := runCommand(t, newRemoteCmd()); err != nil {
		t.Fatal(err)
	}
	g.remote = ""
	if err := runCommand(t, newRemoteCmd()); err == nil {
		t.Fatal("missing remote accepted")
	}
	g.remote = "https://github.com/alice/repo.git"

	// Credential helper protocol branches.
	deps.Stdin = strings.NewReader("protocol=ssh\nhost=github.com\n\n")
	if err := runCommand(t, newCredentialHelperCmd(), "get"); err != nil {
		t.Fatal(err)
	}
	deps.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	ch := newCredentialHelperCmd()
	_ = ch.Flags().Set("account-key", "bad")
	if err := ch.RunE(ch, []string{"get"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCredentialResponse(failingWriter{}, "https", config.Credential{}); err == nil {
		t.Fatal("writer error ignored")
	}
	if _, err := readCredentialRequest(failingReader{}); err == nil {
		t.Fatal("reader error ignored")
	}

	// Completion command's argument validation and all supported shells.
	completion := newCompletionCmd()
	completion.SetOut(io.Discard)
	if err := completion.Args(completion, nil); err == nil {
		t.Fatal("completion accepted missing shell")
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell", "unknown"} {
		if err := completion.RunE(completion, []string{shell}); err != nil && shell != "unknown" {
			t.Fatal(shell, err)
		}
	}
	_ = out
}

func TestAutoAndCloneSelectionBranches(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	if got, err := promptAutoChoice(bufio.NewReader(strings.NewReader("\n"))); err != nil || got != "1" {
		t.Fatalf("choice=%q %v", got, err)
	}
	if got, err := promptAutoChoice(bufio.NewReader(strings.NewReader("3\n"))); err != nil || got != "3" {
		t.Fatalf("choice=%q %v", got, err)
	}
	if _, err := promptAutoChoice(bufio.NewReader(strings.NewReader(""))); err == nil {
		t.Fatal("EOF choice accepted")
	}
	info, _ := remote.Parse("https://github.com/owner/repo.git")
	if _, _, found, err := accountForRemote(info, ""); err != nil || found {
		t.Fatalf("no match=%v %v", found, err)
	}
	if err := store.UpsertAccount("one", config.Account{Login: "owner", Hostname: "github.com", GitName: "O", Email: "o@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("two", config.Account{Login: "owner", Hostname: "github.com", GitName: "O", Email: "o@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := accountForRemote(info, ""); err == nil {
		t.Fatal("ambiguous remote accepted")
	}
	if alias, _, found, err := accountForRemote(info, "one"); err != nil || !found || alias != "one" {
		t.Fatalf("preferred=%q %v %v", alias, found, err)
	}
	gh.hasCredential = true
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), info, "missing", true); err == nil {
		t.Fatal("missing explicit account accepted")
	}
	if _, _, err := selectCloneAccount(context.Background(), newCloneCmd(), info, "one", true); err != nil {
		t.Fatal(err)
	}
	g.remote = info.Raw
	cloneOld := cloneGit
	cloneGit = func(context.Context, string, string, io.Writer, io.Writer) error {
		return errors.New("Authentication failed")
	}
	if err := cloneWithAccount(context.Background(), info.Raw, info, config.Account{Login: "owner", Hostname: "github.com"}); err == nil {
		t.Fatal("clone error ignored")
	}
	cloneGit = cloneOld
	_ = g
}

func TestEditAndIdentityHelpers(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	if err := store.UpsertAccount("alice", config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", "true")
	if got := resolveEditor(); got == "" {
		t.Fatal("editor not resolved")
	}
	if err := openInEditor("", "x"); err == nil {
		t.Fatal("empty editor accepted")
	}
	if err := openInEditor("definitely-missing-editor", "x"); err == nil {
		t.Fatal("missing editor accepted")
	}
	edit := newEditCmd()
	if err := edit.RunE(edit, []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if err := edit.Flags().Set("all", "true"); err != nil {
		t.Fatal(err)
	}
	if err := edit.RunE(edit, nil); err != nil {
		t.Fatal(err)
	}
	if login, host, err := currentGitHubIdentity(context.Background()); err != nil || login != gh.login || host != gh.host {
		t.Fatalf("identity=%q %q %v", login, host, err)
	}
	deps.GitHub = loginOnlyGitHub{login: "fallback"}
	if login, host, err := currentGitHubIdentity(context.Background()); err != nil || login != "fallback" || host != "github.com" {
		t.Fatal("missing identity client accepted")
	}
}

func TestAddLoginAndAutoInteractiveFlows(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	add := newAddCmd()
	for name, value := range map[string]string{"alias": "web", "git-name": "Web", "email": "web@example.com", "protocol": "ssh"} {
		if err := add.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	gh.login = "web"
	if err := add.RunE(add, nil); err != nil {
		t.Fatal(err)
	}
	if account, err := store.GetAccount("web"); err != nil || account.Login != "web" || account.Protocol != "ssh" {
		t.Fatalf("account=%+v %v", account, err)
	}

	// Existing aliases inherit stored host/name/email/protocol and report update.
	login := newLoginCmd()
	gh.login = "web"
	if err := login.RunE(login, []string{"web"}); err != nil {
		t.Fatal(err)
	}

	info, _ := remote.Parse("https://github.com/web/repo.git")
	deps.Stdin = strings.NewReader("web@example.com\n")
	alias, err := runLoginForAuto(newAutoCmd(), info, bufio.NewReader(deps.Stdin))
	if err != nil || alias != "web" {
		t.Fatalf("runLoginForAuto=%q %v", alias, err)
	}
	deps.Stdin = strings.NewReader("web@example.com\n")
	alias, err = runLoginForClone(newCloneCmd(), info)
	if err != nil || alias != "web" {
		t.Fatalf("runLoginForClone=%q %v", alias, err)
	}

	reader := bufio.NewReader(strings.NewReader("\n\n\nemail@example.com\n\n"))
	alias, account, err := promptAutoAccount(reader, info, "", config.Account{}, "", "", "", "", "")
	if err != nil || alias != "web" || account.Login != "web" || account.Protocol != "https" {
		t.Fatalf("prompt account=%q %+v %v", alias, account, err)
	}
	alias, account, err = promptAutoAccount(bufio.NewReader(strings.NewReader("")), info, "existing", config.Account{Login: "web", GitName: "Name", Email: "e@example.com", Protocol: "ssh"}, "flag", "login", "Git", "f@example.com", "ssh")
	if err != nil || alias != "flag" || account.Email != "f@example.com" {
		t.Fatalf("flag account=%q %+v %v", alias, account, err)
	}
	if _, _, err := promptAutoAccount(bufio.NewReader(strings.NewReader("")), remote.Info{}, "", config.Account{}, "", "", "", "", ""); err == nil {
		t.Fatal("empty login accepted")
	}
}

func TestCloneCommandEndToEndAndSelection(t *testing.T) {
	store, _, gh, out := commandSetup(t)
	account := config.Account{Login: "alice", Hostname: "github.com", GitName: "Alice", Email: "alice@example.com", Protocol: "https"}
	if err := store.UpsertAccount("alice", account); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	originalClone := cloneGit
	t.Cleanup(func() { cloneGit = originalClone })
	cloneGit = func(_ context.Context, _ string, _ string, _ io.Writer, _ io.Writer) error {
		if err := os.MkdirAll(filepath.Join("repo", ".git"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join("repo", ".git", "config"), nil, 0o644)
	}
	clone := newCloneCmd()
	if err := clone.RunE(clone, []string{"https://github.com/alice/repo.git", "use", "alice"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cloned") {
		t.Fatal(out.String())
	}

	for _, args := range [][]string{{}, {"a", "b", "c", "d"}, {"a", "bad"}, {"a", "bad", "x"}} {
		if err := clone.Args(clone, args); err == nil {
			t.Errorf("invalid clone args accepted: %v", args)
		}
	}
	if values, _ := clone.ValidArgsFunction(clone, []string{"repo", "use"}, ""); values == nil {
		t.Fatal("alias completion missing")
	}

	info, _ := remote.Parse("https://github.com/org/repo.git")
	gh.hasCredential = false
	if _, _, err := selectCloneAccount(context.Background(), clone, info, "alice", true); err == nil {
		t.Fatal("logged-out explicit account accepted")
	}
	gh.hasCredential = true
	if _, _, err := selectCloneAccount(context.Background(), clone, info, "alice", true); err != nil {
		t.Fatal(err)
	}
	if err := requireAccountCredential(context.Background(), account); err != nil {
		t.Fatal(err)
	}

	cloneGit = func(context.Context, string, string, io.Writer, io.Writer) error { return errors.New("clone") }
	if err := cloneWithAccount(context.Background(), "repo", remote.Info{Protocol: "ssh"}, account); err == nil {
		t.Fatal("plain clone error ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runGitClone(ctx, "https://invalid.example/repo.git", "helper", io.Discard, io.Discard); err == nil {
		t.Fatal("cancelled git clone succeeded")
	}
}
