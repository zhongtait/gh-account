package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeClientRepositoryOperations(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[remote \"origin\"]\n\turl = https://github.com/a/b.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewNativeClient(filepath.Join(root, "nested"))
	isRepo, err := client.IsRepo(context.Background())
	if err != nil || !isRepo {
		t.Fatalf("IsRepo = %v, %v", isRepo, err)
	}
	top, err := client.TopLevel(context.Background())
	if err != nil || top != root {
		t.Fatalf("TopLevel = %q, %v", top, err)
	}
	branch, err := client.CurrentBranch(context.Background())
	if err != nil || branch != "main" {
		t.Fatalf("CurrentBranch = %q, %v", branch, err)
	}
	remoteURL, err := client.GetRemoteURL(context.Background(), "origin")
	if err != nil || remoteURL != "https://github.com/a/b.git" {
		t.Fatalf("GetRemoteURL = %q, %v", remoteURL, err)
	}

	if err := client.SetRemoteURL(context.Background(), "origin", "git@github.com:a/c.git"); err != nil {
		t.Fatalf("SetRemoteURL: %v", err)
	}
	if err := client.SetIdentity(context.Background(), ScopeLocal, Identity{Name: "Tu Xiao", Email: "a@b.com"}); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}
	identity, err := client.GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if identity.Name != "Tu Xiao" || identity.Email != "a@b.com" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
	remoteURL, err = client.GetRemoteURL(context.Background(), "origin")
	if err != nil || remoteURL != "git@github.com:a/c.git" {
		t.Fatalf("updated remote = %q, %v", remoteURL, err)
	}
}

func TestNativeClientGlobalIdentity(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	client := NewNativeClient(t.TempDir())
	if err := client.SetIdentity(context.Background(), ScopeGlobal, Identity{Name: "Global User", Email: "global@example.com"}); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}
	identity, err := client.GetIdentity(context.Background(), ScopeGlobal)
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if identity.Name != "Global User" || identity.Email != "global@example.com" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestNativeClientCredentialHelperReplacesExistingHelpers(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	initial := "[credential]\n\thelper = osxkeychain\n\thelper = old-helper\n\n[remote \"origin\"]\n\turl = https://github.com/a/b.git\n"
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewNativeClient(root)
	if err := client.SetCredentialHelper(context.Background(), ScopeLocal, "!'gha' credential-helper", "github.com|personal-user"); err != nil {
		t.Fatalf("SetCredentialHelper: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "osxkeychain") || strings.Contains(got, "old-helper") {
		t.Fatalf("old credential helpers were not removed:\n%s", got)
	}
	if !strings.Contains(got, "helper = \"\"\n") || !strings.Contains(got, "helper = !'gha' credential-helper\n") {
		t.Fatalf("expected reset and gha credential helpers:\n%s", got)
	}
	if !strings.Contains(got, "account-key = github.com|personal-user") {
		t.Fatalf("credential account key was not saved:\n%s", got)
	}
}

func TestParseScope(t *testing.T) {
	scope, err := ParseScope("global", ScopeLocal)
	if err != nil || scope != ScopeGlobal {
		t.Fatalf("expected global, got %s err=%v", scope, err)
	}
}
