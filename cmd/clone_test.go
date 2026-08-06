package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/remote"
)

type cloneTestGitHub struct {
	credentials map[string]bool
}

func (c cloneTestGitHub) CurrentLogin(context.Context) (string, error) { return "personal", nil }
func (c cloneTestGitHub) CurrentIdentity(context.Context) (string, string, error) {
	return "personal", "github.com", nil
}
func (c cloneTestGitHub) SwitchUser(context.Context, string) error     { return nil }
func (c cloneTestGitHub) Status(context.Context) (string, error)       { return "", nil }
func (c cloneTestGitHub) Login(context.Context, string, string) error  { return nil }
func (c cloneTestGitHub) Logout(context.Context, string, string) error { return nil }
func (c cloneTestGitHub) HasCredential(_ context.Context, login, hostname string) (bool, error) {
	return c.credentials[strings.ToLower(hostname)+"|"+strings.ToLower(login)], nil
}

func TestCloneWithAccountConfiguresLocalRepository(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("personal", config.Account{
		Login: "personal", Hostname: "github.com", GitName: "Personal", Email: "personal@example.com", Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}
	oldDeps := deps
	oldClone := cloneGit
	defer func() { deps = oldDeps; cloneGit = oldClone }()
	deps = Dependencies{
		Store:  store,
		GitHub: cloneTestGitHub{credentials: map[string]bool{"github.com|personal": true}},
		Stdout: io.Discard,
		Stderr: io.Discard,
		Stdin:  strings.NewReader(""),
	}
	destination := filepath.Join(t.TempDir(), "repo")
	cloneGit = func(_ context.Context, _ string, _ string, _ io.Writer, _ io.Writer) error {
		if err := os.MkdirAll(filepath.Join(destination, ".git"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, ".git", "config"), nil, 0o644)
	}

	info, err := remote.Parse("https://github.com/personal/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	alias, account, err := selectCloneAccount(context.Background(), nil, info, "personal", true)
	if err != nil || alias != "personal" {
		t.Fatalf("selectCloneAccount = %q, %+v, %v", alias, account, err)
	}
	if err := cloneWithAccount(context.Background(), info.Raw, info, account); err != nil {
		t.Fatal(err)
	}

	localGit := git.NewNativeClient(destination)
	if err := localGit.SetIdentity(context.Background(), git.ScopeLocal, git.Identity{Name: account.GitName, Email: account.Email}); err != nil {
		t.Fatal(err)
	}
	if err := localGit.SetCredentialHelper(context.Background(), git.ScopeLocal, "!'gha' credential-helper", "github.com|personal"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name = Personal") || !strings.Contains(string(data), "email = personal@example.com") {
		t.Fatalf("local identity not written: %s", data)
	}
}

func TestCloneCommandAcceptsAutoAndUseSyntax(t *testing.T) {
	cmd := newCloneCmd()
	if err := cmd.Args(cmd, []string{"https://github.com/owner/repo.git"}); err != nil {
		t.Fatalf("auto syntax rejected: %v", err)
	}
	if err := cmd.Args(cmd, []string{"https://github.com/owner/repo.git", "auto"}); err != nil {
		t.Fatalf("explicit auto syntax rejected: %v", err)
	}
	if err := cmd.Args(cmd, []string{"https://github.com/owner/repo.git", "use", "personal"}); err != nil {
		t.Fatalf("use syntax rejected: %v", err)
	}
}
