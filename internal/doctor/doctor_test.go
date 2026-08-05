package doctor

import (
	"context"
	"errors"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
)

type fakeGit struct {
	repo bool
}

func (f fakeGit) IsRepo(ctx context.Context) (bool, error)     { return f.repo, nil }
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
	if f.repo {
		return "https://github.com/a/b.git", nil
	}
	return "", errors.New("no remote")
}
func (f fakeGit) SetRemoteURL(ctx context.Context, name, url string) error { return nil }
func (f fakeGit) CurrentBranch(ctx context.Context) (string, error)        { return "main", nil }

type fakeGH struct{}

func (fakeGH) CurrentLogin(ctx context.Context) (string, error)   { return "personal-user", nil }
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
		GitHub: fakeGH{},
	}.Run(context.Background())

	if !report.Healthy() {
		t.Fatalf("expected healthy report: %+v", report)
	}
}
