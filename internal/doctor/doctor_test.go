package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
)

type fakeGit struct {
	repo      bool
	repoErr   error
	remoteErr error
}

func (f fakeGit) IsRepo(ctx context.Context) (bool, error)     { return f.repo, f.repoErr }
func (f fakeGit) TopLevel(ctx context.Context) (string, error) { return "/repo", nil }
func (f fakeGit) GetIdentity(ctx context.Context, scope git.Scope) (git.Identity, error) {
	return git.Identity{}, nil
}
func (f fakeGit) SetIdentity(ctx context.Context, scope git.Scope, identity git.Identity) error {
	return nil
}
func (f fakeGit) SetCredentialHelper(ctx context.Context, scope git.Scope, command, accountKey string) error {
	return nil
}
func (f fakeGit) GetRemoteURL(ctx context.Context, name string) (string, error) {
	if f.remoteErr != nil {
		return "", f.remoteErr
	}
	if f.repo {
		return "https://github.com/a/b.git", nil
	}
	return "", errors.New("no remote")
}
func (f fakeGit) SetRemoteURL(ctx context.Context, name, url string) error { return nil }
func (f fakeGit) CurrentBranch(ctx context.Context) (string, error)        { return "main", nil }

type fakeGH struct {
	login string
	err   error
}

func (f fakeGH) CurrentLogin(ctx context.Context) (string, error) { return f.login, f.err }
func (fakeGH) SwitchUser(ctx context.Context, login string) error { return nil }
func (fakeGH) Status(ctx context.Context) (string, error)         { return "", nil }
func (fakeGH) Login(ctx context.Context, hostname string, gitProtocol string) error {
	return nil
}
func (fakeGH) Logout(ctx context.Context, login string, hostname string) error {
	return nil
}

func TestDoctorHealthy(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStore(dir)
	if err := store.EnsureInitialized(); err != nil {
		t.Fatalf("init: %v", err)
	}

	report := Runner{
		Store:  store,
		Git:    fakeGit{repo: true},
		GitHub: fakeGH{login: "personal-user"},
	}.Run(context.Background())

	if !report.Healthy() {
		t.Fatalf("expected healthy report: %+v", report)
	}
}

func TestDoctorUnhealthyPaths(t *testing.T) {
	invalidStore := config.NewStore(t.TempDir())
	if err := invalidStore.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := invalidStore.UpsertAccount("valid", config.Account{
		Login: "valid", GitName: "Valid", Email: "valid@example.com", Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}
	tests := []Runner{
		{},
		{Store: config.NewStore(t.TempDir())},
		{Store: invalidStore, Git: fakeGit{repoErr: errors.New("repo failed")}},
		{Store: invalidStore, Git: fakeGit{repo: false}},
		{Store: invalidStore, Git: fakeGit{repo: true, remoteErr: errors.New("no remote")}},
		{Store: invalidStore, GitHub: fakeGH{err: errors.New("offline")}},
		{Store: invalidStore, GitHub: fakeGH{}},
	}
	for index, runner := range tests {
		report := runner.Run(context.Background())
		if report.Healthy() {
			t.Errorf("case %d unexpectedly healthy: %+v", index, report)
		}
	}
}

func TestDoctorInvalidAccounts(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "accounts.yaml"), []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Runner{Store: store}.Run(context.Background())
	if report.Healthy() {
		t.Fatalf("invalid config reported healthy: %+v", report)
	}
}
